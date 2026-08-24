// Command gotest-cover turns a raw `go test -coverprofile` file into the
// portfolio coverage reports.
//
// It applies the exclusion patterns Go itself cannot express, then emits the
// text, lcov, and html reporters named in the shared config.
//
//	go test -race -covermode=atomic -coverprofile=coverage/coverage.out ./...
//	gotest-cover
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	gotestconfig "github.com/teo-garcia/gotest-config-shared"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "gotest-cover:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("gotest-cover", flag.ContinueOnError)
	flags.SetOutput(stderr)
	overlay := flags.String("overlay", "", "optional repo-local YAML merged onto the shared baseline")
	profilePath := flags.String("profile", "", "coverage profile to read (default: config value)")
	skipHTML := flags.Bool("no-html", false, "skip the html reporter (it shells out to `go tool cover`)")
	if err := flags.Parse(args); err != nil {
		return err
	}

	overlayData, err := readOverlay(*overlay)
	if err != nil {
		return err
	}
	cfg, err := gotestconfig.Load(overlayData)
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	source := cfg.Coverage.Profile
	if *profilePath != "" {
		source = *profilePath
	}
	profile, err := gotestconfig.LoadProfile(source)
	if err != nil {
		return err
	}

	filtered := profile.Filter(cfg.Coverage.Exclude)
	excluded := len(profile.Blocks) - len(filtered.Blocks)
	fmt.Fprintf(stderr, "gotest-cover: %d blocks, %d excluded by %d patterns\n",
		len(profile.Blocks), excluded, len(cfg.Coverage.Exclude))

	if err := os.MkdirAll(cfg.Coverage.Dir, 0o755); err != nil {
		return fmt.Errorf("create coverage directory: %w", err)
	}

	// `go tool cover -html` needs the filtered profile on disk, and keeping the
	// filtered profile is also what makes the reported number reproducible.
	filteredPath := filepath.Join(cfg.Coverage.Dir, "coverage.filtered.out")
	if err := writeFile(filteredPath, filtered.WriteProfile); err != nil {
		return err
	}

	// Reporters run in the order the config lists them.
	for _, reporter := range cfg.Coverage.Reporters {
		switch reporter {
		case "text":
			if err := filtered.WriteText(stdout); err != nil {
				return err
			}
		case "lcov":
			if err := writeFile(cfg.Coverage.LCOV, filtered.WriteLCOV); err != nil {
				return err
			}
		case "html":
			if *skipHTML {
				continue
			}
			// Both paths come from the repo's own config file, not from user
			// input, and the binary and subcommand are fixed literals.
			cmd := exec.CommandContext(ctx, "go", "tool", "cover", //nolint:gosec // args are config-owned, not user input
				"-html="+filteredPath, "-o="+cfg.Coverage.HTML)
			cmd.Stderr = stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("render html report: %w", err)
			}
		}
	}

	summary := filtered.Summarize()
	fmt.Fprintf(stderr, "gotest-cover: total %.1f%% (%d/%d statements)\n",
		summary.Percent(), summary.CoveredStatements, summary.TotalStatements)

	// A zero threshold means "no floor enforced", which is the portfolio
	// default: a starter suite must not fail every generated project on day one.
	if cfg.Coverage.Threshold > 0 && summary.Percent() < cfg.Coverage.Threshold {
		return fmt.Errorf("coverage %.1f%% is below the configured threshold of %.1f%%",
			summary.Percent(), cfg.Coverage.Threshold)
	}
	return nil
}

func writeFile(path string, render func(io.Writer) error) error {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	if err := render(f); err != nil {
		return err
	}
	return nil
}

func readOverlay(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read overlay %s: %w", path, err)
	}
	return data, nil
}
