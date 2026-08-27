package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const agentLabel = "com.dappermint.occam"

// agentPlist keeps one occam watch running. It does not use launchd's IOKit
// matching, and that is deliberate.
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
const agentPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>watch</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
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

func agentPaths() (plist, logDir string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", agentLabel+".plist"),
		filepath.Join(home, "Library", "Logs", "occam"), nil
}

func newAgent() *cobra.Command {
	c := &cobra.Command{
		Use:   "agent",
		Short: "manage the launchd agent that re-applies the profile on attach",
	}
	c.AddCommand(newAgentInstall(), newAgentUninstall(), newAgentStatus())
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
				if binary, err = filepath.EvalSymlinks(exe); err != nil {
					return err
				}
			}
			// A `go run` binary lives in a temp dir that vanishes, so launchd
			// would fire at a path that no longer exists. --print is exempt,
			// it installs nothing.
			if !print && (strings.Contains(binary, os.TempDir()) || strings.Contains(binary, "/go-build")) {
				return fmt.Errorf("refusing to install %s, it is a go run temporary; "+
					"build first with `just install`, or pass --binary", binary)
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
				filepath.Join(logDir, "watch.log"),
				filepath.Join(logDir, "watch.err"))

			if print {
				fmt.Print(body)
				return nil
			}
			if err := os.WriteFile(plistPath, []byte(body), 0o644); err != nil {
				return err
			}

			// bootout first so a reinstall replaces cleanly. It fails when
			// nothing is loaded, which is fine.
			target := fmt.Sprintf("gui/%d/%s", os.Getuid(), agentLabel)
			_ = exec.Command("launchctl", "bootout", target).Run()
			out, err := exec.Command("launchctl", "bootstrap",
				fmt.Sprintf("gui/%d", os.Getuid()), plistPath).CombinedOutput()
			if err != nil {
				return fmt.Errorf("launchctl bootstrap: %w: %s", err, strings.TrimSpace(string(out)))
			}

			fmt.Println(styleTitle.Render("agent installed"))
			fmt.Printf("  %s %s\n", styleKey.Render(fmt.Sprintf("%-9s", "plist")), plistPath)
			fmt.Printf("  %s %s\n", styleKey.Render(fmt.Sprintf("%-9s", "binary")), binary)
			fmt.Printf("  %s %s\n", styleKey.Render(fmt.Sprintf("%-9s", "logs")), logDir)
			fmt.Println(styleDim.Render("\n  unplug and replug the dongle, then: tail ~/Library/Logs/occam/watch.log"))
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
