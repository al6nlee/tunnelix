//go:build darwin && cgo && !purego

package mactray

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the actual pinned Wails Objective-C controller in a separate AppKit
// process, so native main-thread requirements and status items stay isolated.
func TestNativeMenuLifecycle(t *testing.T) {
	module, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/wailsapp/wails/v3").Output()
	if err != nil {
		t.Fatal(err)
	}
	application := filepath.Join(strings.TrimSpace(string(module)), "pkg", "application")
	binary := filepath.Join(t.TempDir(), "native-tray-test")
	cmd := exec.Command("clang", "-framework", "Cocoa", "-I", application,
		"testdata/menu_lifecycle.m", "menu_darwin.m", filepath.Join(application, "systemtray_darwin.m"), "-o", binary)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile native regression test: %v\n%s", err, output)
	}
	if output, err := exec.Command(binary).CombinedOutput(); err != nil {
		t.Fatalf("native menu lifecycle: %v\n%s", err, output)
	}
}
