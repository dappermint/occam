package cmd

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentPlistIsWellFormed(t *testing.T) {
	body := fmt.Sprintf(agentPlist, agentLabel, "/opt/homebrew/bin/occam",
		"/tmp/occam.log", "/tmp/occam.err")

	if err := xml.Unmarshal([]byte(body), new(struct {
		XMLName xml.Name `xml:"plist"`
	})); err != nil {
		t.Fatalf("plist does not parse: %v", err)
	}
	for _, want := range []string{agentLabel, "/opt/homebrew/bin/occam", "<string>menu</string>"} {
		if !strings.Contains(body, want) {
			t.Errorf("plist is missing %q", want)
		}
	}
	// KeepAlive would resurrect the process after quitting from the menu.
	if strings.Contains(body, "KeepAlive") {
		t.Error("plist sets KeepAlive")
	}
}

// The Homebrew post_install runs repair on every machine, including ones that
// never installed an agent, so a missing plist has to succeed quietly rather
// than bootstrap something nobody asked for.
func TestAgentRepairWithoutPlistIsANoOp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	plistPath, _, err := agentPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := newAgentRepair().RunE(nil, nil); err != nil {
		t.Fatalf("repair failed on a clean machine: %v", err)
	}
	if _, err := os.Stat(plistPath); !os.IsNotExist(err) {
		t.Error("repair created a plist")
	}
}

func TestIsThrowaway(t *testing.T) {
	if !isThrowaway(filepath.Join(os.TempDir(), "go-build123", "occam")) {
		t.Error("a go run temporary was not recognised")
	}
	if isThrowaway("/opt/homebrew/bin/occam") {
		t.Error("an installed binary was called a temporary")
	}
}
