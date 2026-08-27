// Package mixer drives the occmixer launch agent from occam, so the spatial
// renderer can be toggled without a terminal.
//
// It goes through launchd rather than spawning occmixer directly: the agent
// has to survive occam quitting, and launchd already owns restarting it when
// the dongle comes back.
package mixer

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

const Label = "com.dappermint.occmixer"

// Layouts are what occmixer accepts for --layout.
var Layouts = []string{"7.1", "7.1.4"}

// Status is what the window renders.
type Status struct {
	Installed bool
	Running   bool
	Layout    string
	Frames    int
}

func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", Label+".plist")
}

func binaryPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin", "occmixer")
}

func domain() string { return fmt.Sprintf("gui/%d", os.Getuid()) }

var argRe = regexp.MustCompile(`<string>([^<]*)</string>`)

// Read reports what the agent is set to and whether it is loaded.
func Read() Status {
	st := Status{Layout: Layouts[0], Frames: 128}

	raw, err := os.ReadFile(plistPath())
	if err != nil {
		return st
	}
	st.Installed = true

	args := argRe.FindAllStringSubmatch(string(raw), -1)
	for i, m := range args {
		if i+1 >= len(args) {
			break
		}
		switch m[1] {
		case "--layout":
			st.Layout = args[i+1][1]
		case "--frames":
			fmt.Sscanf(args[i+1][1], "%d", &st.Frames)
		}
	}

	st.Running = exec.Command("launchctl", "list", Label).Run() == nil
	return st
}

// Start loads the agent, writing the plist first if it is missing.
func Start(layout string, frames int) error {
	if err := writePlist(layout, frames); err != nil {
		return err
	}
	// Bootstrap reads the plist only on load, so a rewritten one needs a full
	// unload first. Already unloaded is not a failure.
	_ = exec.Command("launchctl", "bootout", domain()+"/"+Label).Run()
	waitGone(3 * time.Second)

	out, err := exec.Command("launchctl", "bootstrap", domain(), plistPath()).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl bootstrap: %s", firstLine(out, err))
	}
	return nil
}

// waitGone blocks until launchd has finished tearing the service down.
// bootout returns before that happens, and bootstrapping against a label that
// still exists fails with EIO.
func waitGone(limit time.Duration) {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if exec.Command("launchctl", "list", Label).Run() != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Stop unloads the agent. occmixer restores system audio as it exits.
func Stop() error {
	out, err := exec.Command("launchctl", "bootout", domain()+"/"+Label).CombinedOutput()
	if err != nil && !bytes.Contains(out, []byte("No such process")) {
		return fmt.Errorf("launchctl bootout: %s", firstLine(out, err))
	}
	waitGone(3 * time.Second)
	return nil
}

// SetLayout rewrites the plist, and restarts only if the agent was running.
// occmixer builds its pipeline once at startup, so a layout change cannot
// take effect without one.
func SetLayout(layout string) error {
	st := Read()
	if err := writePlist(layout, st.Frames); err != nil {
		return err
	}
	if !st.Running {
		return nil
	}
	return Start(layout, st.Frames)
}

func writePlist(layout string, frames int) error {
	if frames <= 0 {
		frames = 128
	}
	if !valid(layout) {
		return fmt.Errorf("no layout %q, have %s", layout, strings.Join(Layouts, " and "))
	}
	bin := binaryPath()
	if _, err := os.Stat(bin); err != nil {
		return fmt.Errorf("occmixer is not installed at %s", bin)
	}

	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>--layout</string>
		<string>%s</string>
		<string>--frames</string>
		<string>%d</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>ThrottleInterval</key>
	<integer>10</integer>
	<key>StandardOutPath</key>
	<string>/tmp/occmixer.log</string>
	<key>StandardErrorPath</key>
	<string>/tmp/occmixer.log</string>
</dict>
</plist>
`, Label, bin, layout, frames)

	if err := os.MkdirAll(filepath.Dir(plistPath()), 0o755); err != nil {
		return err
	}
	return os.WriteFile(plistPath(), []byte(body), 0o644)
}

func valid(layout string) bool { return slices.Contains(Layouts, layout) }

// LayoutRow maps a layout onto its index in Layouts, or 0.
func LayoutRow(layout string) int {
	for i, l := range Layouts {
		if l == layout {
			return i
		}
	}
	return 0
}

func firstLine(out []byte, err error) string {
	s := strings.TrimSpace(string(out))
	if s == "" {
		return err.Error()
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}
