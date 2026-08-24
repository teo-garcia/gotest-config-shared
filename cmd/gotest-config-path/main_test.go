package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPrintsBaselineToStdout(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if err := run(nil, &stdout, &stderr); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	out := stdout.String()
	for _, want := range []string{"covermode: atomic", "reporters:", "exclude:"} {
		if !strings.Contains(out, want) {
			t.Errorf("printed config is missing %q", want)
		}
	}
}

func TestRunArgsPrintsCanonicalGoTestFlags(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if err := run([]string{"--args"}, &stdout, &stderr); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	got := strings.TrimSpace(stdout.String())
	for _, want := range []string{"-race", "-covermode=atomic", "-coverprofile=coverage/coverage.out"} {
		if !strings.Contains(got, want) {
			t.Errorf("args %q is missing %q", got, want)
		}
	}
	if !strings.HasSuffix(got, "./...") {
		t.Errorf("args %q must end with the package pattern", got)
	}
}

func TestRunArgsRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	overlay := filepath.Join(dir, "overlay.yml")
	if err := os.WriteFile(overlay, []byte("coverage:\n  covermode: count\n"), 0o600); err != nil {
		t.Fatalf("seed overlay: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if err := run([]string{"--args", "--overlay", overlay}, &stdout, &stderr); err == nil {
		t.Fatal("run emitted flags for a config that undercounts coverage")
	}
}

func TestRunWritesFileAndEchoesPath(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "gotest.yml")
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--out", path}, &stdout, &stderr); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	if got := strings.TrimSpace(stdout.String()); got != path {
		t.Errorf("stdout = %q, want %q", got, path)
	}
	if _, err := os.ReadFile(path); err != nil {
		t.Fatalf("config was not written: %v", err)
	}
}

func TestRunTreatsMissingOverlayAsEmpty(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	args := []string{"--args", "--overlay", filepath.Join(t.TempDir(), "absent.yml")}

	if err := run(args, &stdout, &stderr); err != nil {
		t.Fatalf("missing overlay should be tolerated, got: %v", err)
	}
}

func TestRunFailsOnUnknownFlag(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if err := run([]string{"--nope"}, &stdout, &stderr); err == nil {
		t.Fatal("run accepted an unknown flag")
	}
}
