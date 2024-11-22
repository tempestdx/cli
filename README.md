# Tempest CLI

[![Go Reference](https://pkg.go.dev/badge/github.com/tempestdx/cli)](https://pkg.go.dev/github.com/tempestdx/cli)
[![Test Status](https://github.com/tempestdx/cli/actions/workflows/go.yml/badge.svg?branch=main)](https://github.com/tempestdx/cli/actions/workflows/go.yml?query=branch%3Amain)

The official [Tempest][tempest] CLI client.

## Requirements

- Go 1.23 or later

## Installation

The quick start installs the Tempest CLI, scaffolds an app locally, and displays
configuration details

```sh
# Install the Tempest CLI
go install github.com/tempestdx/cli/tempest@latest

# Create a directory and initialize your first Private App
mkdir tempest && cd tempest
tempest app init <name>
```

## Documentation

For documentation on all available commands, check out the
[API documentation][api-docs].

For details on all the functionality in this client, see the
[Go documentation][goref].

## Support

New features and bug fixes are released on the latest major version of the
Tempest CLI client library. If you are on an older major version, we recommend
that you upgrade to the latest in order to use the new features and bug fixes
including those for security vulnerabilities. Older major versions of the
package will continue to be available for use, but will not be receiving any
updates.

## Development

Pull requests from the community are welcome. If you submit one, please keep the
following guidelines in mind:

1. Code must be `go fmt` compliant.
2. All types, structs and funcs should be documented.
3. Ensure that `go test` succeeds.

## Test

The test suite needs testify's `require` package to run:

    github.com/stretchr/testify/require

Before running the tests, make sure to grab all of the package's dependencies:

    go get -t -v ./...

Run all tests:

    go test -race -v ./...

Run tests for one package:

    go test -v ./internal/...

Run a single test:

    go test -v ./internal/... -run TestReadConfigSuccess

For any requests, bug or comments, please [open an issue][issues] or
[submit a pull request][pulls].

[api-docs]: https://docs.tempestdx.com/developer/cli/tempest
[goref]: https://pkg.go.dev/github.com/tempestdx/cli
[issues]: https://github.com/tempestdx/cli/issues/new
[pulls]: https://github.com/tempestdx/cli/pulls
[tempest]: https://tempestdx.com/
