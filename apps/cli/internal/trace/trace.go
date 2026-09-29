// Package trace follows one VM through Keystone, Nova, Neutron, the OpenSDN
// Config API, the control nodes and the vRouter agent.
//
// Each stage reads what earlier stages found and adds its own layer to the
// Trace. A stage that cannot run is skipped rather than aborting the run,
// so the output always shows how far the chain goes and where it stops.
package trace

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/PunGrumpy/plumb/apps/cli/internal/httpx"
	"github.com/PunGrumpy/plumb/apps/cli/internal/keystone"
)

// Options are the inputs to Run. Only HTTP and Creds are required; leave
// ConfigURL empty on clouds that do not run OpenSDN (DevStack with OVN).
type Options struct {
	HTTP  *httpx.Client
	Creds keystone.Credentials

	ConfigURL   string
	ConfigToken bool // send the Keystone token to the Config API

	ControlURLs []string // skip bgp-router discovery when set
	ControlPort int

	AgentURL  string // skip compute IP discovery when set
	AgentPort int

	// OnStage, when set, is called as each stage starts. The CLI uses it to
	// show progress.
	OnStage func(name string)
}

type run struct {
	opts     Options
	t        *Trace
	sess     *keystone.Session
	warnings []Issue
}

// Run executes every stage in order and never returns nil. vm is a server
// UUID or name; the nova stage resolves a name to its UUID.
func Run(ctx context.Context, vm string, opts Options) *Trace {
	if opts.ControlPort == 0 {
		opts.ControlPort = 8083
	}
	if opts.AgentPort == 0 {
		opts.AgentPort = 8085
	}
	r := &run{opts: opts, t: &Trace{Query: vm, VMID: vm, Ports: []*Port{}}}
	stages := []struct {
		name string
		run  func(context.Context) error
	}{
		{"keystone", r.keystone},
		{"nova", r.nova},
		{"neutron", r.neutron},
		{"opensdn-config", r.config},
		{"control", r.control},
		{"vrouter", r.vrouter},
	}
	for _, s := range stages {
		if opts.OnStage != nil {
			opts.OnStage(s.name)
		}
		r.warnings = nil
		start := time.Now()
		err := s.run(ctx)
		r.t.Steps = append(r.t.Steps, r.finish(s.name, err, time.Since(start)))
	}
	return r.t
}

type skipError struct{ reason string }

func (e *skipError) Error() string { return e.reason }

func skip(format string, a ...any) error {
	return &skipError{reason: fmt.Sprintf(format, a...)}
}

// codedError is a stage failure with a known cause.
type codedError struct {
	code string
	msg  string
}

func (e *codedError) Error() string { return e.msg }

func fail(code, format string, a ...any) error {
	return &codedError{code: code, msg: fmt.Sprintf(format, a...)}
}

func (r *run) warn(code, format string, a ...any) {
	r.warnings = append(r.warnings, newIssue(code, fmt.Sprintf(format, a...)))
}

func (r *run) finish(name string, err error, d time.Duration) Step {
	s := Step{Name: name, Warnings: r.warnings, DurationMS: d.Milliseconds()}
	var se *skipError
	switch {
	case errors.As(err, &se):
		s.Status, s.Error = StatusSkip, se.reason
	case err != nil:
		iss := Classify(err)
		s.Status, s.Error, s.Code, s.Hint = StatusFail, iss.Message, iss.Code, iss.Hint
	case len(r.warnings) > 0:
		s.Status = StatusWarn
	default:
		s.Status = StatusOK
	}
	return s
}

// port returns the Port with id, adding it if this is the first sighting.
func (r *run) port(id string) *Port {
	for _, p := range r.t.Ports {
		if p.ID == id {
			return p
		}
	}
	p := &Port{ID: id}
	r.t.Ports = append(r.t.Ports, p)
	return p
}

func (r *run) endpoint(serviceType string) string {
	if r.t.Identity == nil {
		return ""
	}
	return r.t.Identity.Endpoints[serviceType]
}

// shortHost strips the domain so "compute-02.lab" matches "compute-02".
func shortHost(h string) string {
	if i := strings.IndexByte(h, '.'); i > 0 {
		return h[:i]
	}
	return h
}

var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-?[0-9a-fA-F]{4}-?[0-9a-fA-F]{4}-?[0-9a-fA-F]{4}-?[0-9a-fA-F]{12}$`)

// IsUUID reports whether s looks like a UUID rather than a server name.
func IsUUID(s string) bool { return uuidRE.MatchString(s) }

func appendUnique(list []string, v string) []string {
	if slices.Contains(list, v) {
		return list
	}
	return append(list, v)
}
