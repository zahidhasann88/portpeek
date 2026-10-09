//go:build windows

package docker

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
)

func TestClientOverNamedPipe(t *testing.T) {
	path := fmt.Sprintf(`\\.\pipe\portpeek-test-%d-%d`, os.Getpid(), time.Now().UnixNano())
	ln, err := winio.ListenPipe(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/containers/json" {
			http.Error(w, "unexpected request", 400)
			return
		}
		_, _ = w.Write([]byte(`[{"Id":"abc123","Names":["/web"],"Image":"nginx","Ports":[{"IP":"127.0.0.1","PublicPort":8080,"PrivatePort":80,"Type":"tcp"}]}]`))
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close(); _ = ln.Close() })
	dial, err := pipeDialer(path)
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := NewClient(dial, 2*time.Second).Bindings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || bindings[0].HostPort != 8080 || bindings[0].Container.Name != "web" {
		t.Fatalf("unexpected bindings: %+v", bindings)
	}
}

func TestPipeDialCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dial, err := pipeDialer(`\\.\pipe\portpeek-missing-test-pipe`)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := dial(ctx, "", "")
	if conn != nil {
		_ = conn.Close()
	}
	if err == nil {
		t.Fatal("expected canceled dial to fail")
	}
}
