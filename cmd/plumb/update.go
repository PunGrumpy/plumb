package main

import (
	"context"
	"fmt"
	"os"
	"slices"
	"time"

	"plumb/internal/ui"
	"plumb/internal/update"
)

// checkForUpdate starts the release check in the background and returns a
// function that prints a notice after the command finishes. The check only
// runs for a person at a terminal, never in CI, scripts or --json output,
// and never delays the command by more than half a second.
func checkForUpdate(args []string, e env) func() {
	url := e.getenv("PLUMB_UPDATE_URL")
	if url == "" {
		url = updateURL
	}
	f, ok := e.stderr.(*os.File)
	if !ok || url == "" || version == "dev" || !ui.IsTerminal(f) ||
		e.getenv("CI") != "" || e.getenv("PLUMB_NO_UPDATE_CHECK") != "" || slices.Contains(args, "--json") {
		return func() {}
	}

	found := make(chan update.Release, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		rel, err := update.Checker{URL: url, CachePath: update.CachePath(e.getenv)}.Latest(ctx)
		if err == nil {
			found <- rel
		}
		close(found)
	}()

	return func() {
		select {
		case rel, ok := <-found:
			if ok && update.Newer(version, rel.Version) {
				fmt.Fprint(f, updateNotice(version, rel, installHint, (&common{}).palette(f, e)))
			}
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func updateNotice(current string, rel update.Release, install string, p ui.Palette) string {
	s := fmt.Sprintf("\n%s plumb %s → %s\n", p.Yellow("Update available:"), current, p.Green(rel.Version))
	switch {
	case install != "":
		s += fmt.Sprintf("  %s   %s\n", p.Cyan("Run"), install)
	case rel.URL != "":
		s += fmt.Sprintf("  %s  %s\n", p.Cyan("Get"), rel.URL)
	}
	return s + p.Dim("  Turn this off with PLUMB_NO_UPDATE_CHECK=1") + "\n"
}
