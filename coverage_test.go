package gotestconfig_test

import (
	"bytes"
	"strings"
	"testing"

	gotestconfig "github.com/teo-garcia/gotest-config-shared"
)

const sampleProfile = `mode: atomic
github.com/teo-garcia/app/internal/tasks/service.go:10.20,14.2 3 5
github.com/teo-garcia/app/internal/tasks/service.go:16.30,18.3 2 0
github.com/teo-garcia/app/cmd/api/main.go:12.13,40.2 20 1
github.com/teo-garcia/app/internal/docs/docs.go:5.10,9.2 4 0
`

func TestParseProfileReadsModeAndBlocks(t *testing.T) {
	t.Parallel()

	profile := mustParse(t, sampleProfile)

	if profile.Mode != "atomic" {
		t.Errorf("mode = %q, want atomic", profile.Mode)
	}
	if len(profile.Blocks) != 4 {
		t.Fatalf("parsed %d blocks, want 4", len(profile.Blocks))
	}

	first := profile.Blocks[0]
	if first.File != "github.com/teo-garcia/app/internal/tasks/service.go" {
		t.Errorf("file = %q", first.File)
	}
	if first.StartLine != 10 || first.StartCol != 20 || first.EndLine != 14 || first.EndCol != 2 {
		t.Errorf("position = %d.%d,%d.%d, want 10.20,14.2",
			first.StartLine, first.StartCol, first.EndLine, first.EndCol)
	}
	if first.NumStmts != 3 || first.Count != 5 {
		t.Errorf("numStmts/count = %d/%d, want 3/5", first.NumStmts, first.Count)
	}
}

func TestParseProfileRejectsMissingModeHeader(t *testing.T) {
	t.Parallel()

	_, err := gotestconfig.ParseProfile(strings.NewReader("a.go:1.1,2.2 1 1\n"))
	if err == nil {
		t.Fatal("parsed a profile with no mode header; that silently changes coverage math")
	}
}

func TestParseProfileRejectsMalformedBlock(t *testing.T) {
	t.Parallel()

	for name, line := range map[string]string{
		"missing count":    "a.go:1.1,2.2 1\n",
		"bad position":     "a.go:1x1,2.2 1 1\n",
		"no range":         "a.go:1.1 1 1\n",
		"non-numeric stmt": "a.go:1.1,2.2 x 1\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := gotestconfig.ParseProfile(strings.NewReader("mode: set\n" + line)); err == nil {
				t.Errorf("accepted malformed block %q", line)
			}
		})
	}
}

func TestFilterExcludesEntrypointsAndGeneratedCode(t *testing.T) {
	t.Parallel()

	profile := mustParse(t, sampleProfile)
	filtered := profile.Filter([]string{"cmd/**/main.go", "**/docs/docs.go"})

	if len(filtered.Blocks) != 2 {
		t.Fatalf("kept %d blocks, want 2", len(filtered.Blocks))
	}
	for _, block := range filtered.Blocks {
		if strings.Contains(block.File, "main.go") || strings.Contains(block.File, "docs.go") {
			t.Errorf("excluded file survived the filter: %s", block.File)
		}
	}
	// Filtering must not mutate the source profile.
	if len(profile.Blocks) != 4 {
		t.Errorf("Filter mutated the receiver: %d blocks left", len(profile.Blocks))
	}
}

func TestFilterPreservesMode(t *testing.T) {
	t.Parallel()

	filtered := mustParse(t, sampleProfile).Filter([]string{"**/*.go"})
	if filtered.Mode != "atomic" {
		t.Errorf("mode = %q after filtering, want atomic", filtered.Mode)
	}
}

func TestMatchesAny(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		file    string
		pattern string
		want    bool
	}{
		// `**` matches across any number of segments...
		{"doublestar spans segments", "github.com/o/r/a/b/c/main.go", "cmd/**/main.go", false},
		{"doublestar from module root", "github.com/o/r/cmd/api/main.go", "cmd/**/main.go", true},
		{"doublestar matches deep nesting", "github.com/o/r/cmd/a/b/c/main.go", "cmd/**/main.go", true},
		// ...including zero segments.
		{"doublestar matches zero segments", "github.com/o/r/cmd/main.go", "cmd/**/main.go", true},
		{"leading doublestar", "github.com/o/r/internal/x_test.go", "**/*_test.go", true},
		{"trailing doublestar", "github.com/o/r/internal/testdata/x.json", "**/testdata/**", true},
		{"single star does not span segments", "github.com/o/r/a/b/main.go", "a/*/main.go", true},
		{"single star stops at separator", "github.com/o/r/a/b/c/main.go", "a/*/main.go", false},
		{"exact suffix", "github.com/o/r/internal/tasks/service.go", "internal/tasks/service.go", true},
		{"non-match", "github.com/o/r/internal/tasks/service.go", "**/*.pb.go", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := gotestconfig.MatchesAny(tc.file, []string{tc.pattern}); got != tc.want {
				t.Errorf("MatchesAny(%q, %q) = %v, want %v", tc.file, tc.pattern, got, tc.want)
			}
		})
	}
}

func TestMatchesAnyWithNoPatterns(t *testing.T) {
	t.Parallel()

	if gotestconfig.MatchesAny("a/b.go", nil) {
		t.Error("MatchesAny with no patterns must never match")
	}
}

func TestSummarizeCountsStatementsNotBlocks(t *testing.T) {
	t.Parallel()

	// 3 covered + 2 uncovered + 20 covered + 4 uncovered = 23/29.
	got := mustParse(t, sampleProfile).Summarize()

	if got.TotalStatements != 29 {
		t.Errorf("total statements = %d, want 29", got.TotalStatements)
	}
	if got.CoveredStatements != 23 {
		t.Errorf("covered statements = %d, want 23", got.CoveredStatements)
	}
	if want := 23.0 / 29.0 * 100; got.Percent() != want {
		t.Errorf("percent = %v, want %v", got.Percent(), want)
	}
}

// An all-excluded profile has nothing uncovered in it. Reporting 0% would make
// that look like a total regression.
func TestSummarizeEmptyProfileReportsFullCoverage(t *testing.T) {
	t.Parallel()

	empty := mustParse(t, sampleProfile).Filter([]string{"**"})
	if got := empty.Summarize().Percent(); got != 100 {
		t.Errorf("percent of an empty profile = %v, want 100", got)
	}
}

func TestSummarizeByFileIsSortedAndPerFile(t *testing.T) {
	t.Parallel()

	files := mustParse(t, sampleProfile).SummarizeByFile()

	if len(files) != 3 {
		t.Fatalf("got %d files, want 3", len(files))
	}
	for i := 1; i < len(files); i++ {
		if files[i-1].File > files[i].File {
			t.Errorf("files are not sorted: %q before %q", files[i-1].File, files[i].File)
		}
	}

	var service gotestconfig.FileSummary
	for _, f := range files {
		if strings.HasSuffix(f.File, "service.go") {
			service = f
		}
	}
	if service.TotalStatements != 5 || service.CoveredStatements != 3 {
		t.Errorf("service.go = %d/%d, want 3/5",
			service.CoveredStatements, service.TotalStatements)
	}
}

func TestWriteTextIncludesEveryFileAndATotal(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := mustParse(t, sampleProfile).WriteText(&buf); err != nil {
		t.Fatalf("WriteText returned error: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"service.go", "main.go", "docs.go", "total:"} {
		if !strings.Contains(out, want) {
			t.Errorf("text report is missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "79.3%") {
		t.Errorf("text report is missing the 23/29 total:\n%s", out)
	}
}

func TestWriteLCOVEmitsValidRecords(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	profile := mustParse(t, "mode: atomic\napp/a.go:1.1,3.2 2 4\napp/a.go:5.1,6.2 1 0\n")
	if err := profile.WriteLCOV(&buf); err != nil {
		t.Fatalf("WriteLCOV returned error: %v", err)
	}

	out := buf.String()
	// Lines 1-3 hit 4 times, lines 5-6 never hit: 5 lines found, 3 hit.
	for _, want := range []string{
		"SF:app/a.go", "DA:1,4", "DA:2,4", "DA:3,4",
		"DA:5,0", "DA:6,0", "LF:5", "LH:3", "end_of_record",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("lcov output is missing %q:\n%s", want, out)
		}
	}
}

// Overlapping blocks are normal in Go profiles; a line reached by any executed
// block is covered, so the highest count must win.
func TestWriteLCOVKeepsHighestCountForOverlappingBlocks(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	profile := mustParse(t, "mode: atomic\napp/a.go:1.1,4.2 2 0\napp/a.go:2.1,3.2 1 7\n")
	if err := profile.WriteLCOV(&buf); err != nil {
		t.Fatalf("WriteLCOV returned error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "DA:2,7") || !strings.Contains(out, "DA:3,7") {
		t.Errorf("overlapping covered lines lost their hit count:\n%s", out)
	}
	if !strings.Contains(out, "DA:1,0") {
		t.Errorf("uncovered line was wrongly marked covered:\n%s", out)
	}
}

// The filtered profile is fed back to `go tool cover -html`, so the round trip
// has to be lossless.
func TestWriteProfileRoundTrips(t *testing.T) {
	t.Parallel()

	original := mustParse(t, sampleProfile)

	var buf bytes.Buffer
	if err := original.WriteProfile(&buf); err != nil {
		t.Fatalf("WriteProfile returned error: %v", err)
	}
	reparsed := mustParse(t, buf.String())

	if reparsed.Mode != original.Mode {
		t.Errorf("mode = %q after round trip, want %q", reparsed.Mode, original.Mode)
	}
	if len(reparsed.Blocks) != len(original.Blocks) {
		t.Fatalf("%d blocks after round trip, want %d", len(reparsed.Blocks), len(original.Blocks))
	}
	for i := range original.Blocks {
		if reparsed.Blocks[i] != original.Blocks[i] {
			t.Errorf("block %d = %+v, want %+v", i, reparsed.Blocks[i], original.Blocks[i])
		}
	}
}

func mustParse(t *testing.T, raw string) *gotestconfig.Profile {
	t.Helper()
	profile, err := gotestconfig.ParseProfile(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("ParseProfile returned error: %v", err)
	}
	return profile
}
