# Contributing to Portpeek

Thanks for helping make Portpeek better. This guide covers how to set up a development environment, how the code is organised, and what we expect in a pull request.

By participating, you agree to abide by the [Code of Conduct](CODE_OF_CONDUCT.md).

## Getting started

Requirements:

- Go 1.22 or newer
- `make`
- [golangci-lint](https://golangci-lint.run/) v2 (the version CI uses is in `.github/workflows/ci.yml`)
- Optional: [GoReleaser](https://goreleaser.com/) v2, to test release builds

```sh
git clone https://github.com/zahidhasann88/portpeek.git
cd portpeek
make check   # vet, test, lint
make build
./portpeek
```

## Architecture

```
cmd/portpeek/        main package. Build metadata and a single call into internal/cli.
internal/cli/        cobra commands, flag parsing, exit codes, prompts. OS access is limited to deps.go (wiring) and term.go (TTY check).
internal/ports/      platform-agnostic model (Socket, Entry, Container), the collector that merges
                     sources, kill safety rules, and the gopsutil-backed system implementation.
internal/docker/     Docker Engine HTTP client over a Unix socket or named pipe, and response parsing.
internal/render/     table, detail, and JSON output. Writes to an io.Writer and reads no OS state.
```

Dependency rules:

- `render` and `cli` depend on interfaces, not on the operating system. Tests inject fakes.
- The interfaces live in `internal/ports/source.go`: `PortSource`, `ProcessSource`, `ContainerSource`, and `ProcessControl`.
- gopsutil is used only in `internal/ports/system.go`. OS access in the CLI is limited to `internal/cli/deps.go` (wiring) and `internal/cli/term.go` (TTY check).
- `internal/docker` must not import the Docker SDK. HTTP requests use `net/http`; Windows named-pipe transport uses `go-winio`.

## Testing

- Unit tests live next to the code. Table-driven tests are preferred.
- Docker parsing uses recorded JSON fixtures in `internal/docker/testdata/`. To add a fixture, copy a real `GET /containers/json` response and remove sensitive values.
- Rendering uses golden files in `internal/render/testdata/`. After an intentional output change, run `go test ./internal/render -update` and review the diff.
- Kill safety rules have dedicated tests in `internal/ports/kill_test.go`. Changes to those rules need matching tests.
- Run the race detector with `go test -race ./...` before opening a pull request.

## Coding standards

- Format with `gofmt` (`make fmt`). Lint with `make lint`.
- Handle every error. Wrap errors with context using `%w`. Do not panic in normal paths.
- Keep rendering and CLI code free of direct operating-system calls.
- Keep dependencies minimal. Discuss new modules in an issue first.
- Exported identifiers need doc comments.

## Commits and pull requests

- Use [Conventional Commits](https://www.conventionalcommits.org/): `feat:`, `fix:`, `docs:`, `test:`, `chore:`, `ci:`, `refactor:`.
- Keep commits focused. One logical change per commit is ideal.
- Update `CHANGELOG.md` under **Unreleased** for user-visible changes.
- Fill in the pull request template. Link the issue it fixes.

## Platform notes

Portpeek must build for Linux, macOS, and Windows. If you change `internal/ports/system.go`, `internal/docker/pipe_windows.go`, or any `*_windows.go` file, check the cross-compile:

```sh
GOOS=windows GOARCH=amd64 go build ./...
GOOS=darwin  GOARCH=arm64 go build ./...
```

## Reporting bugs and ideas

Use the issue templates. For security problems, follow [SECURITY.md](SECURITY.md) instead.
