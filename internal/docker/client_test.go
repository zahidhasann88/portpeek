package docker

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/zahidhasann88/portpeek/internal/ports"
)

// serverDialer sends every connection to an in-process test server, ignoring
// the target. It lets the real client code run without a Docker daemon.
func serverDialer(addr string) DialFunc {
	d := &net.Dialer{Timeout: time.Second}
	return func(ctx context.Context, _, _ string) (net.Conn, error) {
		return d.DialContext(ctx, "tcp", addr)
	}
}

func TestClientBindingsOverFakeDaemon(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/containers/json" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		http.ServeFile(w, r, "testdata/containers_desktop.json")
	}))
	defer srv.Close()

	client := NewClient(serverDialer(srv.Listener.Addr().String()), 2*time.Second)
	got, err := client.Bindings(context.Background())
	if err != nil {
		t.Fatalf("Bindings() error = %v", err)
	}
	if len(got) != 1 || got[0].HostPort != 6379 {
		t.Errorf("Bindings() = %+v", got)
	}
}

func TestClientNon200IsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewClient(serverDialer(srv.Listener.Addr().String()), 2*time.Second)
	_, err := client.Bindings(context.Background())
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
	if errors.Is(err, ports.ErrUnavailable) {
		t.Error("HTTP error must not be reported as unavailable")
	}
}

func TestClientMissingSocketIsUnavailable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket paths are not used on Windows")
	}
	missing := filepath.Join(t.TempDir(), "absent.sock")
	target := Target{Network: "unix", Address: missing}
	dial, err := target.dialer()
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewClient(dial, time.Second).Bindings(context.Background())
	if !errors.Is(err, ports.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestClientRefusedConnectionIsUnavailable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close() // nothing listens on addr now
	_, err = NewClient(serverDialer(addr), time.Second).Bindings(context.Background())
	if !errors.Is(err, ports.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestClientPermissionDeniedIsReportedNotSkipped(t *testing.T) {
	dial := func(context.Context, string, string) (net.Conn, error) {
		return nil, &net.OpError{Op: "dial", Net: "unix", Err: &os.PathError{Op: "dial", Path: "/var/run/docker.sock", Err: fs.ErrPermission}}
	}
	_, err := NewClient(dial, time.Second).Bindings(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ports.ErrUnavailable) {
		t.Error("permission error must not be silently treated as unavailable")
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("error should wrap fs.ErrPermission: %v", err)
	}
}

func TestSourceHonoursDockerHostAndUnavailable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket paths are not used on Windows")
	}
	src := Source{Getenv: env(map[string]string{"DOCKER_HOST": "unix://" + filepath.Join(t.TempDir(), "none.sock")})}
	_, err := src.Bindings(context.Background())
	if !errors.Is(err, ports.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestSourceBadDockerHostIsNotUnavailable(t *testing.T) {
	src := Source{Getenv: env(map[string]string{"DOCKER_HOST": "ssh://nope"})}
	_, err := src.Bindings(context.Background())
	if err == nil || errors.Is(err, ports.ErrUnavailable) {
		t.Fatalf("err = %v, want a configuration error", err)
	}
}
