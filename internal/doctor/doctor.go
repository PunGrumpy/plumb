// Package doctor checks, before any trace, that this machine has
// credentials and can reach every layer plumb calls: the OpenStack APIs,
// the Config API, each control node and each vRouter agent.
package doctor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"plumb/internal/httpx"
	"plumb/internal/keystone"
	"plumb/internal/opensdn/config"
	"plumb/internal/trace"
	"plumb/internal/ui"
)

type Options struct {
	HTTP        *httpx.Client
	Creds       keystone.Credentials
	ConfigURL   string
	ConfigToken bool
	ControlURLs []string
	ControlPort int
	AgentPort   int
	// ProbeTimeout bounds each reachability probe. Default 3s.
	ProbeTimeout time.Duration
	// OnCheck, when set, is called as each check starts.
	OnCheck func(name string)

	// agentURL builds an agent's introspect URL from its IP. Tests replace
	// it; the default uses AgentPort.
	agentURL func(ip string) string
}

// Check is one line of the report. Failed lists the targets that did not
// answer when a check probes several of them.
type Check struct {
	Name   string       `json:"name"`
	Status trace.Status `json:"status"`
	Detail string       `json:"detail"`
	Hint   string       `json:"hint,omitempty"`
	Failed []string     `json:"failed,omitempty"`
}

type doctor struct {
	o      Options
	sess   *keystone.Session
	checks []Check
}

// Run performs every check in order. A check that depends on a failed one
// is skipped.
func Run(ctx context.Context, o Options) []Check {
	if o.ProbeTimeout == 0 {
		o.ProbeTimeout = 3 * time.Second
	}
	if o.ControlPort == 0 {
		o.ControlPort = 8083
	}
	if o.AgentPort == 0 {
		o.AgentPort = 8085
	}
	if o.agentURL == nil {
		o.agentURL = func(ip string) string { return fmt.Sprintf("http://%s:%d", ip, o.AgentPort) }
	}
	d := &doctor{o: o}
	d.credentials(ctx)
	for _, svc := range []string{"compute", "network", "image"} {
		d.endpoint(ctx, svc)
	}
	vrs := d.configAPI(ctx)
	d.controlNodes(ctx)
	d.agents(ctx, vrs)
	return d.checks
}

func (d *doctor) start(name string) {
	if d.o.OnCheck != nil {
		d.o.OnCheck(name)
	}
}

func (d *doctor) add(c Check) { d.checks = append(d.checks, c) }

func (d *doctor) credentials(ctx context.Context) {
	d.start("keystone")
	if err := d.o.Creds.Validate(); err != nil {
		e, _ := trace.Explain("no-credentials")
		d.add(Check{Name: "keystone", Status: trace.StatusFail, Detail: err.Error(), Hint: e.Check})
		return
	}
	s, err := keystone.Authenticate(ctx, d.o.HTTP, d.o.Creds)
	if err != nil {
		d.add(failed("keystone", err))
		return
	}
	d.sess = s
	c := Check{
		Name:   "keystone",
		Status: trace.StatusOK,
		Detail: fmt.Sprintf("token for %s in project %s, roles %s", s.User, s.Project, strings.Join(s.Roles, ", ")),
	}
	if !s.HasRole("admin") {
		e, _ := trace.Explain("no-admin")
		c.Status, c.Hint = trace.StatusWarn, e.Check
	}
	d.add(c)
}

// endpoint probes the catalog URL of one service. Any HTTP answer, even an
// error status, proves the endpoint is reachable.
func (d *doctor) endpoint(ctx context.Context, svc string) {
	d.start(svc)
	if d.sess == nil {
		d.add(Check{Name: svc, Status: trace.StatusSkip, Detail: "needs a Keystone token"})
		return
	}
	u, err := d.sess.Endpoint(svc, d.o.Creds.Interface, d.o.Creds.Region)
	if err != nil {
		e, _ := trace.Explain("endpoint-missing")
		d.add(Check{Name: svc, Status: trace.StatusFail, Detail: err.Error(), Hint: e.Check})
		return
	}
	if err := d.probe(ctx, u); err != nil {
		d.add(failed(svc, err))
		return
	}
	d.add(Check{Name: svc, Status: trace.StatusOK, Detail: u})
}

func (d *doctor) configAPI(ctx context.Context) []config.VirtualRouter {
	d.start("opensdn-config")
	if d.o.ConfigURL == "" {
		d.add(Check{Name: "opensdn-config", Status: trace.StatusSkip, Detail: "no Config API URL; run plumb link if the cloud runs OpenSDN"})
		return nil
	}
	token := ""
	if d.o.ConfigToken && d.sess != nil {
		token = d.sess.Token
	}
	c := config.New(d.o.HTTP, d.o.ConfigURL, token)
	vrs, err := c.VirtualRouters(ctx)
	if err != nil {
		d.add(failed("opensdn-config", err))
		return nil
	}
	d.add(Check{Name: "opensdn-config", Status: trace.StatusOK, Detail: fmt.Sprintf("%s answers, %d virtual-routers", d.o.ConfigURL, len(vrs))})

	if len(d.o.ControlURLs) == 0 {
		nodes, err := c.ControlNodes(ctx)
		if err == nil {
			for _, n := range nodes {
				d.o.ControlURLs = append(d.o.ControlURLs, fmt.Sprintf("http://%s:%d", n.Params.Address, d.o.ControlPort))
			}
		}
	}
	return vrs
}

func (d *doctor) controlNodes(ctx context.Context) {
	d.start("control")
	if len(d.o.ControlURLs) == 0 {
		d.add(Check{Name: "control", Status: trace.StatusSkip, Detail: "no control nodes known; needs the Config API or --control-url"})
		return
	}
	targets := make([]target, len(d.o.ControlURLs))
	for i, u := range d.o.ControlURLs {
		targets[i] = target{name: u, url: u}
	}
	d.add(d.probeAll(ctx, "control", targets, "control-unreachable"))
}

func (d *doctor) agents(ctx context.Context, vrs []config.VirtualRouter) {
	d.start("vrouter")
	if len(vrs) == 0 {
		d.add(Check{Name: "vrouter", Status: trace.StatusSkip, Detail: "no virtual-routers known; needs the Config API"})
		return
	}
	targets := make([]target, len(vrs))
	for i, vr := range vrs {
		targets[i] = target{
			name: fmt.Sprintf("%s %s", vr.FQName[len(vr.FQName)-1], vr.IPAddress),
			url:  d.o.agentURL(vr.IPAddress),
		}
	}
	d.add(d.probeAll(ctx, "vrouter", targets, "unreachable"))
}

type target struct{ name, url string }

// probeAll probes targets in parallel, 16 at a time, and summarises them
// as one check.
func (d *doctor) probeAll(ctx context.Context, name string, targets []target, code string) Check {
	errs := make([]error, len(targets))
	sem := make(chan struct{}, 16)
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			errs[i] = d.probe(ctx, t.url+"/")
		}()
	}
	wg.Wait()

	c := Check{Name: name}
	for i, err := range errs {
		if err != nil {
			c.Failed = append(c.Failed, fmt.Sprintf("%s: %s", targets[i].name, short(err)))
		}
	}
	ok := len(targets) - len(c.Failed)
	c.Detail = fmt.Sprintf("%d of %d reachable", ok, len(targets))
	switch {
	case len(c.Failed) == 0:
		c.Status = trace.StatusOK
		return c
	case ok == 0:
		c.Status = trace.StatusFail
	default:
		c.Status = trace.StatusWarn
	}
	e, _ := trace.Explain(code)
	c.Hint = e.Check
	return c
}

func (d *doctor) probe(ctx context.Context, url string) error {
	ctx, cancel := context.WithTimeout(ctx, d.o.ProbeTimeout)
	defer cancel()
	return Reachable(ctx, d.o.HTTP, url)
}

// Reachable reports whether url answers HTTP at all. An error status still
// proves that something listens there.
func Reachable(ctx context.Context, hc *httpx.Client, url string) error {
	_, err := hc.Do(ctx, http.MethodGet, url, nil, nil)
	var se *httpx.StatusError
	if errors.As(err, &se) {
		return nil
	}
	return err
}

func failed(name string, err error) Check {
	iss := trace.Classify(err)
	return Check{Name: name, Status: trace.StatusFail, Detail: short(err), Hint: iss.Hint}
}

// short keeps the cause of a network error ("connection refused", "i/o
// timeout") or the status code of an HTTP error.
func short(err error) string {
	var se *httpx.StatusError
	if errors.As(err, &se) {
		return fmt.Sprintf("HTTP %d", se.Code)
	}
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i >= 0 {
		return msg[i+2:]
	}
	return msg
}

// Render prints one line per check, the targets that failed, and a
// verdict that says how far a trace from this machine can go.
func Render(w io.Writer, checks []Check, p ui.Palette) {
	fmt.Fprintln(w, p.Bold("Doctor"))
	var firstFail, firstWarn *Check
	skippedOpenSDN := false
	for i := range checks {
		c := &checks[i]
		detail := c.Detail
		if c.Status == trace.StatusSkip {
			detail = p.Dim(detail)
		}
		fmt.Fprintf(w, "  %s %-15s %s\n", p.Mark(string(c.Status)), c.Name, detail)
		for _, f := range c.Failed {
			fmt.Fprintf(w, "      %s %s\n", p.Red("✗"), f)
		}
		switch {
		case c.Status == trace.StatusFail && firstFail == nil:
			firstFail = c
		case c.Status == trace.StatusWarn && firstWarn == nil:
			firstWarn = c
		case c.Status == trace.StatusSkip && c.Name == "opensdn-config":
			skippedOpenSDN = true
		}
	}
	fmt.Fprintln(w)
	switch {
	case firstFail != nil:
		fmt.Fprintf(w, "%s %s\n", p.Red("✗"), p.Bold("A trace from this machine stops at "+firstFail.Name))
		if firstFail.Hint != "" {
			fmt.Fprintf(w, "  %s  %s\n", p.Cyan("Hint"), firstFail.Hint)
		}
	case firstWarn != nil:
		fmt.Fprintf(w, "%s %s\n", p.Yellow("!"), p.Bold("This machine can trace, with gaps at "+firstWarn.Name))
		if firstWarn.Hint != "" {
			fmt.Fprintf(w, "  %s  %s\n", p.Cyan("Hint"), firstWarn.Hint)
		}
	case skippedOpenSDN:
		fmt.Fprintf(w, "%s %s\n", p.Green("✓"), p.Bold("This machine can trace down to Neutron"))
		fmt.Fprintf(w, "  %s  Run `plumb link --config-url <url>` to check the OpenSDN layers too.\n", p.Cyan("Hint"))
	default:
		fmt.Fprintf(w, "%s %s\n", p.Green("✓"), p.Bold("This machine can run a full trace"))
	}
}
