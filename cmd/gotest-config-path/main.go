// Command gotest-config-path exposes the shared Go test configuration to a
// consumer repo.
//
//	gotest-config-path                        # print the baseline YAML
//	gotest-config-path --out gotest.yml       # write the merged config
//	gotest-config-path --args                 # print canonical `go test` flags
//	gotest-config-path --args --overlay o.yml # flags with repo overrides
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	gotestconfig "github.com/teo-garcia/gotest-config-shared"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "gotest-config-path:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("gotest-config-path", flag.ContinueOnError)
	flags.SetOutput(stderr)
	out := flags.String("out", "", "write the merged config to this path instead of stdout")
	overlay := flags.String("overlay", "", "optional repo-local YAML merged onto the shared baseline")
	printArgs := flags.Bool("args", false, "print the canonical `go test` flags instead of the config")
	if err := flags.Parse(args); err != nil {
		return err
	}

	overlayData, err := readOverlay(*overlay)
	if err != nil {
		return err
	}

	if *printArgs {
		cfg, err := gotestconfig.Load(overlayData)
		if err != nil {
			return err
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, strings.Join(cfg.TestArgs(), " "))
		return err
	}

	if *out == "" {
		merged, err := gotestconfig.Merge(overlayData)
		if err != nil {
			return err
		}
		_, err = stdout.Write(merged)
		return err
	}

	if err := gotestconfig.Write(*out, overlayData); err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, *out)
	return err
}

// readOverlay loads the overlay file. A missing overlay is not an error: the
// template ships without one, and `make test` must still work before a
// consumer decides it needs overrides.
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
