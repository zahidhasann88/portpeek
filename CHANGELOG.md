# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- Windows Docker named-pipe requests support cancellation and deadlines.
- Kill target selection checks every matching socket for protected/container ownership.
- Kill supports JSON output and Windows permission hints identify Administrator privileges.
- Windows checkout preserves LF in Go sources and golden fixtures.


## [0.1.0] - 2026-10-10

### Added

- List listening TCP and UDP sockets with port, protocol, address, PID, process name, user, and container.
- Detail view for a single port: command line, working directory, executable, start time, parent PID, and container.
- `kill <port>` command: confirmation prompt (`--yes` to skip), SIGTERM first, `--force` for SIGKILL, and refusal of unsafe targets (PID 0/1, Windows System, portpeek itself, container-published ports, ambiguous or hidden owners).
- Docker integration over the Engine HTTP API (Unix socket, Windows named pipe, or `DOCKER_HOST`), with `docker-proxy` attribution.
- `--json` output with a documented, versioned schema.
- Flags `--tcp`, `--udp`, `--all`, `--no-docker`, `--no-color`, and `--version`.
- Colour output on terminals, respecting `NO_COLOR`.
- Documented exit codes: 0 success, 1 port not in use, 2 usage error, 3 permission or runtime error.
- `portpeek version` and `--version` print version, commit, and build date.
- GoReleaser configuration for Linux, macOS, and Windows on amd64 and arm64, with checksums.
- CI for tests, vet, and lint on Ubuntu, macOS, and Windows, with Go 1.22 and 1.23.

[Unreleased]: https://github.com/zahidhasann88/portpeek/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/zahidhasann88/portpeek/releases/tag/v0.1.0
