<div align="center">

# gotest-config-shared

**Shared `go test` and coverage configuration for teo-garcia Go templates**

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev)

Part of the [@teo-garcia/templates](https://github.com/teo-garcia/templates) ecosystem

</div>

---

## Why this exists

`go test` has no config file, and the toolchain is missing three things the
portfolio coverage contract requires:

| Gap | What this package does |
| --- | --- |
| No way to **exclude files** from a coverage profile | Filters the profile after collection using `**` globs |
| No **LCOV** output | Converts the profile to LCOV |
| No **threshold** check | Optional floor, defaulted off |

Lint and format settings live in the sibling `golangci-config-shared`.

---

## Settings

| Setting | Value |
| --- | --- |
| **Test flags** | `-race -shuffle=on -timeout=5m` |
| **covermode** | `atomic` (required with `-race`) |
| **Packages** | `./...` |
| **Coverage dir** | `coverage/` |
| **Profile** | `coverage/coverage.out` |
| **Reporters** | `text`, `lcov`, `html` — in that order |
| **Thresholds** | **none enforced** — deliberate |
| **Excluded** | tests, testdata, mocks, `*.gen.go`, `*_generated.go`, `*.pb.go`, `docs/docs.go`, `cmd/**/main.go`, migrations, seed |

Why no thresholds: a template ships a starter suite, so any floor would fail
every project generated from it on day one. If a floor is ever wanted, it has to
be added to every template in the same change, not one at a time.

`Validate()` rejects `covermode: set`/`count` while `-race` is on, because those
modes undercount concurrent hits and would silently report low coverage.

---

## Requirements

- Go 1.25+

---

## Usage

```bash
go get github.com/teo-garcia/gotest-config-shared
```

### Collect and report

```bash
go test $(gotest-config-path --args)
gotest-cover
```

`gotest-config-path --args` prints the canonical flags
(`-race -shuffle=on -timeout=5m -covermode=atomic -coverprofile=coverage/coverage.out ./...`),
and `gotest-cover` reads the resulting profile, drops excluded files, and writes:

```
coverage/coverage.filtered.out   profile after exclusions
coverage/lcov.info               lcov reporter
coverage/coverage.html           html reporter
                                 text reporter goes to stdout
```

### In a Makefile

```make
GOTEST_ARGS = $(shell go run github.com/teo-garcia/gotest-config-shared/cmd/gotest-config-path --args)

coverage:
	@mkdir -p coverage
	go test $(GOTEST_ARGS)
	go run github.com/teo-garcia/gotest-config-shared/cmd/gotest-cover
```

### Repo-specific overrides

Commit a `gotest.overrides.yml` with only what differs:

```yaml
coverage:
  exclude:
    - "**/*_test.go"
    - "internal/legacy/**"
```

```bash
gotest-cover --overlay gotest.overrides.yml
```

Merge semantics match `golangci-config-shared`: maps merge key-by-key at every
depth; scalars **and sequences** are replaced wholesale, so a consumer can
narrow `coverage.exclude`, not just grow it. A missing overlay file is treated
as empty.

### Exclusion patterns

Patterns match the profile's file path and any suffix of it, so
`cmd/**/main.go` works without knowing the module path. `**` matches zero or
more segments; `*`, `?`, and character classes behave as in `path.Match`.

### As a library

```go
import gotestconfig "github.com/teo-garcia/gotest-config-shared"

cfg, err := gotestconfig.Load(overlay)   // baseline + overlay
args := cfg.TestArgs()                   // canonical go test flags
err = cfg.Validate()

profile, err := gotestconfig.LoadProfile("coverage/coverage.out")
filtered := profile.Filter(cfg.Coverage.Exclude)
summary := filtered.Summarize()          // statement coverage
err = filtered.WriteLCOV(w)
```

---

## Scripts

| Command | Description |
| --- | --- |
| `make tools` | Install the pinned golangci-lint into `./bin` |
| `make lint-check` | Run linters |
| `make format` | Apply formatters |
| `make format-check` | Verify formatting without writing |
| `make test` | Run tests with `-race -shuffle=on` |
| `make coverage` | Self-hosted: collects with its own flags, reports with its own reporter |
| `make check` | Single CI gate: lint + format-check + test |

---

## Related Packages

| Package | Description |
| --- | --- |
| `golangci-config-shared` | golangci-lint settings — lint and format |
| `teo-pytest-config-shared` | pytest and coverage settings (Python) |
| `vitest-config-shared` | Vitest and coverage settings (TypeScript) |

---

## License

MIT © teo-garcia
