// Command plumb follows one VM from Nova down to the vRouter agent and
// prints every layer as a single tree.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"plumb/internal/httpx"
	"plumb/internal/profile"
	"plumb/internal/ui"
)

// Set at build time with -ldflags "-X main.version=v1.2.3 ...". An empty
// updateURL turns the update check off; installHint is the command that
// the update notice tells the user to run.
var (
	version     = "dev"
	updateURL   = ""
	installHint = ""
)

const help = `plumb follows a VM through every layer of OpenStack and OpenSDN.

Usage
  plumb <vm> [flags]         same as plumb trace <vm>

Commands
  trace <vm>                 follow a VM by UUID, name, fixed IP or floating IP
  path <from> <to>           check whether one VM can reach another
  doctor                     check what this machine can reach
  link                       remember the OpenSDN URLs for this cloud
  whoami                     show the user, project and OpenSDN URLs in use

Learn
  demo [scenario]            trace the built-in lab, no cloud needed
  explain [code]             explain a warning code

Examples
  plumb demo                         start here, no cloud needed
  plumb demo missing-route           see how a broken layer looks
  source openrc admin admin
  plumb link --config-url http://10.0.0.10:8082
  plumb doctor                       once on each machine you trace from
  plumb web-01
  plumb 203.0.113.10                 a floating IP works too
  plumb path web-01 db-01 --port 5432

Run "plumb <command> -h" for the flags of a command.
`

// env is everything a command reads from the outside world, so tests can
// run commands without touching the process.
type env struct {
	stdout, stderr io.Writer
	getenv         func(string) string
}

func main() {
	os.Exit(run(os.Args[1:], env{stdout: os.Stdout, stderr: os.Stderr, getenv: os.Getenv}))
}

func run(args []string, e env) int {
	notify := checkForUpdate(args, e)
	code := dispatch(args, e)
	notify()
	return code
}

func dispatch(args []string, e env) int {
	if len(args) == 0 {
		fmt.Fprint(e.stdout, help)
		return 0
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(e.stdout, help)
		return 0
	case "version", "--version":
		fmt.Fprintf(e.stdout, "plumb %s\n", version)
		return 0
	case "trace":
		return traceCmd(args[1:], e)
	case "path":
		return pathCmd(args[1:], e)
	case "link":
		return linkCmd(args[1:], e)
	case "whoami":
		return whoamiCmd(args[1:], e)
	case "demo":
		return demoCmd(args[1:], e)
	case "doctor":
		return doctorCmd(args[1:], e)
	case "explain":
		return explainCmd(args[1:], e)
	}
	return traceCmd(args, e)
}

// flags shared by trace, demo and doctor.
type common struct {
	configURL     string
	controlURLs   string
	agentURL      string
	controlPort   int
	agentPort     int
	noConfigToken bool
	json          bool
	dump          bool
	insecure      bool
	noColor       bool
	noTimings     bool
	timeout       time.Duration
	reqTimeout    time.Duration

	// linked is true when the OpenSDN URLs came from `plumb link`.
	linked bool
}

func (c *common) output(fs *flag.FlagSet) {
	fs.BoolVar(&c.json, "json", false, "print the result as JSON")
	fs.BoolVar(&c.noColor, "no-color", false, "disable color (NO_COLOR also works)")
	fs.BoolVar(&c.noTimings, "no-timings", false, "hide step durations")
	fs.BoolVar(&c.dump, "debug", false, "log every HTTP call to stderr")
	fs.BoolVar(&c.dump, "dump-http", false, "same as --debug")
}

func (c *common) endpoints(fs *flag.FlagSet, getenv func(string) string) {
	fs.StringVar(&c.configURL, "config-url", getenv("OPENSDN_CONFIG_URL"), "OpenSDN Config API `URL`, for example http://10.0.0.10:8082 (env OPENSDN_CONFIG_URL)")
	fs.StringVar(&c.controlURLs, "control-url", getenv("OPENSDN_CONTROL_URLS"), "comma-separated control introspect `URLs`; default discovers them from bgp-routers")
	fs.StringVar(&c.agentURL, "agent-url", getenv("OPENSDN_AGENT_URL"), "vRouter agent introspect `URL`; default is http://<compute IP>:<agent-port>")
	fs.IntVar(&c.controlPort, "control-port", 8083, "control node introspect port")
	fs.IntVar(&c.agentPort, "agent-port", 8085, "vRouter agent introspect port")
	fs.BoolVar(&c.noConfigToken, "no-config-token", false, "do not send the Keystone token to the Config API")
	fs.BoolVar(&c.insecure, "insecure", false, "skip TLS certificate verification")
	fs.DurationVar(&c.timeout, "timeout", 2*time.Minute, "deadline for the whole command")
	fs.DurationVar(&c.reqTimeout, "request-timeout", 15*time.Second, "deadline for each HTTP call")
}

// useLink fills the OpenSDN URLs saved by `plumb link` for this cloud when
// no flag or environment variable set them. Flags and variables win.
func (c *common) useLink(e env) {
	if c.configURL != "" {
		return
	}
	f, err := profile.Load(profile.Path(e.getenv))
	if err != nil {
		return
	}
	l, ok := f.Lookup(e.getenv("OS_AUTH_URL"))
	if !ok {
		return
	}
	c.configURL, c.linked = l.ConfigURL, true
	if c.controlURLs == "" {
		c.controlURLs = strings.Join(l.ControlURLs, ",")
	}
}

// header prints the version on stderr, as the first line an interactive
// user sees. Pipes and --json never get it.
func (c *common) header(e env) {
	f, ok := e.stderr.(*os.File)
	if !ok || c.json || !ui.IsTerminal(f) {
		return
	}
	p := c.palette(f, e)
	line := "plumb " + version
	if c.linked {
		line += ", OpenSDN " + c.configURL + " from plumb link"
	}
	fmt.Fprintln(f, p.Dim(line))
}

func (c *common) transport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	if c.insecure {
		t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return t
}

// client builds the HTTP client on top of rt, or on a fresh transport when
// rt is nil.
func (c *common) client(e env, rt http.RoundTripper) *httpx.Client {
	if rt == nil {
		rt = c.transport()
	}
	hc := &httpx.Client{HTTP: &http.Client{Transport: rt, Timeout: c.reqTimeout}}
	if c.dump {
		hc.Dump = e.stderr
	}
	return hc
}

func (c *common) palette(w io.Writer, e env) ui.Palette {
	if c.noColor {
		return ui.Palette{}
	}
	f, ok := w.(*os.File)
	return ui.Palette{On: ok && ui.ColorEnabled(f, e.getenv)}
}

// progress starts a spinner on stderr when stderr is a terminal. The
// returned stop function is always safe to call.
func (c *common) progress(e env, hc *httpx.Client) (set func(string), stop func()) {
	f, ok := e.stderr.(*os.File)
	if !ok || c.dump || !ui.IsTerminal(f) || e.getenv("CI") != "" {
		return func(string) {}, func() {}
	}
	sp := ui.StartSpinner(f, c.palette(f, e), ui.Width(e.getenv))
	stage := ""
	set = func(s string) {
		stage = s
		sp.Set(stage)
	}
	hc.OnRequest = func(method, url string) {
		sp.Set(fmt.Sprintf("%-14s %s %s", stage, method, url))
	}
	return set, sp.Stop
}

func (c *common) context() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), c.timeout)
}

// parse accepts flags before and after positional arguments.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return pos, nil
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

// fail prints an error in the same shape everywhere: what went wrong, then
// what to do about it.
func fail(e env, msg, hint string) {
	p := (&common{}).palette(e.stderr, e)
	fmt.Fprintf(e.stderr, "%s %s\n", p.Red(p.Bold("Error:")), msg)
	if hint != "" {
		fmt.Fprintf(e.stderr, "%s  %s\n", p.Cyan("Hint:"), hint)
	}
}

func splitList(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// closest returns the candidate within two edits of s, for "did you mean".
func closest(s string, candidates []string) string {
	best, bestD := "", 3
	for _, c := range candidates {
		if d := distance(s, c); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}

func distance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}
