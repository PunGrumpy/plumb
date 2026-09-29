// Package ui holds the terminal details: color, and a spinner that shows
// which call plumb is waiting on.
package ui

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"
)

// Palette wraps text in ANSI colors when On is true.
type Palette struct{ On bool }

func (p Palette) wrap(code, s string) string {
	if !p.On || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (p Palette) Bold(s string) string   { return p.wrap("1", s) }
func (p Palette) Dim(s string) string    { return p.wrap("2", s) }
func (p Palette) Red(s string) string    { return p.wrap("31", s) }
func (p Palette) Green(s string) string  { return p.wrap("32", s) }
func (p Palette) Yellow(s string) string { return p.wrap("33", s) }
func (p Palette) Cyan(s string) string   { return p.wrap("36", s) }

// IsTerminal reports whether f is an interactive terminal.
func IsTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// ColorEnabled follows the NO_COLOR and FORCE_COLOR conventions, then falls
// back to whether f is a terminal.
func ColorEnabled(f *os.File, getenv func(string) string) bool {
	switch {
	case getenv("NO_COLOR") != "":
		return false
	case getenv("FORCE_COLOR") != "" && getenv("FORCE_COLOR") != "0":
		return true
	case getenv("TERM") == "dumb":
		return false
	}
	return IsTerminal(f)
}

// Width returns the terminal width from $COLUMNS, or 80.
func Width(getenv func(string) string) int {
	if n, err := strconv.Atoi(getenv("COLUMNS")); err == nil && n > 20 {
		return n
	}
	return 80
}

// Mark returns the colored symbol for a stage or check status: ok, warn,
// fail or skip.
func (p Palette) Mark(status string) string {
	switch status {
	case "ok":
		return p.Green("✓")
	case "warn":
		return p.Yellow("!")
	case "fail":
		return p.Red("✗")
	}
	return p.Dim("-")
}

var frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner redraws one status line until Stop clears it.
type Spinner struct {
	w     io.Writer
	p     Palette
	width int
	mu    sync.Mutex
	text  string
	stop  chan struct{}
	done  chan struct{}
}

func StartSpinner(w io.Writer, p Palette, width int) *Spinner {
	s := &Spinner{w: w, p: p, width: width, stop: make(chan struct{}), done: make(chan struct{})}
	go s.loop()
	return s
}

// Set replaces the status text.
func (s *Spinner) Set(text string) {
	s.mu.Lock()
	s.text = text
	s.mu.Unlock()
}

// Stop clears the line and returns once the spinner is gone.
func (s *Spinner) Stop() {
	close(s.stop)
	<-s.done
}

func (s *Spinner) loop() {
	defer close(s.done)
	t := time.NewTicker(80 * time.Millisecond)
	defer t.Stop()
	for i := 0; ; i++ {
		s.mu.Lock()
		text := truncate(s.text, s.width-3)
		s.mu.Unlock()
		fmt.Fprintf(s.w, "\r\x1b[2K%s %s", s.p.Cyan(frames[i%len(frames)]), text)
		select {
		case <-s.stop:
			fmt.Fprint(s.w, "\r\x1b[2K")
			return
		case <-t.C:
		}
	}
}

func truncate(s string, n int) string {
	if n <= 1 || utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}
