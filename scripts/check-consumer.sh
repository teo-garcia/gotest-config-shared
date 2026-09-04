#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
tmp_root=$(mktemp -d "${TMPDIR:-/tmp}/gotest-config-consumer.XXXXXX")
export GOCACHE="$tmp_root/go-build-cache"
export GOLANGCI_LINT_CACHE="$tmp_root/golangci-lint-cache"

cleanup() {
	rm -rf -- "$tmp_root"
}
trap cleanup EXIT HUP INT TERM

cp -R "$repo_root/testdata/consumer" "$tmp_root/consumer"
cd "$tmp_root/consumer"

go mod edit -replace "github.com/teo-garcia/gotest-config-shared=$repo_root"
go get github.com/teo-garcia/gotest-config-shared@v0.0.0
test_args=$(go run github.com/teo-garcia/gotest-config-shared/cmd/gotest-config-path --args)
mkdir -p coverage
# The config command intentionally emits a shell-ready list of Go flags.
# shellcheck disable=SC2086
go test $test_args
go run github.com/teo-garcia/gotest-config-shared/cmd/gotest-cover

test -s coverage/coverage.filtered.out
test -s coverage/lcov.info
test -s coverage/coverage.html
grep -q 'SF:example.com/gotest-config-consumer/math.go' coverage/lcov.info
if grep -q '/cmd/demo/main.go' coverage/lcov.info; then
	echo "consumer report included the configured cmd/**/main.go exclusion" >&2
	exit 1
fi

if go run github.com/teo-garcia/gotest-config-shared/cmd/gotest-cover \
	--overlay threshold.yml >/dev/null 2>&1; then
	echo "consumer reporter unexpectedly accepted coverage below its threshold" >&2
	exit 1
fi
