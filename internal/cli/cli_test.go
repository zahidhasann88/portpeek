package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zahidhasann88/portpeek/internal/ports"
)

// fakeSockets, fakeProcesses, fakeContainers and fakeControl replace the
// operating system in CLI tests.
type fakeSockets struct {
	socks []ports.Socket
	err   error
}

func (f fakeSockets) Sockets(context.Context) ([]ports.Socket, error) { return f.socks, f.err }

type fakeProcesses struct {
	info   map[int32]ports.ProcessInfo
	denied map[int32]bool
}

func (f fakeProcesses) Process(_ context.Context, pid int32) (ports.ProcessInfo, error) {
	info, ok := f.info[pid]
	if !ok {
		return ports.ProcessInfo{}, errors.New("no such process")
	}
	if f.denied[pid] {
		return info, fmt.Errorf("cwd: %w", ports.ErrAccessDenied)
	}
	return info, nil
}

type fakeContainers struct {
	bindings []ports.Binding
	err      error
	called   *bool
}

func (f fakeContainers) Bindings(context.Context) ([]ports.Binding, error) {
	if f.called != nil {
		*f.called = true
	}
	return f.bindings, f.err
}

type fakeControl struct {
	alive      map[int32]bool
	ignoreTerm bool
	terminated []int32
	killed     []int32
}

func (f *fakeControl) Terminate(pid int32) error {
	f.terminated = append(f.terminated, pid)
	if !f.ignoreTerm {
		delete(f.alive, pid)
	}
	return nil
}

func (f *fakeControl) Kill(pid int32) error {
	f.killed = append(f.killed, pid)
	delete(f.alive, pid)
	return nil
}

func (f *fakeControl) Alive(pid int32) (bool, error) { return f.alive[pid], nil }

// harness bundles the fakes and captured output for one CLI invocation.
type harness struct {
	deps    Deps
	stdout  *bytes.Buffer
	stderr  *bytes.Buffer
	control *fakeControl
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	h.control = &fakeControl{alive: map[int32]bool{4311: true, 2100: true, 900: true}}
	h.deps = Deps{
		Sockets: fakeSockets{socks: []ports.Socket{
			{Protocol: ports.ProtocolTCP, Address: "::", Port: 3000, State: ports.StateListen, PID: 4311},
			{Protocol: ports.ProtocolTCP, Address: "0.0.0.0", Port: 8080, State: ports.StateListen, PID: 2100},
			{Protocol: ports.ProtocolUDP, Address: "0.0.0.0", Port: 5353, State: ports.StateUnconnect, PID: 0},
			{Protocol: ports.ProtocolTCP, Address: "0.0.0.0", Port: 22, State: ports.StateListen, PID: 0},
			{Protocol: ports.ProtocolTCP, Address: "127.0.0.1", Port: 3000, State: "ESTABLISHED", PID: 4311, RemoteIP: "127.0.0.1", RemotePort: 51000},
		}},
		Processes: fakeProcesses{
			info: map[int32]ports.ProcessInfo{
				4311: {Name: "node", User: "alice", Command: "node server.js", Executable: "/usr/bin/node", WorkingDir: "/srv", PPID: 1},
				2100: {Name: "docker-proxy", User: "root"},
				900:  {Name: "sleeper", User: "alice"},
			},
			denied: map[int32]bool{},
		},
		Containers: fakeContainers{bindings: []ports.Binding{
			{HostIP: "0.0.0.0", HostPort: 8080, Protocol: ports.ProtocolTCP,
				Container: ports.Container{ID: "7f3a9c2e1b4d", Name: "web", Image: "nginx:1.27"}},
		}},
		Control:   h.control,
		Stdin:     strings.NewReader(""),
		Stdout:    h.stdout,
		Stderr:    h.stderr,
		Getenv:    func(string) string { return "" },
		Self:      5555,
		GOOS:      "linux",
		Build:     BuildInfo{Version: "v0.1.0", Commit: "abc1234", Date: "2026-10-10T00:00:00Z"},
		Timing:    ports.KillTiming{Grace: 20 * time.Millisecond, ForceGrace: 20 * time.Millisecond, Poll: time.Millisecond},
		Elevated:  false,
		StdoutTTY: false,
		StdinTTY:  false,
	}
	return h
}

func (h *harness) run(args ...string) int {
	return Run(args, h.deps)
}

func TestExitCodes(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		mutate   func(h *harness)
		wantCode int
	}{
		{name: "default list", args: nil, wantCode: ExitOK},
		{name: "port in use", args: []string{"3000"}, wantCode: ExitOK},
		{name: "port not in use", args: []string{"9999"}, wantCode: ExitNotInUse},
		{name: "port zero is usage error", args: []string{"0"}, wantCode: ExitUsage},
		{name: "port above 65535 is usage error", args: []string{"70000"}, wantCode: ExitUsage},
		{name: "non-numeric port is usage error", args: []string{"http"}, wantCode: ExitUsage},
		{name: "two positional args is usage error", args: []string{"80", "81"}, wantCode: ExitUsage},
		{name: "unknown flag is usage error", args: []string{"--bogus"}, wantCode: ExitUsage},
		{name: "kill without port is usage error", args: []string{"kill"}, wantCode: ExitUsage},
		{name: "socket table failure is runtime error", args: nil, mutate: func(h *harness) {
			h.deps.Sockets = fakeSockets{err: errors.New("read /proc/net/tcp: permission denied")}
		}, wantCode: ExitRuntime},
		{name: "version", args: []string{"--version"}, wantCode: ExitOK},
		{name: "version command", args: []string{"version"}, wantCode: ExitOK},
		{name: "help", args: []string{"--help"}, wantCode: ExitOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			if tc.mutate != nil {
				tc.mutate(h)
			}
			if got := h.run(tc.args...); got != tc.wantCode {
				t.Errorf("exit = %d, want %d\nstdout: %s\nstderr: %s", got, tc.wantCode, h.stdout, h.stderr)
			}
		})
	}
}

func TestDefaultTableHasColumnsAndSortedPorts(t *testing.T) {
	h := newHarness(t)
	if code := h.run(); code != ExitOK {
		t.Fatalf("exit = %d; stderr: %s", code, h.stderr)
	}
	lines := strings.Split(strings.TrimRight(h.stdout.String(), "\n"), "\n")
	if want := "PORT"; !strings.HasPrefix(lines[0], want) {
		t.Fatalf("header = %q", lines[0])
	}
	for _, col := range []string{"PROTO", "ADDRESS", "PID", "PROCESS", "USER", "CONTAINER"} {
		if !strings.Contains(lines[0], col) {
			t.Errorf("header missing %s: %q", col, lines[0])
		}
	}
	if strings.Contains(lines[0], "STATE") {
		t.Error("STATE column must only appear with --all")
	}
	var ports []string
	for _, l := range lines[1:] {
		ports = append(ports, strings.Fields(l)[0])
	}
	if strings.Join(ports, ",") != "22,3000,5353,8080" {
		t.Errorf("ports = %v, want sorted 22,3000,5353,8080 (ESTABLISHED excluded)", ports)
	}
	if !strings.Contains(h.stdout.String(), "web (7f3a9c2e1b4d)") {
		t.Errorf("container not shown:\n%s", h.stdout)
	}
}

func TestAllShowsStateColumn(t *testing.T) {
	h := newHarness(t)
	h.run("--all")
	if !strings.Contains(h.stdout.String(), "STATE") || !strings.Contains(h.stdout.String(), "ESTABLISHED") {
		t.Errorf("--all output:\n%s", h.stdout)
	}
}

func TestJSONIsStableArrayAndNeverColoured(t *testing.T) {
	h := newHarness(t)
	h.deps.StdoutTTY = true
	if code := h.run("--json", "3000"); code != ExitOK {
		t.Fatalf("exit = %d; stderr: %s", code, h.stderr)
	}
	out := h.stdout.String()
	if strings.Contains(out, "\x1b[") {
		t.Error("JSON output must not contain ANSI escapes")
	}
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(decoded) != 1 || decoded[0]["process_name"] != "node" || decoded[0]["pid"] != float64(4311) {
		t.Errorf("unexpected JSON: %v", decoded)
	}
}

func TestPortNotInUseJSONPrintsEmptyArray(t *testing.T) {
	h := newHarness(t)
	code := h.run("--json", "9999")
	if code != ExitNotInUse {
		t.Fatalf("exit = %d, want %d", code, ExitNotInUse)
	}
	if strings.TrimSpace(h.stdout.String()) != "[]" {
		t.Errorf("stdout = %q, want []", h.stdout)
	}
	if !strings.Contains(h.stderr.String(), "port 9999 is not in use") {
		t.Errorf("stderr = %q", h.stderr)
	}
}

func TestDetailViewShowsContainerAndProcessFields(t *testing.T) {
	h := newHarness(t)
	if code := h.run("8080"); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	out := h.stdout.String()
	for _, want := range []string{"Port:          8080", "Container:     web", "ID:", "Image:       nginx:1.27"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail missing %q:\n%s", want, out)
		}
	}
}

func TestColourRules(t *testing.T) {
	cases := []struct {
		name       string
		tty        bool
		noColor    bool
		noColorEnv string
		wantANSI   bool
	}{
		{name: "terminal enables colour", tty: true, wantANSI: true},
		{name: "pipe disables colour", tty: false},
		{name: "--no-color flag", tty: true, noColor: true},
		{name: "NO_COLOR set", tty: true, noColorEnv: "1"},
		{name: "NO_COLOR empty does not disable", tty: true, noColorEnv: "", wantANSI: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.deps.StdoutTTY = tc.tty
			h.deps.Getenv = func(k string) string {
				if k == "NO_COLOR" {
					return tc.noColorEnv
				}
				return ""
			}
			args := []string{}
			if tc.noColor {
				args = append(args, "--no-color")
			}
			h.run(args...)
			if got := strings.Contains(h.stdout.String(), "\x1b["); got != tc.wantANSI {
				t.Errorf("ANSI present = %v, want %v", got, tc.wantANSI)
			}
		})
	}
}

func TestPermissionHintOnlyWhenNotElevated(t *testing.T) {
	h := newHarness(t)
	h.run()
	if !strings.Contains(h.stderr.String(), "sudo") {
		t.Errorf("expected sudo hint for hidden owners, stderr=%q", h.stderr)
	}
	if n := strings.Count(h.stderr.String(), "hint:"); n != 1 {
		t.Errorf("hint printed %d times, want exactly once", n)
	}

	elevated := newHarness(t)
	elevated.deps.Elevated = true
	elevated.run()
	if strings.Contains(elevated.stderr.String(), "hint:") {
		t.Errorf("no hint expected when elevated, stderr=%q", elevated.stderr)
	}
}

func TestWindowsHintMentionsAdministrator(t *testing.T) {
	h := newHarness(t)
	h.deps.GOOS = "windows"
	h.run()
	if !strings.Contains(h.stderr.String(), "Administrator") {
		t.Errorf("stderr = %q", h.stderr)
	}
}

func TestDockerUnavailableIsSilent(t *testing.T) {
	h := newHarness(t)
	h.deps.Containers = fakeContainers{err: fmt.Errorf("dial: %w", ports.ErrUnavailable)}
	if code := h.run(); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if strings.Contains(h.stderr.String(), "warning") {
		t.Errorf("unavailable Docker must not warn: %q", h.stderr)
	}
}

func TestDockerErrorWarnsAndContinues(t *testing.T) {
	h := newHarness(t)
	h.deps.Containers = fakeContainers{err: errors.New("docker API returned 500 Internal Server Error")}
	if code := h.run(); code != ExitOK {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(h.stderr.String(), "warning") {
		t.Errorf("expected warning on stderr: %q", h.stderr)
	}
	if !strings.Contains(h.stdout.String(), "3000") {
		t.Error("table should still list sockets")
	}
}

func TestNoDockerSkipsContainerLookup(t *testing.T) {
	h := newHarness(t)
	called := false
	h.deps.Containers = fakeContainers{called: &called}
	h.run("--no-docker")
	if called {
		t.Error("--no-docker must not query the container source")
	}
}

func TestVersionOutput(t *testing.T) {
	h := newHarness(t)
	h.run("version")
	want := "portpeek v0.1.0 (commit abc1234, built 2026-10-10T00:00:00Z)\n"
	if h.stdout.String() != want {
		t.Errorf("version = %q, want %q", h.stdout, want)
	}
	h2 := newHarness(t)
	h2.run("--version")
	if !strings.Contains(h2.stdout.String(), "commit abc1234") {
		t.Errorf("--version = %q", h2.stdout)
	}
}

// --- kill command -----------------------------------------------------------

func TestKillRefusesWithoutConfirmationWhenNotInteractive(t *testing.T) {
	h := newHarness(t)
	if code := h.run("kill", "3000"); code != ExitUsage {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, ExitUsage, h.stderr)
	}
	if len(h.control.terminated) != 0 {
		t.Error("process must not be signalled without confirmation")
	}
}

func TestKillYesTerminatesProcess(t *testing.T) {
	h := newHarness(t)
	if code := h.run("kill", "3000", "--yes"); code != ExitOK {
		t.Fatalf("exit = %d; stderr=%q stdout=%q", code, h.stderr, h.stdout)
	}
	if len(h.control.terminated) != 1 || h.control.terminated[0] != 4311 {
		t.Errorf("terminated = %v, want [4311]", h.control.terminated)
	}
	if len(h.control.killed) != 0 {
		t.Error("SIGKILL must not be sent without --force")
	}
	if !strings.Contains(h.stdout.String(), "SIGTERM") {
		t.Errorf("stdout = %q", h.stdout)
	}
}

func TestKillInteractiveDeclineDoesNothing(t *testing.T) {
	h := newHarness(t)
	h.deps.StdinTTY = true
	h.deps.Stdin = strings.NewReader("n\n")
	if code := h.run("kill", "3000"); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if len(h.control.terminated) != 0 {
		t.Error("declined kill must not signal")
	}
	if !strings.Contains(h.stdout.String(), "Aborted") {
		t.Errorf("stdout = %q", h.stdout)
	}
	if !strings.Contains(h.stderr.String(), "[y/N]") {
		t.Errorf("prompt missing: %q", h.stderr)
	}
}

func TestKillInteractiveConfirm(t *testing.T) {
	h := newHarness(t)
	h.deps.StdinTTY = true
	h.deps.Stdin = strings.NewReader("y\n")
	if code := h.run("kill", "3000"); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if len(h.control.terminated) != 1 {
		t.Errorf("terminated = %v", h.control.terminated)
	}
}

func TestKillSafetyExitCodes(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		mutate   func(h *harness)
		wantCode int
		wantMsg  string
	}{
		{name: "port not in use", args: []string{"kill", "9999", "--yes"}, wantCode: ExitNotInUse, wantMsg: "not in use"},
		{name: "owner hidden needs sudo", args: []string{"kill", "22", "--yes"}, wantCode: ExitRuntime, wantMsg: "sudo"},
		{name: "portpeek itself is refused", args: []string{"kill", "3000", "--yes"}, mutate: func(h *harness) {
			h.deps.Self = 4311
		}, wantCode: ExitRuntime, wantMsg: "portpeek itself"},
		{name: "container-owned port is refused", args: []string{"kill", "8080", "--yes"}, wantCode: ExitRuntime, wantMsg: "docker stop"},
		{name: "ambiguous owners are a usage error", args: []string{"kill", "3000", "--yes"}, mutate: func(h *harness) {
			h.deps.Sockets = fakeSockets{socks: []ports.Socket{
				{Protocol: ports.ProtocolTCP, Address: "::", Port: 3000, State: ports.StateListen, PID: 4311},
				{Protocol: ports.ProtocolTCP, Address: "0.0.0.0", Port: 3000, State: ports.StateListen, PID: 900},
			}}
		}, wantCode: ExitUsage, wantMsg: "--tcp or --udp"},
		{name: "SIGTERM ignored without --force", args: []string{"kill", "3000", "--yes"}, mutate: func(h *harness) {
			h.control.ignoreTerm = true
		}, wantCode: ExitRuntime, wantMsg: "--force"},
		{name: "SIGTERM ignored with --force", args: []string{"kill", "3000", "--yes", "--force"}, mutate: func(h *harness) {
			h.control.ignoreTerm = true
		}, wantCode: ExitOK},
		{name: "invalid port", args: []string{"kill", "0", "--yes"}, wantCode: ExitUsage, wantMsg: "invalid port"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			if tc.mutate != nil {
				tc.mutate(h)
			}
			code := h.run(tc.args...)
			if code != tc.wantCode {
				t.Fatalf("exit = %d, want %d\nstdout: %s\nstderr: %s", code, tc.wantCode, h.stdout, h.stderr)
			}
			if tc.wantMsg != "" && !strings.Contains(h.stdout.String()+h.stderr.String(), tc.wantMsg) {
				t.Errorf("output lacks %q:\nstdout: %s\nstderr: %s", tc.wantMsg, h.stdout, h.stderr)
			}
		})
	}
}

func TestKillNeverSignalsRefusedTargets(t *testing.T) {
	h := newHarness(t)
	h.deps.Sockets = fakeSockets{socks: []ports.Socket{
		{Protocol: ports.ProtocolTCP, Address: "0.0.0.0", Port: 1, State: ports.StateListen, PID: 1},
	}}
	if code := h.run("kill", "1", "--yes"); code != ExitRuntime {
		t.Fatalf("exit = %d, want %d", code, ExitRuntime)
	}
	if len(h.control.terminated)+len(h.control.killed) != 0 {
		t.Errorf("PID 1 was signalled: TERM %v KILL %v", h.control.terminated, h.control.killed)
	}
}

func TestWindowsKillPermissionHint(t *testing.T) {
	h := newHarness(t)
	h.deps.GOOS = "windows"
	if code := h.run("kill", "22", "--yes"); code != ExitRuntime {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(h.stderr.String(), "Administrator") || strings.Contains(h.stderr.String(), "sudo") {
		t.Fatalf("incorrect Windows hint: %s", h.stderr)
	}
}

func TestKillJSONOutput(t *testing.T) {
	h := newHarness(t)
	if code := h.run("kill", "3000", "--yes", "--json"); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	var entries []map[string]any
	if err := json.Unmarshal(h.stdout.Bytes(), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0]["pid"] != float64(4311) || len(h.control.terminated) != 1 {
		t.Fatalf("unexpected result: %v", entries)
	}
	declined := newHarness(t)
	declined.deps.StdinTTY = true
	declined.deps.Stdin = strings.NewReader("n\n")
	if code := declined.run("kill", "3000", "--json"); code != ExitOK || strings.TrimSpace(declined.stdout.String()) != "[]" || len(declined.control.terminated) != 0 {
		t.Fatalf("unexpected declined result: %s", declined.stdout)
	}
}
