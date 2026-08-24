package gotestconfig_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	gotestconfig "github.com/teo-garcia/gotest-config-shared"
)

func TestBaselineMatchesThePortfolioCoverageContract(t *testing.T) {
	t.Parallel()

	cfg := mustLoad(t, nil)

	if got := cfg.Coverage.Dir; got != "coverage" {
		t.Errorf("coverage.dir = %q, want coverage/", got)
	}
	// Reporters must be text, lcov, html — in that order.
	want := []string{"text", "lcov", "html"}
	if len(cfg.Coverage.Reporters) != len(want) {
		t.Fatalf("reporters = %v, want %v", cfg.Coverage.Reporters, want)
	}
	for i := range want {
		if cfg.Coverage.Reporters[i] != want[i] {
			t.Errorf("reporters = %v, want %v (order is part of the contract)",
				cfg.Coverage.Reporters, want)
			break
		}
	}
	// No thresholds enforced: a floor would fail every generated project on day one.
	if cfg.Coverage.Threshold != 0 {
		t.Errorf("coverage.threshold = %v, want 0 (no enforced floor)", cfg.Coverage.Threshold)
	}
}

func TestBaselineUsesAtomicWithRace(t *testing.T) {
	t.Parallel()

	cfg := mustLoad(t, nil)

	if !cfg.Test.Race {
		t.Error("test.race is off; the portfolio baseline runs tests under -race")
	}
	if cfg.Coverage.CoverMode != "atomic" {
		t.Errorf("covermode = %q, want atomic (required with -race)", cfg.Coverage.CoverMode)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("baseline fails its own validation: %v", err)
	}
}

func TestBaselineExcludesGeneratedCodeAndEntrypoints(t *testing.T) {
	t.Parallel()

	excludes := mustLoad(t, nil).Coverage.Exclude

	shouldExclude := []string{
		"github.com/o/r/cmd/api/main.go",
		"github.com/o/r/internal/tasks/service_test.go",
		"github.com/o/r/internal/mocks/repo.go",
		"github.com/o/r/internal/api/api.pb.go",
		"github.com/o/r/internal/api/client.gen.go",
		"github.com/o/r/docs/docs.go",
		"github.com/o/r/internal/db/migrations/001.go",
	}
	for _, file := range shouldExclude {
		if !gotestconfig.MatchesAny(file, excludes) {
			t.Errorf("%s should be excluded from coverage but is not", file)
		}
	}

	shouldKeep := []string{
		"github.com/o/r/internal/tasks/service.go",
		"github.com/o/r/internal/tasks/repository.go",
		"github.com/o/r/internal/shared/middleware/requestid.go",
	}
	for _, file := range shouldKeep {
		if gotestconfig.MatchesAny(file, excludes) {
			t.Errorf("%s is application code and must stay in coverage", file)
		}
	}
}

func TestTestArgsRendersCanonicalFlags(t *testing.T) {
	t.Parallel()

	args := strings.Join(mustLoad(t, nil).TestArgs(), " ")

	for _, want := range []string{
		"-race", "-shuffle=on", "-timeout=5m",
		"-covermode=atomic", "-coverprofile=coverage/coverage.out", "./...",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("test args %q is missing %q", args, want)
		}
	}
	// Verbose is off by default: that is CI's choice, not the default loop's.
	if strings.Contains(args, "-v ") {
		t.Errorf("test args %q enables -v by default", args)
	}
}

func TestTestArgsPutsPackagesLast(t *testing.T) {
	t.Parallel()

	args := mustLoad(t, nil).TestArgs()

	if got := args[len(args)-1]; got != "./..." {
		t.Errorf("last arg = %q, want ./... (go test requires packages after flags)", got)
	}
}

func TestValidateRejectsNonAtomicModeUnderRace(t *testing.T) {
	t.Parallel()

	cfg := mustLoad(t, []byte("coverage:\n  covermode: set\n"))

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted covermode=set with -race; that silently undercounts coverage")
	}
}

func TestValidateRejectsUnknownReporter(t *testing.T) {
	t.Parallel()

	cfg := mustLoad(t, []byte("coverage:\n  reporters: [text, cobertura]\n"))

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted an unknown reporter")
	}
}

func TestValidateRejectsOutOfRangeThreshold(t *testing.T) {
	t.Parallel()

	cfg := mustLoad(t, []byte("coverage:\n  threshold: 140\n"))

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted a threshold above 100")
	}
}

func TestLoadAppliesOverlay(t *testing.T) {
	t.Parallel()

	cfg := mustLoad(t, []byte("test:\n  timeout: 90s\n  verbose: true\n"))

	if cfg.Test.Timeout != "90s" {
		t.Errorf("timeout = %q, want 90s", cfg.Test.Timeout)
	}
	if !cfg.Test.Verbose {
		t.Error("verbose override was not applied")
	}
	// Sibling keys survive the merge.
	if !cfg.Test.Race {
		t.Error("test.race was dropped by the overlay merge")
	}
}

func TestMergeReplacesExcludeListInsteadOfAppending(t *testing.T) {
	t.Parallel()

	cfg := mustLoad(t, []byte("coverage:\n  exclude:\n    - \"**/only.go\"\n"))

	if len(cfg.Coverage.Exclude) != 1 || cfg.Coverage.Exclude[0] != "**/only.go" {
		t.Errorf("exclude = %v, want exactly [**/only.go]; consumers must be able to narrow it",
			cfg.Coverage.Exclude)
	}
}

func TestBaselineReturnsACopy(t *testing.T) {
	t.Parallel()

	first := gotestconfig.Baseline()
	first[0] = 'X'

	if second := gotestconfig.Baseline(); second[0] == 'X' {
		t.Fatal("Baseline() leaked its backing array to the caller")
	}
}

func TestWriteEmitsGeneratedHeader(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", "gotest.yml")
	if err := gotestconfig.Write(path, nil); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("generated config unreadable: %v", err)
	}
	if !strings.HasPrefix(string(data), "# Generated by gotest-config-shared") {
		t.Error("generated config is missing the do-not-edit header")
	}
}

func TestLoadRejectsMalformedOverlay(t *testing.T) {
	t.Parallel()

	if _, err := gotestconfig.Load([]byte("test:\n\trace: true\n")); err == nil {
		t.Fatal("Load accepted malformed YAML")
	}
}

func mustLoad(t *testing.T, overlay []byte) gotestconfig.Config {
	t.Helper()
	cfg, err := gotestconfig.Load(overlay)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	return cfg
}

// Without -coverpkg, Go credits a package only for tests inside it, so a
// layered service under-reports its real coverage.
func TestTestArgsIncludesCoverPkg(t *testing.T) {
	t.Parallel()

	args := strings.Join(mustLoad(t, nil).TestArgs(), " ")

	if !strings.Contains(args, "-coverpkg=./...") {
		t.Errorf("test args %q is missing -coverpkg", args)
	}
}
