package gotestconfig

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Block is one coverage block from a `go test -coverprofile` file.
//
// The profile line format is:
//
//	<file>:<startLine>.<startCol>,<endLine>.<endCol> <numStmts> <count>
type Block struct {
	File      string
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
	NumStmts  int
	Count     int
}

// Profile is a parsed coverage profile.
type Profile struct {
	Mode   string
	Blocks []Block
}

// ParseProfile reads a coverage profile in the standard Go format.
func ParseProfile(r io.Reader) (*Profile, error) {
	scanner := bufio.NewScanner(r)
	// Coverage lines are short, but generated files can produce long paths.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	profile := &Profile{}
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if after, ok := strings.CutPrefix(line, "mode:"); ok {
			profile.Mode = strings.TrimSpace(after)
			continue
		}
		block, err := parseBlock(line)
		if err != nil {
			return nil, fmt.Errorf("profile line %d: %w", lineNo, err)
		}
		profile.Blocks = append(profile.Blocks, block)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read profile: %w", err)
	}
	if profile.Mode == "" {
		return nil, fmt.Errorf("profile is missing its `mode:` header")
	}
	return profile, nil
}

func parseBlock(line string) (Block, error) {
	// Split off the trailing "<numStmts> <count>" first: the file path itself
	// may contain colons on some platforms, but never spaces in practice.
	fields := strings.Fields(line)
	if len(fields) != 3 {
		return Block{}, fmt.Errorf("want 3 fields, got %d in %q", len(fields), line)
	}

	numStmts, err := strconv.Atoi(fields[1])
	if err != nil {
		return Block{}, fmt.Errorf("statement count %q: %w", fields[1], err)
	}
	count, err := strconv.Atoi(fields[2])
	if err != nil {
		return Block{}, fmt.Errorf("hit count %q: %w", fields[2], err)
	}

	idx := strings.LastIndex(fields[0], ":")
	if idx < 0 {
		return Block{}, fmt.Errorf("missing position separator in %q", fields[0])
	}
	file, positions := fields[0][:idx], fields[0][idx+1:]

	start, end, ok := strings.Cut(positions, ",")
	if !ok {
		return Block{}, fmt.Errorf("missing range separator in %q", positions)
	}
	startLine, startCol, err := parsePosition(start)
	if err != nil {
		return Block{}, err
	}
	endLine, endCol, err := parsePosition(end)
	if err != nil {
		return Block{}, err
	}

	return Block{
		File:      file,
		StartLine: startLine,
		StartCol:  startCol,
		EndLine:   endLine,
		EndCol:    endCol,
		NumStmts:  numStmts,
		Count:     count,
	}, nil
}

func parsePosition(s string) (line, col int, err error) {
	lineStr, colStr, ok := strings.Cut(s, ".")
	if !ok {
		return 0, 0, fmt.Errorf("malformed position %q", s)
	}
	if line, err = strconv.Atoi(lineStr); err != nil {
		return 0, 0, fmt.Errorf("position line %q: %w", lineStr, err)
	}
	if col, err = strconv.Atoi(colStr); err != nil {
		return 0, 0, fmt.Errorf("position column %q: %w", colStr, err)
	}
	return line, col, nil
}

// Filter removes every block whose file matches one of the exclude patterns.
//
// This is the gap the package exists to close: `go test` has no way to keep
// generated code, mocks, or entrypoints out of a coverage profile, so the
// portfolio coverage contract has to be applied after collection.
//
// Patterns match against the file path with the module prefix stripped, so
// `cmd/**/main.go` works regardless of the module name.
func (p *Profile) Filter(patterns []string) *Profile {
	out := &Profile{Mode: p.Mode, Blocks: make([]Block, 0, len(p.Blocks))}
	for _, block := range p.Blocks {
		if !MatchesAny(block.File, patterns) {
			out.Blocks = append(out.Blocks, block)
		}
	}
	return out
}

// MatchesAny reports whether file matches any of the patterns.
func MatchesAny(file string, patterns []string) bool {
	candidates := pathCandidates(file)
	for _, pattern := range patterns {
		for _, candidate := range candidates {
			if matchGlob(pattern, candidate) {
				return true
			}
		}
	}
	return false
}

// pathCandidates returns the forms a pattern may be written against: the full
// profile path (`github.com/org/repo/cmd/api/main.go`) and every suffix of it.
// Matching suffixes is what lets `cmd/**/main.go` work without the author
// having to know the module path.
func pathCandidates(file string) []string {
	file = filepath.ToSlash(file)
	segments := strings.Split(file, "/")
	out := make([]string, 0, len(segments))
	for i := range segments {
		out = append(out, strings.Join(segments[i:], "/"))
	}
	return out
}

// matchGlob matches a `**`-aware glob against a slash-separated path.
//
// `**` matches zero or more path segments; every other segment is matched with
// path.Match, so `*`, `?`, and character classes behave as usual.
func matchGlob(pattern, name string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchSegments(pattern, name []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			// Trailing `**` swallows whatever is left, including nothing.
			if len(pattern) == 1 {
				return true
			}
			// Try to consume 0..n segments and match the rest of the pattern.
			for i := 0; i <= len(name); i++ {
				if matchSegments(pattern[1:], name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		ok, err := path.Match(pattern[0], name[0])
		if err != nil || !ok {
			return false
		}
		pattern, name = pattern[1:], name[1:]
	}
	return len(name) == 0
}

// Summary is the aggregate coverage of a profile.
type Summary struct {
	TotalStatements   int
	CoveredStatements int
}

// Percent returns statement coverage as a percentage.
//
// An empty profile is reported as 100%: there is nothing uncovered in it, and
// reporting 0% would make an all-excluded profile look like a regression.
func (s Summary) Percent() float64 {
	if s.TotalStatements == 0 {
		return 100
	}
	return float64(s.CoveredStatements) / float64(s.TotalStatements) * 100
}

// Summarize aggregates statement coverage across the whole profile.
func (p *Profile) Summarize() Summary {
	var s Summary
	for _, block := range p.Blocks {
		s.TotalStatements += block.NumStmts
		if block.Count > 0 {
			s.CoveredStatements += block.NumStmts
		}
	}
	return s
}

// FileSummary is per-file coverage, used by the text reporter.
type FileSummary struct {
	File string
	Summary
}

// SummarizeByFile aggregates coverage per file, sorted by path.
func (p *Profile) SummarizeByFile() []FileSummary {
	byFile := map[string]Summary{}
	for _, block := range p.Blocks {
		s := byFile[block.File]
		s.TotalStatements += block.NumStmts
		if block.Count > 0 {
			s.CoveredStatements += block.NumStmts
		}
		byFile[block.File] = s
	}

	out := make([]FileSummary, 0, len(byFile))
	for file, summary := range byFile {
		out = append(out, FileSummary{File: file, Summary: summary})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].File < out[j].File })
	return out
}

// WriteProfile serializes the profile back to the standard Go format so
// `go tool cover` can consume the filtered result.
func (p *Profile) WriteProfile(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "mode: %s\n", p.Mode); err != nil {
		return fmt.Errorf("write profile mode: %w", err)
	}
	for _, b := range p.Blocks {
		_, err := fmt.Fprintf(w, "%s:%d.%d,%d.%d %d %d\n",
			b.File, b.StartLine, b.StartCol, b.EndLine, b.EndCol, b.NumStmts, b.Count)
		if err != nil {
			return fmt.Errorf("write profile block: %w", err)
		}
	}
	return nil
}

// WriteText renders the `text` reporter: a per-file table plus a total.
func (p *Profile) WriteText(w io.Writer) error {
	files := p.SummarizeByFile()
	width := len("total:")
	for _, f := range files {
		if len(f.File) > width {
			width = len(f.File)
		}
	}

	for _, f := range files {
		_, err := fmt.Fprintf(w, "%-*s  %6.1f%%  (%d/%d)\n",
			width, f.File, f.Percent(), f.CoveredStatements, f.TotalStatements)
		if err != nil {
			return fmt.Errorf("write text report: %w", err)
		}
	}

	total := p.Summarize()
	_, err := fmt.Fprintf(w, "%-*s  %6.1f%%  (%d/%d)\n",
		width, "total:", total.Percent(), total.CoveredStatements, total.TotalStatements)
	if err != nil {
		return fmt.Errorf("write text report total: %w", err)
	}
	return nil
}

// WriteLCOV renders the `lcov` reporter.
//
// Go emits no LCOV, but the portfolio coverage contract requires it, and it is
// what external coverage services consume. Go coverage is statement-based
// while LCOV is line-based, so every line spanned by a block inherits that
// block's hit count — the same approximation gcov2lcov makes.
func (p *Profile) WriteLCOV(w io.Writer) error {
	byFile := map[string]map[int]int{}
	order := make([]string, 0)
	for _, b := range p.Blocks {
		lines, ok := byFile[b.File]
		if !ok {
			lines = map[int]int{}
			byFile[b.File] = lines
			order = append(order, b.File)
		}
		for line := b.StartLine; line <= b.EndLine; line++ {
			// Overlapping blocks keep the highest count: a line reached by any
			// executed block is covered.
			if existing, seen := lines[line]; !seen || b.Count > existing {
				lines[line] = b.Count
			}
		}
	}
	sort.Strings(order)

	for _, file := range order {
		lines := byFile[file]
		numbers := make([]int, 0, len(lines))
		for line := range lines {
			numbers = append(numbers, line)
		}
		sort.Ints(numbers)

		if _, err := fmt.Fprintf(w, "TN:\nSF:%s\n", file); err != nil {
			return fmt.Errorf("write lcov header: %w", err)
		}
		hit := 0
		for _, line := range numbers {
			count := lines[line]
			if count > 0 {
				hit++
			}
			if _, err := fmt.Fprintf(w, "DA:%d,%d\n", line, count); err != nil {
				return fmt.Errorf("write lcov line: %w", err)
			}
		}
		if _, err := fmt.Fprintf(w, "LF:%d\nLH:%d\nend_of_record\n", len(numbers), hit); err != nil {
			return fmt.Errorf("write lcov footer: %w", err)
		}
	}
	return nil
}

// LoadProfile reads and parses a coverage profile from disk.
func LoadProfile(name string) (*Profile, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open coverage profile: %w", err)
	}
	defer f.Close()
	return ParseProfile(f)
}
