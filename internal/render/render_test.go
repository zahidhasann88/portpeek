package render

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zahidhasann88/portpeek/internal/ports"
)

var update = flag.Bool("update", false, "rewrite golden files in testdata/")

var started = time.Date(2026, 10, 9, 8, 30, 0, 0, time.UTC)

// sampleEntries covers the shapes the renderer must handle: a fully known
// process, an IPv6 listener, a UDP socket, a hidden owner, a Docker-published
// port, and an ESTABLISHED connection (only shown with --all).
func sampleEntries() []ports.Entry {
	return []ports.Entry{
		{
			Port: 22, Protocol: ports.ProtocolTCP, Family: ports.FamilyIPv4, Address: "0.0.0.0",
			State: ports.StateListen, PID: 812, PPID: 1, ProcessName: "sshd", User: "root",
			Command: "sshd: /usr/sbin/sshd -D [listener] 0 of 10-100 startups", Executable: "/usr/sbin/sshd",
			WorkingDir: "/", StartTime: started,
		},
		{
			Port: 3000, Protocol: ports.ProtocolTCP, Family: ports.FamilyIPv6, Address: "::",
			State: ports.StateListen, PID: 4311, PPID: 4300, ProcessName: "node", User: "alice",
			Command: "node server.js --port 3000", Executable: "/usr/bin/node",
			WorkingDir: "/home/alice/app", StartTime: started.Add(2 * time.Hour),
		},
		{
			Port: 5353, Protocol: ports.ProtocolUDP, Family: ports.FamilyIPv4, Address: "0.0.0.0",
			State: ports.StateUnconnect, PID: 0,
		},
		{
			Port: 5432, Protocol: ports.ProtocolTCP, Family: ports.FamilyIPv4, Address: "127.0.0.1",
			State: ports.StateListen, PID: 0,
			Container: &ports.Container{ID: "0123456789ab", Name: "analytics_db", Image: "postgres:16"},
		},
		{
			Port: 8080, Protocol: ports.ProtocolTCP, Family: ports.FamilyIPv4, Address: "0.0.0.0",
			State: ports.StateListen, PID: 2100, ProcessName: "docker-proxy", User: "root",
			Command:   "/usr/bin/docker-proxy -proto tcp -host-ip 0.0.0.0 -host-port 8080",
			Container: &ports.Container{ID: "7f3a9c2e1b4d", Name: "web", Image: "nginx:1.27-alpine"},
		},
		{
			Port: 8080, Protocol: ports.ProtocolTCP, Family: ports.FamilyIPv4, Address: "127.0.0.1",
			State: "ESTABLISHED", PID: 4311, ProcessName: "node", User: "alice",
			RemoteAddress: "127.0.0.1:51844",
		},
	}
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("update %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run go test -update): %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("output differs from %s (run go test -update to accept)\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

func renderTable(t *testing.T, entries []ports.Entry, opts TableOptions) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := Table(&buf, entries, opts); err != nil {
		t.Fatalf("Table() error = %v", err)
	}
	return buf.Bytes()
}

func TestTableDefaultGolden(t *testing.T) {
	golden(t, "table_default", renderTable(t, sampleEntries()[:5], TableOptions{}))
}

func TestTableAllGolden(t *testing.T) {
	golden(t, "table_all", renderTable(t, sampleEntries(), TableOptions{State: true}))
}

func TestTableColorGolden(t *testing.T) {
	golden(t, "table_color", renderTable(t, sampleEntries()[:4], TableOptions{Color: true}))
}

func TestTableNoANSIWhenColorDisabled(t *testing.T) {
	out := renderTable(t, sampleEntries(), TableOptions{State: true})
	if bytes.Contains(out, []byte("\x1b[")) {
		t.Errorf("plain table contains ANSI escapes:\n%s", out)
	}
}

func TestTableEmptyWritesHeaderOnly(t *testing.T) {
	out := string(renderTable(t, nil, TableOptions{}))
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "PORT") {
		t.Errorf("empty table = %q, want header only", out)
	}
}

func TestDockerProxyShowsContainerInProcessColumn(t *testing.T) {
	out := string(renderTable(t, sampleEntries()[4:5], TableOptions{}))
	if !strings.Contains(out, "web (7f3a9c2e1b4d)") {
		t.Errorf("container missing from CONTAINER column:\n%s", out)
	}
	fields := strings.Fields(strings.Split(out, "\n")[1])
	if fields[4] != "web" {
		t.Errorf("PROCESS column = %q, want container name %q", fields[4], "web")
	}
}

func TestDetailSingleGolden(t *testing.T) {
	var buf bytes.Buffer
	if err := Detail(&buf, sampleEntries()[1:2], DetailOptions{}); err != nil {
		t.Fatal(err)
	}
	golden(t, "detail_single", buf.Bytes())
}

func TestDetailContainerGolden(t *testing.T) {
	var buf bytes.Buffer
	if err := Detail(&buf, []ports.Entry{sampleEntries()[4], sampleEntries()[3]}, DetailOptions{}); err != nil {
		t.Fatal(err)
	}
	golden(t, "detail_container", buf.Bytes())
}

func TestJSONGolden(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, sampleEntries()); err != nil {
		t.Fatal(err)
	}
	golden(t, "json_all", buf.Bytes())
}

func TestJSONEmptyIsArray(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, nil); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != "[]" {
		t.Errorf("empty JSON = %q, want []", got)
	}
}

func TestJSONUsesNullForUnknownAndSnakeCaseKeys(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, sampleEntries()[2:3]); err != nil {
		t.Fatal(err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	obj := decoded[0]
	for _, key := range []string{"pid", "process_name", "user", "started_at", "container", "remote_address", "command"} {
		v, ok := obj[key]
		if !ok {
			t.Errorf("key %q missing", key)
			continue
		}
		if v != nil {
			t.Errorf("key %q = %v, want null", key, v)
		}
	}
	for key := range obj {
		if strings.ToLower(key) != key || strings.Contains(key, " ") {
			t.Errorf("key %q is not snake_case", key)
		}
	}
	if obj["protocol"] != "udp" || obj["port"] != float64(5353) {
		t.Errorf("unexpected core fields: %v", obj)
	}
}
