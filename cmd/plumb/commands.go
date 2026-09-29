package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"time"

	"plumb/internal/demo"
	"plumb/internal/doctor"
	"plumb/internal/httpx"
	"plumb/internal/keystone"
	"plumb/internal/render"
	"plumb/internal/trace"
)

var commands = []string{"trace", "path", "doctor", "link", "whoami", "demo", "explain", "version", "help"}

func newFlagSet(name, usage string, e env) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	fs.Usage = func() {
		fmt.Fprint(e.stderr, usage+"\nFlags:\n")
		fs.PrintDefaults()
	}
	return fs
}

// flagExit maps a flag parse error to an exit code. The flag package has
// already printed the message.
func flagExit(err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	return 2
}

func traceCmd(args []string, e env) int {
	fs := newFlagSet("plumb trace", `Usage: plumb trace <vm> [flags]
       plumb <vm> [flags]

<vm> is a server UUID, name, fixed IP or floating IP. Credentials come
from the OS_* variables that openrc exports. The OpenSDN URLs come from
the flags, the OPENSDN_* variables or plumb link, in that order. Without
them the OpenSDN stages skip.
`, e)
	var c common
	c.endpoints(fs, e.getenv)
	c.output(fs)
	record := fs.String("record", "", "save every HTTP response into `DIR`")
	replay := fs.String("replay", "", "answer HTTP calls from recordings in `DIR`, without network access")
	pos, err := parse(fs, args)
	if err != nil {
		return flagExit(err)
	}
	if len(pos) != 1 {
		fail(e, "expected one VM UUID or name", "Run `plumb demo` to try the built-in lab first.")
		return 2
	}
	rt, creds, code := c.connect(e, *record, *replay, pos[0])
	if code != 0 {
		return code
	}
	c.useLink(e)
	c.header(e)
	return c.runTrace(e, c.client(e, rt), creds, pos[0], c.configURL)
}

// connect picks the transport and credentials for trace and path. A
// replay needs no credentials; a live run exits 2 without them. typed is
// the first argument, used to suggest a command the user may have meant.
func (c *common) connect(e env, record, replay, typed string) (http.RoundTripper, keystone.Credentials, int) {
	if record != "" && replay != "" {
		fail(e, "--record and --replay cannot be used together", "")
		return nil, keystone.Credentials{}, 2
	}
	creds := keystone.CredentialsFromEnv(e.getenv)
	if replay != "" {
		if creds.Username == "" && creds.AppCredID == "" {
			// Recordings ignore the request body, so any credentials will do.
			creds.Username, creds.Password, creds.ProjectName = "replay", "replay", "replay"
		}
		return &httpx.Replayer{Read: httpx.DirReader(replay)}, creds, 0
	}
	if err := creds.Validate(); err != nil {
		if cmd := closest(typed, commands); cmd != "" && !trace.IsUUID(typed) {
			fail(e, fmt.Sprintf("unknown command %q", typed), fmt.Sprintf("Did you mean `plumb %s`?", cmd))
			return nil, creds, 2
		}
		ex, _ := trace.Explain("no-credentials")
		fail(e, err.Error(), ex.Check)
		return nil, creds, 2
	}
	if record != "" {
		return &httpx.Recorder{Dir: record, Next: c.transport()}, creds, 0
	}
	return nil, creds, 0
}

func pathCmd(args []string, e env) int {
	fs := newFlagSet("plumb path", `Usage: plumb path <from> <to> [flags]

Checks whether <from> can reach <to>: both ports, the router between the
networks, both security groups and, on OpenSDN, the route and next hop
in the source VRF. <from> and <to> take a UUID, name or IP, as trace does.
`, e)
	var c common
	c.endpoints(fs, e.getenv)
	c.output(fs)
	proto := fs.String("proto", "icmp", "protocol to check: icmp, tcp or udp; tcp when only --port is set")
	port := fs.Int("port", 0, "destination port for tcp and udp")
	record := fs.String("record", "", "save every HTTP response into `DIR`")
	replay := fs.String("replay", "", "answer HTTP calls from recordings in `DIR`, without network access")
	pos, err := parse(fs, args)
	if err != nil {
		return flagExit(err)
	}
	protoSet := false
	fs.Visit(func(f *flag.Flag) { protoSet = protoSet || f.Name == "proto" })
	if *port != 0 && !protoSet {
		*proto = "tcp"
	}
	if len(pos) != 2 {
		fail(e, "expected two VMs: plumb path <from> <to>", "For example `plumb path web-01 db-01 --port 5432`.")
		return 2
	}
	switch {
	case *proto != "icmp" && *proto != "tcp" && *proto != "udp":
		fail(e, fmt.Sprintf("--proto %q is not icmp, tcp or udp", *proto), "")
		return 2
	case *proto != "icmp" && (*port < 1 || *port > 65535):
		fail(e, "--port between 1 and 65535 is required with --proto "+*proto, fmt.Sprintf("For example `plumb path %s %s --proto %s --port 22`.", pos[0], pos[1], *proto))
		return 2
	}
	rt, creds, code := c.connect(e, *record, *replay, "")
	if code != 0 {
		return code
	}
	c.useLink(e)
	c.header(e)

	hc := c.client(e, rt)
	set, stop := c.progress(e, hc)
	ctx, cancel := c.context()
	defer cancel()
	p := trace.TracePath(ctx, pos[0], pos[1], c.traceOptions(hc, creds, c.configURL, set), trace.PathOptions{Protocol: *proto, Port: *port})
	stop()

	if c.json {
		enc := json.NewEncoder(e.stdout)
		enc.SetIndent("", "  ")
		enc.Encode(p)
	} else {
		render.Path(e.stdout, p, render.Options{Color: c.palette(e.stdout, e).On})
	}
	if p.Failed() {
		return 1
	}
	return 0
}

func (c *common) traceOptions(hc *httpx.Client, creds keystone.Credentials, configURL string, onStage func(string)) trace.Options {
	return trace.Options{
		HTTP:        hc,
		Creds:       creds,
		ConfigURL:   configURL,
		ConfigToken: !c.noConfigToken,
		ControlURLs: splitList(c.controlURLs),
		ControlPort: c.controlPort,
		AgentURL:    c.agentURL,
		AgentPort:   c.agentPort,
		OnStage:     onStage,
	}
}

func (c *common) runTrace(e env, hc *httpx.Client, creds keystone.Credentials, vm, configURL string) int {
	set, stop := c.progress(e, hc)
	ctx, cancel := c.context()
	defer cancel()
	t := trace.Run(ctx, vm, c.traceOptions(hc, creds, configURL, set))
	stop()

	if c.json {
		if err := render.JSON(e.stdout, t); err != nil {
			fail(e, err.Error(), "")
			return 1
		}
	} else {
		render.Tree(e.stdout, t, render.Options{Timings: !c.noTimings, Color: c.palette(e.stdout, e).On})
	}
	if t.Failed() {
		return 1
	}
	return 0
}

func demoCmd(args []string, e env) int {
	fs := newFlagSet("plumb demo", `Usage: plumb demo [scenario] [flags]

Traces web-01 in a lab recorded inside the binary. No cloud, credentials
or network access needed. Each scenario breaks one layer.
`, e)
	var c common
	c.output(fs)
	list := fs.Bool("list", false, "list the scenarios")
	pos, err := parse(fs, args)
	if err != nil {
		return flagExit(err)
	}
	if *list || (len(pos) == 1 && pos[0] == "list") {
		p := c.palette(e.stdout, e)
		fmt.Fprintln(e.stdout, p.Bold("Scenarios"))
		for _, s := range demo.Scenarios {
			fmt.Fprintf(e.stdout, "  %-16s %s\n", s.Name, p.Dim(s.Description))
		}
		fmt.Fprintf(e.stdout, "\nRun one with: plumb demo %s\n", demo.Scenarios[3].Name)
		return 0
	}
	if len(pos) > 1 {
		fail(e, "expected at most one scenario", "Run `plumb demo --list` to see them.")
		return 2
	}
	name := demo.Scenarios[0].Name
	if len(pos) == 1 {
		name = pos[0]
	}
	sc, ok := demo.Find(name)
	if !ok {
		hint := "Run `plumb demo --list` to see them."
		var names []string
		for _, s := range demo.Scenarios {
			names = append(names, s.Name)
		}
		if m := closest(name, names); m != "" {
			hint = fmt.Sprintf("Did you mean `plumb demo %s`?", m)
		}
		fail(e, fmt.Sprintf("no demo scenario %q", name), hint)
		return 2
	}

	c.timeout, c.reqTimeout = time.Minute, 15*time.Second
	c.header(e)
	configURL := demo.ConfigURL
	if sc.NoOpenSDN {
		configURL = ""
	}
	creds := keystone.Credentials{AuthURL: demo.AuthURL, Username: "demo", Password: "demo", ProjectName: "admin"}
	ep := c.palette(e.stderr, e)
	if !c.json {
		fmt.Fprintf(e.stderr, "%s\n\n", ep.Dim(fmt.Sprintf("Demo lab, scenario %s: %s", sc.Name, sc.Description)))
	}
	code := c.runTrace(e, c.client(e, &httpx.Replayer{Read: sc.Read}), creds, demo.VMName, configURL)
	if !c.json {
		fmt.Fprintf(e.stderr, "\n%s\n", ep.Dim("Next: `plumb demo --list` for broken scenarios, or `plumb doctor` on a machine with your openrc."))
	}
	return code
}

func doctorCmd(args []string, e env) int {
	fs := newFlagSet("plumb doctor", `Usage: plumb doctor [flags]

Checks the credentials, then probes every endpoint plumb calls: the
OpenStack APIs, the Config API, each control node and every vRouter
agent. Run it once on each machine you plan to trace from.
`, e)
	var c common
	c.endpoints(fs, e.getenv)
	c.output(fs)
	pos, err := parse(fs, args)
	if err != nil {
		return flagExit(err)
	}
	if len(pos) != 0 {
		fail(e, "doctor takes no arguments", "")
		return 2
	}
	c.useLink(e)
	c.header(e)
	hc := c.client(e, c.transport())
	set, stop := c.progress(e, hc)
	ctx, cancel := c.context()
	defer cancel()
	checks := doctor.Run(ctx, doctor.Options{
		HTTP:        hc,
		Creds:       keystone.CredentialsFromEnv(e.getenv),
		ConfigURL:   c.configURL,
		ConfigToken: !c.noConfigToken,
		ControlURLs: splitList(c.controlURLs),
		ControlPort: c.controlPort,
		AgentPort:   c.agentPort,
		OnCheck:     set,
	})
	stop()

	if c.json {
		enc := json.NewEncoder(e.stdout)
		enc.SetIndent("", "  ")
		enc.Encode(checks)
	} else {
		doctor.Render(e.stdout, checks, c.palette(e.stdout, e))
	}
	for _, ch := range checks {
		if ch.Status == trace.StatusFail {
			return 1
		}
	}
	return 0
}

func explainCmd(args []string, e env) int {
	var c common
	p := c.palette(e.stdout, e)
	if len(args) == 0 {
		fmt.Fprintln(e.stdout, p.Bold("Warning and error codes"))
		for _, code := range trace.Codes() {
			ex, _ := trace.Explain(code)
			fmt.Fprintf(e.stdout, "  %-20s %s\n", code, p.Dim(ex.Meaning))
		}
		fmt.Fprintln(e.stdout, "\nRun: plumb explain <code>")
		return 0
	}
	ex, ok := trace.Explain(args[0])
	if !ok {
		hint := "Run `plumb explain` to list every code."
		if m := closest(args[0], trace.Codes()); m != "" {
			hint = fmt.Sprintf("Did you mean `plumb explain %s`?", m)
		}
		fail(e, fmt.Sprintf("no code %q", args[0]), hint)
		return 2
	}
	fmt.Fprintf(e.stdout, "%s\n\n%s\n\n%s  %s\n", p.Bold(ex.Code), ex.Meaning, p.Cyan("Next:"), ex.Check)
	fmt.Fprintf(e.stdout, "%s  docs/troubleshooting.md#%s\n", p.Cyan("Docs:"), ex.Code)
	return 0
}
