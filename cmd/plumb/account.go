package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"plumb/internal/doctor"
	"plumb/internal/keystone"
	"plumb/internal/profile"
	"plumb/internal/trace"
)

func linkCmd(args []string, e env) int {
	fs := newFlagSet("plumb link", `Usage: plumb link --config-url <url> [flags]
       plumb link
       plumb link --remove

Remembers the OpenSDN URLs for the cloud in OS_AUTH_URL, so trace and
doctor find them without flags. With no flags, shows the current link.
`, e)
	configURL := fs.String("config-url", "", "OpenSDN Config API `URL` to remember")
	controlURLs := fs.String("control-url", "", "comma-separated control introspect `URLs` to remember; default discovers them")
	remove := fs.Bool("remove", false, "forget the link for this cloud")
	force := fs.Bool("force", false, "save the link even if the Config API does not answer from here")
	var c common
	fs.BoolVar(&c.insecure, "insecure", false, "skip TLS certificate verification")
	fs.BoolVar(&c.noColor, "no-color", false, "disable color")
	pos, err := parse(fs, args)
	if err != nil {
		return flagExit(err)
	}
	if len(pos) != 0 {
		fail(e, "link takes no arguments", "Pass the URL with --config-url.")
		return 2
	}
	authURL := e.getenv("OS_AUTH_URL")
	if authURL == "" {
		fail(e, "OS_AUTH_URL is not set", "Source your openrc first. plumb link saves the URLs for the cloud in OS_AUTH_URL.")
		return 2
	}
	path := profile.Path(e.getenv)
	f, err := profile.Load(path)
	if err != nil {
		fail(e, fmt.Sprintf("read %s: %v", path, err), "Fix or delete the file, then run plumb link again.")
		return 1
	}
	key := profile.Key(authURL)
	p := c.palette(e.stdout, e)
	shown := tildePath(path, e.getenv("HOME"))

	switch {
	case *remove:
		if _, ok := f.Clouds[key]; !ok {
			fmt.Fprintf(e.stdout, "%s has no link.\n", key)
			return 0
		}
		delete(f.Clouds, key)
		if err := f.Save(path); err != nil {
			fail(e, err.Error(), "")
			return 1
		}
		fmt.Fprintf(e.stdout, "%s Removed the link for %s\n", p.Green("✓"), key)
		return 0

	case *configURL == "":
		l, ok := f.Clouds[key]
		if !ok {
			fmt.Fprintf(e.stdout, "%s has no OpenSDN link.\n", key)
			fmt.Fprintf(e.stdout, "  %s  Run `plumb link --config-url <url>` if this cloud runs OpenSDN.\n", p.Cyan("Hint"))
			return 0
		}
		fmt.Fprintf(e.stdout, "%s is linked to OpenSDN at %s\n", key, l.ConfigURL)
		if len(l.ControlURLs) > 0 {
			fmt.Fprintf(e.stdout, "  %s  %s\n", p.Dim("Control"), strings.Join(l.ControlURLs, ", "))
		}
		fmt.Fprintf(e.stdout, "  %s  %s\n", p.Dim("Saved in"), shown)
		return 0
	}

	if !*force {
		c.reqTimeout = 5 * time.Second
		if err := doctor.Reachable(context.Background(), c.client(e, nil), strings.TrimRight(*configURL, "/")+"/"); err != nil {
			fail(e, fmt.Sprintf("%s does not answer from this machine: %v", *configURL, err),
				"Check the URL. To save it for use from another machine, add --force.")
			return 1
		}
	}
	f.Clouds[key] = profile.Link{ConfigURL: *configURL, ControlURLs: splitList(*controlURLs)}
	if err := f.Save(path); err != nil {
		fail(e, err.Error(), "")
		return 1
	}
	fmt.Fprintf(e.stdout, "%s Linked %s to OpenSDN at %s\n", p.Green("✓"), key, *configURL)
	fmt.Fprintf(e.stdout, "  %s  %s\n", p.Dim("Saved in"), shown)
	fmt.Fprintf(e.stdout, "  %s      plumb doctor\n", p.Cyan("Next"))
	return 0
}

func whoamiCmd(args []string, e env) int {
	fs := newFlagSet("plumb whoami", `Usage: plumb whoami [flags]

Shows who the OS_* credentials log in as, and which OpenSDN URLs trace
and doctor will use.
`, e)
	var c common
	c.endpoints(fs, e.getenv)
	c.output(fs)
	pos, err := parse(fs, args)
	if err != nil {
		return flagExit(err)
	}
	if len(pos) != 0 {
		fail(e, "whoami takes no arguments", "")
		return 2
	}
	creds := keystone.CredentialsFromEnv(e.getenv)
	if err := creds.Validate(); err != nil {
		ex, _ := trace.Explain("no-credentials")
		fail(e, err.Error(), ex.Check)
		return 2
	}
	ctx, cancel := c.context()
	defer cancel()
	s, err := keystone.Authenticate(ctx, c.client(e, nil), creds)
	if err != nil {
		iss := trace.Classify(err)
		fail(e, err.Error(), iss.Hint)
		return 1
	}

	c.useLink(e)
	source := "--config-url"
	switch {
	case c.linked:
		source = "plumb link"
	case c.configURL == "":
		source = "not set"
	case c.configURL == e.getenv("OPENSDN_CONFIG_URL"):
		source = "OPENSDN_CONFIG_URL"
	}

	if c.json {
		enc := json.NewEncoder(e.stdout)
		enc.SetIndent("", "  ")
		enc.Encode(map[string]any{
			"user": s.User, "project": s.Project, "roles": s.Roles,
			"auth_url":          creds.AuthURL,
			"config_url":        c.configURL,
			"config_url_source": source,
		})
		return 0
	}
	p := c.palette(e.stdout, e)
	fmt.Fprintf(e.stdout, "%s\n", p.Bold(fmt.Sprintf("%s in project %s", s.User, s.Project)))
	fmt.Fprintf(e.stdout, "  %s  %s\n", p.Dim("Cloud  "), creds.AuthURL)
	fmt.Fprintf(e.stdout, "  %s  %s\n", p.Dim("Roles  "), strings.Join(s.Roles, ", "))
	if c.configURL == "" {
		fmt.Fprintf(e.stdout, "  %s  not set\n", p.Dim("OpenSDN"))
		fmt.Fprintf(e.stdout, "  %s     Run `plumb link --config-url <url>` if this cloud runs OpenSDN.\n", p.Cyan("Hint"))
		return 0
	}
	fmt.Fprintf(e.stdout, "  %s  %s, from %s\n", p.Dim("OpenSDN"), c.configURL, source)
	return 0
}

// tildePath shortens a path under home to ~/…, as a user would type it.
func tildePath(path, home string) string {
	if home != "" && strings.HasPrefix(path, home+"/") {
		return "~" + path[len(home):]
	}
	return path
}
