package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const agentLabel = "com.dappermint.occam"

// agentPlist keeps one occam menu running. The menu bar app does everything
// occam watch does, so there is no reason to run both.
//
// It does not use launchd's IOKit matching, and that is deliberate.
//
// LaunchEvents with com.apple.iokit.matching requires IOMatchLaunchStream,
// which hands the job an XPC event stream it is expected to drain with
// xpc_set_event_stream_handler. A job that does not drain it leaves the event
// pending, so launchd relaunches immediately and forever: measured here at one
// relaunch every eleven seconds. Draining it means becoming a long-lived XPC
// daemon, at which point run-once has bought nothing.
//
// So: one long-lived process that blocks on an in-process IOKit attach
// notification. It is asleep until the kernel posts an arrival.
//
// KeepAlive is deliberately absent: quitting from the menu should quit, not
// get the process resurrected a second later. RunAtLoad starts it at login.
const agentPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>menu</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>ThrottleInterval</key>
	<integer>10</integer>
	<key>ProcessType</key>
	<string>Background</string>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`

// isThrowaway reports whether a path is a `go run` build, which lives in a
// temp directory that vanishes and would leave launchd firing at nothing.
func isThrowaway(path string) bool {
	return strings.Contains(path, os.TempDir()) || strings.Contains(path, "/go-build")
}

func agentPaths() (plist, logDir string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", agentLabel+".plist"),
		filepath.Join(home, "Library", "Logs", "occam"), nil
}

func agentDomain() string { return fmt.Sprintf("gui/%d", os.Getuid()) }

// loadAgent replaces whatever launchd currently holds for the label with the
// plist on disk.
//
// The bootout is what makes an upgrade work. The plist names the stable
// /opt/homebrew/bin symlink, but launchd resolves it once at bootstrap and
// pins the job to that Cellar inode, so `brew cleanup` deleting the old
// version leaves the loaded job pointing at a binary that no longer exists.
// It does not fail with ENOENT: the replacement fails AMFI's launch
// constraint check instead, and the job dies in 7ms with
// OS_REASON_CODESIGNING until something re-bootstraps it.
//
// Bootout fails when nothing is loaded, which is fine. It is not instant
// though, and bootstrapping over a job still going down fails with
// "Bootstrap failed: 5: Input/output error", so retry briefly.
func loadAgent(plistPath string) error {
	domain := agentDomain()
	_ = exec.Command("launchctl", "bootout", domain+"/"+agentLabel).Run()

	var out []byte
	var err error
	for attempt := 1; attempt <= 5; attempt++ {
		out, err = exec.Command("launchctl", "bootstrap", domain, plistPath).CombinedOutput()
		if err == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("launchctl bootstrap: %w: %s", err, strings.TrimSpace(string(out)))
}

func newAgent() *cobra.Command {
	c := &cobra.Command{
		Use:   "agent",
		Short: "manage the launchd agent that re-applies the profile on attach",
	}
	c.AddCommand(newAgentInstall(), newAgentUninstall(), newAgentStatus(), newAgentRepair())
	return c
}

func newAgentInstall() *cobra.Command {
	var binary string
	var print bool

	c := &cobra.Command{
		Use:   "install",
		Short: "write and load the launchd agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			if binary == "" {
				exe, err := os.Executable()
				if err != nil {
					return err
				}
				// Resolve only to decide whether this is a throwaway build.
				// The unresolved path is the one worth recording: under
				// Homebrew it is the stable /opt/homebrew/bin symlink, while
				// the resolved one points into a versioned Cellar directory
				// that disappears on the next upgrade.
				resolved, err := filepath.EvalSymlinks(exe)
				if err != nil {
					resolved = exe
				}
				if isThrowaway(resolved) {
					return fmt.Errorf("refusing to install %s, it is a go run temporary; "+
						"build first with `just install`, or pass --binary", resolved)
				}
				binary = exe
			}
			plistPath, logDir, err := agentPaths()
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
				return err
			}
			if err := os.MkdirAll(logDir, 0o755); err != nil {
				return err
			}

			body := fmt.Sprintf(agentPlist,
				agentLabel, binary,
				filepath.Join(logDir, "occam.log"),
				filepath.Join(logDir, "occam.err"))

			if print {
				fmt.Print(body)
				return nil
			}
			if err := os.WriteFile(plistPath, []byte(body), 0o644); err != nil {
				return err
			}

			// bootout first so a reinstall replaces cleanly.
			if err := loadAgent(plistPath); err != nil {
				return err
			}

			fmt.Println(styleTitle.Render("agent installed"))
			fmt.Printf("  %s %s\n", styleKey.Render(fmt.Sprintf("%-9s", "plist")), plistPath)
			fmt.Printf("  %s %s\n", styleKey.Render(fmt.Sprintf("%-9s", "binary")), binary)
			fmt.Printf("  %s %s\n", styleKey.Render(fmt.Sprintf("%-9s", "logs")), logDir)
			fmt.Println(styleDim.Render("\n  the headphones icon is in the menu bar, and it starts at login"))
			return nil
		},
	}
	c.Flags().StringVar(&binary, "binary", "", "path to the occam binary, defaults to this one")
	c.Flags().BoolVar(&print, "print", false, "print the plist instead of installing it")
	return c
}

func newAgentUninstall() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "unload and remove the launchd agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			plistPath, _, err := agentPaths()
			if err != nil {
				return err
			}
			target := fmt.Sprintf("gui/%d/%s", os.Getuid(), agentLabel)
			_ = exec.Command("launchctl", "bootout", target).Run()
			if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
				return err
			}
			fmt.Println(styleHit.Render("agent removed"))
			return nil
		},
	}
}

func newAgentStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "show whether the agent is installed and loaded",
		RunE: func(cmd *cobra.Command, args []string) error {
			plistPath, logDir, err := agentPaths()
			if err != nil {
				return err
			}

			state := "not installed"
			if _, err := os.Stat(plistPath); err == nil {
				state = "installed"
			}
			loaded := "no"
			if err := exec.Command("launchctl", "print",
				fmt.Sprintf("gui/%d/%s", os.Getuid(), agentLabel)).Run(); err == nil {
				loaded = "yes"
			}

			fmt.Printf("  %s %s\n", styleKey.Render(fmt.Sprintf("%-9s", "plist")), state)
			fmt.Printf("  %s %s\n", styleKey.Render(fmt.Sprintf("%-9s", "loaded")), loaded)
			fmt.Printf("  %s %s\n", styleKey.Render(fmt.Sprintf("%-9s", "logs")), logDir)
			return nil
		},
	}
}

// newAgentRepair re-bootstraps an agent that is already installed, so an
// upgrade can hand launchd the new binary. It never writes a plist: installing
// the agent stays an explicit choice, and this has to be safe to run from a
// package manager on a machine that never wanted one.
func newAgentRepair() *cobra.Command {
	return &cobra.Command{
		Use:   "repair",
		Short: "re-load an already installed agent, for after an upgrade",
		RunE: func(cmd *cobra.Command, args []string) error {
			plistPath, _, err := agentPaths()
			if err != nil {
				return err
			}
			if _, err := os.Stat(plistPath); err != nil {
				if os.IsNotExist(err) {
					fmt.Println(styleDim.Render("no agent installed, nothing to repair"))
					return nil
				}
				return err
			}
			if err := loadAgent(plistPath); err != nil {
				return err
			}
			fmt.Println(styleTitle.Render("agent reloaded"))
			return nil
		},
	}
}
