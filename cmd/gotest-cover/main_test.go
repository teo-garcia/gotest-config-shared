package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const profile = `mode: atomic
github.com/teo-garcia/app/internal/tasks/service.go:10.20,14.2 3 5
github.com/teo-garcia/app/internal/tasks/service.go:16.30,18.3 2 0
github.com/teo-garcia/app/cmd/api/main.go:12.13,40.2 20 1
`

// seed writes a profile plus an overlay that redirects every output into a
// temp dir, and chdirs there so relative config paths resolve.
func seed(t *testing.T, extraOverlay string) (dir, overlayPath string) {
	t.Helper()

	dir = t.TempDir()
	t.Chdir(dir)

	if err := os.MkdirAll("coverage", 0o755); err != nil {
		t.Fatalf("create coverage dir: %v", err)
	}
	if err := os.WriteFile("coverage/coverage.out", []byte(profile), 0o600); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	overlayPath = filepath.Join(dir, "overlay.yml")
	if err := os.WriteFile(overlayPath, []byte(extraOverlay), 0o600); err != nil {
		t.Fatalf("seed overlay: %v", err)
	}
	return dir, overlayPath
}

func TestRunAppliesExclusionsToTheReportedTotal(t *testing.T) {
	_, overlay := seed(t, "coverage:\n  exclude:\n    - \"cmd/**/main.go\"\n")

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"--overlay", overlay, "--no-html"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stderr.String())
	}

	// main.go (20/20) is excluded, leaving service.go at 3/5 = 60%.
	if !strings.Contains(stdout.String(), "60.0%") {
		t.Errorf("text report should total 60%% after excluding main.go:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "main.go") {
		t.Errorf("excluded file appears in the report:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "1 excluded") {
		t.Errorf("stderr should report the exclusion count:\n%s", stderr.String())
	}
}

func TestRunWritesLcovAndFilteredProfile(t *testing.T) {
	_, overlay := seed(t, "coverage:\n  exclude:\n    - \"cmd/**/main.go\"\n")

	var stdout, stderr bytes.Buffer
	if err := run(t.Context(), []string{"--overlay", overlay, "--no-html"}, &stdout, &stderr); err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stderr.String())
	}

	lcov, err := os.ReadFile("coverage/lcov.info")
	if err != nil {
		t.Fatalf("lcov report was not written: %v", err)
	}
	if !strings.Contains(string(lcov), "SF:github.com/teo-garcia/app/internal/tasks/service.go") {
		t.Errorf("lcov is missing the covered source file:\n%s", lcov)
	}
	if strings.Contains(string(lcov), "main.go") {
		t.Errorf("lcov contains an excluded file:\n%s", lcov)
	}

	filtered, err := os.ReadFile("coverage/coverage.filtered.out")
	if err != nil {
		t.Fatalf("filtered profile was not written: %v", err)
	}
	if !strings.HasPrefix(string(filtered), "mode: atomic") {
		t.Errorf("filtered profile lost its mode header:\n%s", filtered)
	}
}

// A zero threshold is the portfolio default and must never fail the build.
func TestRunSucceedsWithNoThresholdEvenAtLowCoverage(t *testing.T) {
	_, overlay := seed(t, "coverage:\n  exclude: []\n")

	var stdout, stderr bytes.Buffer
	if err := run(t.Context(), []string{"--overlay", overlay, "--no-html"}, &stdout, &stderr); err != nil {
		t.Fatalf("zero threshold must not fail the build, got: %v", err)
	}
}

func TestRunFailsWhenAConfiguredThresholdIsMissed(t *testing.T) {
	_, overlay := seed(t, "coverage:\n  threshold: 95\n  exclude: []\n")

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"--overlay", overlay, "--no-html"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("run passed despite coverage below the configured threshold")
	}
	if !strings.Contains(err.Error(), "below the configured threshold") {
		t.Errorf("error = %v, want a threshold message", err)
	}
}

func TestRunFailsOnMissingProfile(t *testing.T) {
	dir, overlay := seed(t, "")
	if err := os.Remove(filepath.Join(dir, "coverage", "coverage.out")); err != nil {
		t.Fatalf("remove profile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if err := run(t.Context(), []string{"--overlay", overlay, "--no-html"}, &stdout, &stderr); err == nil {
		t.Fatal("run succeeded with no coverage profile on disk")
	}
}

func TestRunRejectsInvalidConfig(t *testing.T) {
	_, overlay := seed(t, "coverage:\n  covermode: set\n")

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"--overlay", overlay, "--no-html"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("run accepted covermode=set with -race enabled")
	}
}

func TestRunHonoursExplicitProfileFlag(t *testing.T) {
	dir, overlay := seed(t, "coverage:\n  exclude: []\n")

	alt := filepath.Join(dir, "alt.out")
	if err := os.WriteFile(alt, []byte("mode: atomic\napp/x.go:1.1,2.2 4 4\n"), 0o600); err != nil {
		t.Fatalf("seed alternate profile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"--overlay", overlay, "--profile", alt, "--no-html"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "app/x.go") {
		t.Errorf("report did not read the --profile file:\n%s", stdout.String())
	}
}
