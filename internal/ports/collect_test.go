package ports

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func collect(t *testing.T, c Collector, opts Options) Result {
	t.Helper()
	res, err := c.Collect(context.Background(), opts)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	return res
}

func listen(proto, addr string, port uint16, pid int32) Socket {
	state := StateListen
	if proto == ProtocolUDP {
		state = StateUnconnect
	}
	return Socket{Protocol: proto, Address: addr, Port: port, State: state, PID: pid}
}

func ports_(entries []Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, fmt.Sprintf("%s/%d/%s", e.Protocol, e.Port, e.Address))
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestCollectSortsByPortThenProtocol(t *testing.T) {
	c := Collector{Sockets: fakeSockets{socks: []Socket{
		listen(ProtocolUDP, "0.0.0.0", 5353, 1),
		listen(ProtocolTCP, "::", 8080, 1),
		listen(ProtocolTCP, "0.0.0.0", 22, 1),
		listen(ProtocolUDP, "0.0.0.0", 22, 1),
	}}}
	got := ports_(collect(t, c, Options{}).Entries)
	want := []string{"tcp/22/0.0.0.0", "udp/22/0.0.0.0", "udp/5353/0.0.0.0", "tcp/8080/::"}
	if !equalStrings(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestCollectDefaultKeepsOnlyListening(t *testing.T) {
	c := Collector{Sockets: fakeSockets{socks: []Socket{
		listen(ProtocolTCP, "127.0.0.1", 3000, 10),
		{Protocol: ProtocolTCP, Address: "127.0.0.1", Port: 3000, State: "ESTABLISHED", PID: 11, RemoteIP: "127.0.0.1", RemotePort: 50000},
		listen(ProtocolUDP, "0.0.0.0", 5353, 12),
		{Protocol: ProtocolUDP, Address: "10.0.0.2", Port: 40000, State: StateConnected, PID: 13, RemoteIP: "1.1.1.1", RemotePort: 53},
	}}}

	got := ports_(collect(t, c, Options{}).Entries)
	if want := []string{"tcp/3000/127.0.0.1", "udp/5353/0.0.0.0"}; !equalStrings(got, want) {
		t.Errorf("default = %v, want %v", got, want)
	}

	all := collect(t, c, Options{All: true}).Entries
	if len(all) != 4 {
		t.Fatalf("--all returned %d entries, want 4", len(all))
	}
	for _, e := range all {
		if e.Port == 3000 && e.State == "ESTABLISHED" && e.RemoteAddress != "127.0.0.1:50000" {
			t.Errorf("RemoteAddress = %q, want 127.0.0.1:50000", e.RemoteAddress)
		}
	}
}

func TestCollectProtocolAndPortFilters(t *testing.T) {
	socks := []Socket{
		listen(ProtocolTCP, "0.0.0.0", 80, 1),
		listen(ProtocolUDP, "0.0.0.0", 80, 2),
		listen(ProtocolTCP, "0.0.0.0", 443, 3),
	}
	c := Collector{Sockets: fakeSockets{socks: socks}}

	cases := []struct {
		name string
		opts Options
		want []string
	}{
		{"both by default", Options{}, []string{"tcp/80/0.0.0.0", "udp/80/0.0.0.0", "tcp/443/0.0.0.0"}},
		{"tcp only", Options{TCP: true}, []string{"tcp/80/0.0.0.0", "tcp/443/0.0.0.0"}},
		{"udp only", Options{UDP: true}, []string{"udp/80/0.0.0.0"}},
		{"tcp and udp", Options{TCP: true, UDP: true}, []string{"tcp/80/0.0.0.0", "udp/80/0.0.0.0", "tcp/443/0.0.0.0"}},
		{"single port", Options{Port: 443}, []string{"tcp/443/0.0.0.0"}},
		{"port with proto mismatch", Options{Port: 443, UDP: true}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ports_(collect(t, c, tc.opts).Entries)
			if !equalStrings(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCollectDropsDuplicatesAndPortZero(t *testing.T) {
	dup := listen(ProtocolTCP, "::", 9000, 42)
	c := Collector{Sockets: fakeSockets{socks: []Socket{
		dup, dup,
		{Protocol: ProtocolTCP, Address: "/run/some.sock", Port: 0, State: StateListen},
	}}}
	res := collect(t, c, Options{})
	if len(res.Entries) != 1 {
		t.Fatalf("got %d entries, want 1: %+v", len(res.Entries), res.Entries)
	}
}

func TestCollectEnrichesAndCachesProcesses(t *testing.T) {
	started := time.Date(2026, 10, 9, 8, 30, 0, 0, time.UTC)
	procs := newFakeProcesses()
	procs.procs[500] = ProcessInfo{
		Name: "node", User: "alice", Command: "node server.js",
		Executable: "/usr/bin/node", WorkingDir: "/srv/app", PPID: 100, StartTime: started,
	}
	c := Collector{
		Sockets:   fakeSockets{socks: []Socket{listen(ProtocolTCP, "0.0.0.0", 3000, 500), listen(ProtocolTCP, "::", 3000, 500)}},
		Processes: procs,
	}
	res := collect(t, c, Options{})
	if procs.calls[500] != 1 {
		t.Errorf("process looked up %d times, want 1 (cached)", procs.calls[500])
	}
	if res.Restricted != 0 {
		t.Errorf("Restricted = %d, want 0", res.Restricted)
	}
	e := res.Entries[0]
	if e.ProcessName != "node" || e.User != "alice" || e.PPID != 100 || e.WorkingDir != "/srv/app" || !e.StartTime.Equal(started) {
		t.Errorf("entry not enriched: %+v", e)
	}
	if res.Entries[1].Family != FamilyIPv6 || res.Entries[0].Family != FamilyIPv4 {
		t.Errorf("families = %q, %q", res.Entries[0].Family, res.Entries[1].Family)
	}
}

func TestCollectCountsRestrictedEntries(t *testing.T) {
	procs := newFakeProcesses()
	procs.procs[7] = ProcessInfo{Name: "postgres", User: "postgres"}
	procs.errs[7] = fmt.Errorf("cwd: %w", ErrAccessDenied)
	c := Collector{
		Sockets: fakeSockets{socks: []Socket{
			listen(ProtocolTCP, "127.0.0.1", 5432, 7),
			listen(ProtocolTCP, "0.0.0.0", 631, 0), // owner not visible
		}},
		Processes: procs,
	}
	res := collect(t, c, Options{})
	if res.Restricted != 2 {
		t.Errorf("Restricted = %d, want 2", res.Restricted)
	}
	pg := entryOnPort(t, res.Entries, 5432)
	if pg.ProcessName != "postgres" || pg.PID != 7 {
		t.Errorf("partial info not kept: %+v", pg)
	}
	hidden := entryOnPort(t, res.Entries, 631)
	if hidden.PID != 0 || hidden.ProcessName != "" {
		t.Errorf("hidden owner should be unknown: %+v", hidden)
	}
}

func entryOnPort(t *testing.T, entries []Entry, port uint16) Entry {
	t.Helper()
	for _, e := range entries {
		if e.Port == port {
			return e
		}
	}
	t.Fatalf("no entry on port %d", port)
	return Entry{}
}

func TestCollectIgnoresVanishedProcess(t *testing.T) {
	procs := newFakeProcesses()
	procs.errs[9] = errors.New("process does not exist")
	c := Collector{
		Sockets:   fakeSockets{socks: []Socket{listen(ProtocolTCP, "0.0.0.0", 8000, 9)}},
		Processes: procs,
	}
	res := collect(t, c, Options{})
	if res.Restricted != 0 {
		t.Errorf("vanished process should not count as restricted, got %d", res.Restricted)
	}
	if res.Entries[0].ProcessName != "" || res.Entries[0].PID != 9 {
		t.Errorf("unexpected entry: %+v", res.Entries[0])
	}
}

func TestCollectSocketErrorIsFatal(t *testing.T) {
	boom := errors.New("boom")
	c := Collector{Sockets: fakeSockets{err: boom}}
	_, err := c.Collect(context.Background(), Options{})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want wrapping boom", err)
	}
}

func TestCollectWithoutPortSourceFails(t *testing.T) {
	if _, err := (Collector{}).Collect(context.Background(), Options{}); err == nil {
		t.Fatal("expected error without socket source")
	}
}

func TestCollectDockerAttributesDockerProxy(t *testing.T) {
	procs := newFakeProcesses()
	procs.procs[300] = ProcessInfo{Name: "docker-proxy", User: "root"}
	web := Container{ID: "a1b2c3d4e5f6", Name: "web", Image: "nginx:1.27"}
	c := Collector{
		Sockets:    fakeSockets{socks: []Socket{listen(ProtocolTCP, "0.0.0.0", 8080, 300), listen(ProtocolTCP, "::", 8080, 300)}},
		Processes:  procs,
		Containers: fakeContainers{bindings: []Binding{{HostIP: "0.0.0.0", HostPort: 8080, Protocol: ProtocolTCP, Container: web}}},
	}
	res := collect(t, c, Options{})
	for _, e := range res.Entries {
		if e.Container == nil || e.Container.Name != "web" {
			t.Fatalf("entry not attributed to container: %+v", e)
		}
		if got := e.DisplayProcess(); got != "web" {
			t.Errorf("DisplayProcess() = %q, want web", got)
		}
		if e.ProcessName != "docker-proxy" {
			t.Errorf("ProcessName = %q, want docker-proxy (real owner kept)", e.ProcessName)
		}
	}
}

func TestCollectDockerAttributesHiddenOwner(t *testing.T) {
	db := Container{ID: "0123456789ab", Name: "db", Image: "postgres:16"}
	c := Collector{
		Sockets:    fakeSockets{socks: []Socket{listen(ProtocolTCP, "0.0.0.0", 5432, 0)}},
		Containers: fakeContainers{bindings: []Binding{{HostIP: "", HostPort: 5432, Protocol: ProtocolTCP, Container: db}}},
	}
	res := collect(t, c, Options{})
	if got := res.Entries[0].Container; got == nil || got.Name != "db" {
		t.Errorf("hidden owner not attributed: %+v", res.Entries[0])
	}
}

func TestCollectDockerDoesNotStealOtherProcesses(t *testing.T) {
	procs := newFakeProcesses()
	procs.procs[55] = ProcessInfo{Name: "node", User: "alice"}
	c := Collector{
		Sockets:    fakeSockets{socks: []Socket{listen(ProtocolTCP, "0.0.0.0", 3000, 55)}},
		Processes:  procs,
		Containers: fakeContainers{bindings: []Binding{{HostIP: "0.0.0.0", HostPort: 3000, Protocol: ProtocolTCP, Container: Container{Name: "api"}}}},
	}
	res := collect(t, c, Options{})
	if res.Entries[0].Container != nil {
		t.Errorf("node process wrongly attributed to container: %+v", res.Entries[0].Container)
	}
	if res.Entries[0].DisplayProcess() != "node" {
		t.Errorf("DisplayProcess() = %q", res.Entries[0].DisplayProcess())
	}
}

func TestCollectDockerUnavailableIsSilent(t *testing.T) {
	c := Collector{
		Sockets:    fakeSockets{socks: []Socket{listen(ProtocolTCP, "0.0.0.0", 80, 0)}},
		Containers: fakeContainers{err: fmt.Errorf("dial: %w", ErrUnavailable)},
	}
	res := collect(t, c, Options{})
	if len(res.Warnings) != 0 {
		t.Errorf("warnings = %v, want none", res.Warnings)
	}
}

func TestCollectDockerErrorWarnsButKeepsEntries(t *testing.T) {
	c := Collector{
		Sockets:    fakeSockets{socks: []Socket{listen(ProtocolTCP, "0.0.0.0", 80, 0)}},
		Containers: fakeContainers{err: errors.New("docker API returned 500")},
	}
	res := collect(t, c, Options{})
	if len(res.Warnings) != 1 {
		t.Fatalf("warnings = %v, want one", res.Warnings)
	}
	if len(res.Entries) != 1 {
		t.Errorf("entries dropped on container error")
	}
}

func TestAddressMatches(t *testing.T) {
	cases := []struct {
		bind, addr string
		want       bool
	}{
		{"", "0.0.0.0", true},
		{"0.0.0.0", "127.0.0.1", true},
		{"::", "::", true},
		{"127.0.0.1", "127.0.0.1", true},
		{"127.0.0.1", "0.0.0.0", false},
		{"10.0.0.5", "127.0.0.1", false},
	}
	for _, tc := range cases {
		if got := addressMatches(tc.bind, tc.addr); got != tc.want {
			t.Errorf("addressMatches(%q, %q) = %v, want %v", tc.bind, tc.addr, got, tc.want)
		}
	}
}

func TestIsDockerProxy(t *testing.T) {
	for name, want := range map[string]bool{
		"docker-proxy":     true,
		"Docker-Proxy.exe": true,
		"rootlesskit":      true,
		"node":             false,
		"dockerd":          false,
		"":                 false,
	} {
		if got := IsDockerProxy(name); got != want {
			t.Errorf("IsDockerProxy(%q) = %v, want %v", name, got, want)
		}
	}
}
