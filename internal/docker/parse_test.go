package docker

import (
	"os"
	"reflect"
	"testing"

	"github.com/zahidhasann88/portpeek/internal/ports"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}

func TestParseContainersLinuxFixture(t *testing.T) {
	got, err := ParseContainers(readFixture(t, "containers_linux.json"))
	if err != nil {
		t.Fatalf("ParseContainers() error = %v", err)
	}

	web := ports.Container{ID: "7f3a9c2e1b4d", Name: "web", Image: "nginx:1.27-alpine"}
	db := ports.Container{ID: "0123456789ab", Name: "analytics_db", Image: "postgres:16"}
	dns := ports.Container{ID: "fedcba987654", Name: "dns", Image: "coredns/coredns:1.11.1"}

	want := []ports.Binding{
		{HostIP: "0.0.0.0", HostPort: 8080, Protocol: "tcp", Container: web},
		{HostIP: "::", HostPort: 8080, Protocol: "tcp", Container: web},
		{HostIP: "127.0.0.1", HostPort: 5432, Protocol: "tcp", Container: db},
		{HostIP: "0.0.0.0", HostPort: 5353, Protocol: "udp", Container: dns},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseContainers() mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

func TestParseContainersSkipsUnpublishedAndUnknownProtocols(t *testing.T) {
	got, err := ParseContainers(readFixture(t, "containers_linux.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range got {
		if b.HostPort == 0 || b.HostPort == 1234 || b.HostPort == 443 || b.HostPort == 9153 {
			t.Errorf("unexpected binding kept: %+v", b)
		}
	}
}

func TestParseContainersDesktopFixture(t *testing.T) {
	got, err := ParseContainers(readFixture(t, "containers_desktop.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].HostPort != 6379 || got[0].Container.Name != "redis-cache" {
		t.Errorf("unexpected bindings: %+v", got)
	}
}

func TestParseContainersEmpty(t *testing.T) {
	got, err := ParseContainers(readFixture(t, "containers_empty.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %d bindings, want 0", len(got))
	}
}

func TestParseContainersRejectsNonList(t *testing.T) {
	if _, err := ParseContainers(readFixture(t, "not_a_list.json")); err == nil {
		t.Error("expected error for object response")
	}
	if _, err := ParseContainers([]byte("not json")); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestContainerNameFallsBackToShortID(t *testing.T) {
	r := containerRecord{ID: "abcdefabcdef1234", Names: nil}
	if got := containerName(r); got != "abcdefabcdef" {
		t.Errorf("containerName() = %q, want short ID", got)
	}
}
