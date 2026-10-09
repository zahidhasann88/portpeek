# Portpeek

Portpeek shows which applications and Docker containers are using your local network ports, and can stop the process that holds a port.

> **Status:** early release (v0.1.0). The command-line interface and JSON schema are documented below. Changes are listed in [CHANGELOG.md](CHANGELOG.md); before 1.0 they may change in minor releases.

## Features

- Lists listening TCP and UDP sockets with the port, protocol, bind address, PID, process name, user, and owning Docker container.
- A detail view for one port: full command line, working directory, executable, start time, parent PID, and container details.
- `portpeek kill <port>` stops the owner of a port, with a confirmation prompt, a SIGTERM-first policy, and refusal rules for unsafe targets.
- Stable `--json` output for scripts.
- Works on Linux, macOS, and Windows, on amd64 and arm64.
- A single binary. Docker information requires a running Docker daemon.

## Install

### With Go (1.22 or newer)

```sh
go install github.com/zahidhasann88/portpeek/cmd/portpeek@latest
```

### Release binaries

Download the archive for your platform from the [Releases page](https://github.com/zahidhasann88/portpeek/releases). Archives are `.tar.gz` for Linux and macOS and `.zip` for Windows. Each release includes `checksums.txt` (SHA-256).

```sh
# Example: Linux amd64
curl -LO https://github.com/zahidhasann88/portpeek/releases/download/v0.1.0/portpeek_0.1.0_linux_amd64.tar.gz
curl -LO https://github.com/zahidhasann88/portpeek/releases/download/v0.1.0/checksums.txt
sha256sum -c checksums.txt --ignore-missing
tar -xzf portpeek_0.1.0_linux_amd64.tar.gz portpeek
sudo install -m 0755 portpeek /usr/local/bin/
```

### From source

```sh
git clone https://github.com/zahidhasann88/portpeek.git
cd portpeek
make build      # ./portpeek
make install    # into $GOBIN
```

On Windows PowerShell, build without Make:

```powershell
go build -o bin/portpeek.exe ./cmd/portpeek
.\bin\portpeek.exe
```

## Usage

### List listening sockets (default)

```
$ portpeek
PORT  PROTO  ADDRESS    PID    PROCESS          USER             CONTAINER
22    tcp    0.0.0.0    812    sshd             root             -
53    udp    127.0.0.1  1043   systemd-resolve  systemd-resolve  -
3000  tcp    ::         48211  node             alice            -
5432  tcp    127.0.0.1  -      -                -                analytics_db (0123456789ab)
8080  tcp    0.0.0.0    2100   web              root             web (7f3a9c2e1b4d)
8787  tcp    127.0.0.1  51377  python3          alice            -
```

Rows are sorted by port, then protocol (TCP before UDP), then address. A `-` means the value is unknown, usually because the process belongs to another user (see [Permissions](#permissions)).

When a published Docker port is held by `docker-proxy`, the PROCESS column shows the container name and the CONTAINER column shows the container. The `docker-proxy` process itself is still reported in `--json` as `process_name`.

### Include connections that are not listening

```
$ portpeek --all --tcp 3000
PORT  PROTO  ADDRESS  STATE        PID    PROCESS  USER   CONTAINER
3000  tcp    ::       LISTEN       48211  node     alice  -
3000  tcp    ::1      ESTABLISHED  48211  node     alice  -
```

`--all` adds a `STATE` column. Without it, only listening TCP sockets and unconnected UDP sockets are shown.

### Details for one port

```
$ portpeek 8080
Port:          8080
Protocol:      tcp
Family:        ipv4
Address:       0.0.0.0
State:         LISTEN
Remote:        -
PID:           2100
Parent PID:    1
Process:       docker-proxy
User:          root
Command:       /usr/bin/docker-proxy -proto tcp -host-ip 0.0.0.0 -host-port 8080 -container-ip 172.17.0.2 -container-port 80
Executable:    /usr/bin/docker-proxy
Working dir:   /
Started:       2026-10-09T08:12:04+06:00
Container:     web
  ID:          7f3a9c2e1b4d
  Image:       nginx:1.27-alpine
```

Fields that cannot be read show `-`. If several sockets use the port, each one gets its own block.

### Kill the process on a port

```
$ portpeek kill 3000
Kill PID 48211 (node, user alice) holding tcp port 3000? [y/N] y
Sent SIGTERM to PID 48211 (node); it exited. Port 3000 is free.
```

```
$ portpeek kill 9000 --yes --force
Sent SIGTERM, then SIGKILL, to PID 61002 (java); it exited. Port 9000 is free.
```

See [Killing processes](#killing-processes) for the full rules.

### Flags

| Flag | Applies to | Description |
| --- | --- | --- |
| `--json` | all | Print a JSON array instead of a table. See [JSON output](#json-output). |
| `--tcp` | list, port, kill | Show only TCP sockets. |
| `--udp` | list, port, kill | Show only UDP sockets. |
| `--all` | list, port | Include non-listening sockets, such as established connections. Adds the `STATE` column in the table. |
| `--no-docker` | all | Do not query the Docker daemon. |
| `--no-color` | all | Disable colour. Colour is also disabled by `NO_COLOR` and for non-terminal output. |
| `--version` | all | Print version, commit, and build date. |
| `-y`, `--yes` | kill | Do not ask for confirmation. |
| `--force` | kill | Send SIGKILL if SIGTERM does not stop the process within the grace period. |

Run `portpeek --help` or `portpeek kill --help` for the same information.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success. Also used when you decline the kill prompt. |
| `1` | A specific port is not in use (`portpeek <port>`, `portpeek kill <port>`). |
| `2` | Usage error: invalid port, unknown flag, wrong number of arguments, or `kill` run without a terminal and without `--yes`. Also used when a port has more than one owner and `--tcp` or `--udp` is needed to choose. |
| `3` | Permission or runtime error, or a refused action: the system could not be read, a process could not be signalled, a protected target was refused, or SIGTERM did not stop the process without `--force`. |

With `--json`, a port that is not in use still prints `[]` on stdout and exits `1`, so scripts can check both.

## Killing processes

`portpeek kill <port>` stops the process that is listening on the port.

1. **Select the target.** The port must have exactly one listening owner (one PID across TCP and UDP, after `--tcp` / `--udp` filters).
2. **Check safety.** The command refuses to act when:
   - the owner is not visible to you (run with `sudo`);
   - more than one process holds the port;
   - the target is PID 0 or PID 1, the Windows System process (PID 4), or portpeek itself;
   - the port is published by a container. The message suggests `docker stop <name>` instead, because killing `docker-proxy` does not stop the container cleanly.
3. **Confirm.** Without `--yes`, it prompts `[y/N]`. Declining exits `0` and changes nothing. If stdin is not a terminal, it refuses with exit `2` rather than assuming yes, so scripts must pass `--yes`.
4. **Signal.** It sends SIGTERM and waits up to 5 seconds for the process to exit. If the process is still running:
   - without `--force`, it stops and exits `3`. It never sends SIGKILL on its own;
   - with `--force`, it sends SIGKILL and waits up to 2 more seconds.

On Windows there is no separate graceful signal. The first step calls `TerminateProcess`, so it is already forceful.

## JSON output

`--json` prints a JSON array with one object per socket. The array is always present, and it is empty (`[]`) when nothing matches. Keys are `snake_case`. Unknown values are `null`, never `""` or `0`.

Example (abbreviated):

```json
[
  {
    "port": 8080,
    "protocol": "tcp",
    "family": "ipv4",
    "address": "0.0.0.0",
    "state": "LISTEN",
    "pid": 2100,
    "parent_pid": 1,
    "process_name": "docker-proxy",
    "user": "root",
    "command": "/usr/bin/docker-proxy -proto tcp -host-ip 0.0.0.0 -host-port 8080",
    "executable": "/usr/bin/docker-proxy",
    "working_dir": "/",
    "started_at": "2026-10-09T02:12:04Z",
    "remote_address": null,
    "container": {
      "id": "7f3a9c2e1b4d",
      "name": "web",
      "image": "nginx:1.27-alpine"
    }
  }
]
```

For a successful kill --json, the array contains the selected socket as inspected before termination. Declining confirmation returns []; the confirmation prompt goes to stderr.

### Schema (version 1)

| Key | Type | Description |
| --- | --- | --- |
| `port` | integer | Local port number, 1–65535. |
| `protocol` | string | `"tcp"` or `"udp"`. |
| `family` | string | `"ipv4"` or `"ipv6"`, from the local address. |
| `address` | string or null | Local (bind) address, such as `"0.0.0.0"`, `"::"`, or `"127.0.0.1"`. |
| `state` | string | TCP: `LISTEN`, `ESTABLISHED`, and other OS states. UDP: `UNCONN` (unconnected) or `CONNECTED`. |
| `pid` | integer or null | Owning process ID. `null` if the owner is not visible. |
| `parent_pid` | integer or null | Parent process ID. |
| `process_name` | string or null | Process name as reported by the OS. For Docker-published ports this is the real owner, such as `docker-proxy`. |
| `user` | string or null | User that owns the process. |
| `command` | string or null | Full command line. |
| `executable` | string or null | Path to the executable. |
| `working_dir` | string or null | Current working directory of the process. |
| `started_at` | string or null | Process start time, RFC 3339. |
| `remote_address` | string or null | Remote `ip:port` for connected sockets. Usually `null` for listeners. |
| `container` | object or null | Present when a Docker container publishes this port. Fields: `id` (12-character short ID), `name`, `image`. |

The schema is versioned by the release. Keys are only added in minor releases; existing keys are not renamed or removed before 1.0.

## Permissions

Portpeek reads the operating system's socket table and per-process data.

- **Linux:** The socket table is world-readable, but a process's owner is only visible to its owner or to root. Other users' sockets appear with `-` for PID and process.
- **macOS:** Similar to Linux. Some process details may need root.
- **Windows:** Process details of protected processes need an elevated (Administrator) shell.

When some details are hidden, portpeek shows `-` and prints one hint on stderr, for example:

```
hint: some process details are hidden (other users); re-run with sudo to show them (shown as "-").
```

Portpeek never crashes on permission errors. It does not need root to run, and it only uses root when you choose to run it with `sudo`. `kill` on a process you do not own fails with exit `3` and suggests `sudo`.

## Docker

Portpeek reads running containers from the Docker Engine HTTP API. It does not use the Docker SDK.

- **Endpoint:** `DOCKER_HOST` if set (`unix://`, `npipe://`, or `tcp://` without TLS). Otherwise `/var/run/docker.sock` on Linux and macOS, or `\\.\pipe\docker_engine` on Windows.
- **Missing daemon:** If the socket does not exist or refuses connections, container information is skipped silently.
- **Permission denied on the socket:** Portpeek prints a warning and continues without container information (for example, when your user is not in the `docker` group).
- **Mapping:** Each published host port (TCP or UDP) maps to the container's name, image, and short (12-character) ID. Only running containers are listed.
- **`docker-proxy`:** Docker's proxy usually owns published ports. For those sockets portpeek shows the container. The rule also applies to Docker Desktop's `com.docker.backend` and rootless Docker's `rootlesskit`.
- **Heuristic:** If the owning process is hidden (for example, a root-owned `docker-proxy` seen by a normal user) and a published port matches on protocol, port, and address, the socket is attributed to that container. This is a match on the published port, not a proof of ownership.
- **Not covered:** Containers in host network mode, and containers whose processes listen directly on the host without a published port.
- `--no-docker` skips the Docker lookup entirely.

## Supported platforms

| OS | Architectures | Status |
| --- | --- | --- |
| Linux | amd64, arm64 | Run end-to-end on Linux (amd64) with real processes and a fake Docker daemon. Unit tests run in CI on Go 1.22 and 1.23. |
| macOS | amd64, arm64 | Cross-compiled and linted. Not yet run on macOS. CI runs the test suite on macOS. |
| Windows | amd64, arm64 | Windows amd64 CLI and Docker Desktop published-port mapping checked locally. Named-pipe transport has regression tests. ARM64 is cross-compiled; CI results must be checked after pushing. |

The CI workflow in `.github/workflows/ci.yml` runs tests, vet, and lint on Ubuntu, macOS, and Windows.

Portpeek needs Go 1.22 or newer to build.

## Comparison with lsof, ss, and netstat

- `lsof -i` and `ss -tulpn` are powerful, but their output is built for experts, and their flags and columns differ across systems. `netstat` is deprecated on Linux, and its columns differ on macOS and Windows.
- Portpeek has one command, one table format on every OS, a Docker-aware view, and a kill command with safety checks. It does not replace their full filtering and inspection capabilities.

## Design decisions

These choices were made while building v0.1.0. Tell us if you disagree.

- **Default view is listening sockets only.** Use `--all` for established connections.
- **UDP "listening" means unconnected UDP sockets.** UDP has no LISTEN state.
- **IPv4 and IPv6 are separate rows,** because they can be bound differently. The table shows `tcp`/`udp`; the family is in `--json` and the detail view.
- **Unknown values are `-` in the table and `null` in JSON.** This keeps "not visible" distinct from zero.
- **No colour unless stdout is a terminal.** It is also off when `NO_COLOR` is set (to any non-empty value) or with `--no-color`, and always off for `--json`. Table headers are kept in plain output so scripts can read columns.
- **Hints and warnings go to stderr,** so stdout stays machine-readable.
- **`kill` refuses instead of guessing.** Ambiguous owners, hidden owners, container-owned ports, and protected PIDs all stop the command.
- **Non-interactive `kill` needs `--yes`.** Otherwise it refuses with exit `2`.
- **Declining the kill prompt is not an error** (exit `0`).
- **Portpeek does not kill `docker-proxy`.** Stop the container instead.
- **`--json` keeps the real process name** and adds `container`. The table shows the container name in PROCESS for proxied ports.
- **Only running containers are listed,** and only via the Docker Engine API. No Docker SDK.
- **No telemetry.** The only connection Portpeek makes is to the Docker endpoint you configure, and only when it is available.

## Development

```sh
make check    # vet, test, and lint
make test     # go test ./...
make lint     # gofmt check and golangci-lint
make build    # ./portpeek for this platform
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the architecture and contribution workflow.

## Contributing

Contributions are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) first, and follow the [Code of Conduct](CODE_OF_CONDUCT.md).

## Security

To report a vulnerability, follow [SECURITY.md](SECURITY.md). Please do not open a public issue for security problems.

## License

[MIT](LICENSE) © 2026 Zahid Hasan
