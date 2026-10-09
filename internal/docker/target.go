// Package docker reads published container ports from the Docker Engine HTTP
// API. It talks to the daemon over the Unix socket (or named pipe on Windows)
// with net/http, so it has no dependency on the Docker SDK.
package docker

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// dialTimeout bounds how long connecting to the daemon may take.
const dialTimeout = 2 * time.Second

// Target is where the Docker daemon listens.
type Target struct {
	// Network is "unix", "npipe" (Windows named pipe), or "tcp".
	Network string
	// Address is the socket path, pipe path, or host:port.
	Address string
}

// ResolveTarget picks the daemon endpoint from DOCKER_HOST, falling back to the
// platform default. Supported schemes are unix://, npipe://, and tcp:// (tcp
// without TLS).
func ResolveTarget(getenv func(string) string) (Target, error) {
	raw := strings.TrimSpace(getenv("DOCKER_HOST"))
	if raw == "" {
		return defaultTarget(), nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Target{}, fmt.Errorf("parse DOCKER_HOST %q: %w", raw, err)
	}
	switch u.Scheme {
	case "unix":
		if u.Path == "" {
			return Target{}, fmt.Errorf("DOCKER_HOST %q has no socket path", raw)
		}
		return Target{Network: "unix", Address: u.Path}, nil
	case "npipe":
		path := u.Host + u.Path
		if path == "" {
			return Target{}, fmt.Errorf("DOCKER_HOST %q has no pipe path", raw)
		}
		return Target{Network: "npipe", Address: path}, nil
	case "tcp":
		if u.Host == "" {
			return Target{}, fmt.Errorf("DOCKER_HOST %q has no host", raw)
		}
		return Target{Network: "tcp", Address: u.Host}, nil
	default:
		return Target{}, fmt.Errorf("unsupported DOCKER_HOST scheme %q", u.Scheme)
	}
}

// dialer returns a DialFunc that connects to the target.
func (t Target) dialer() (DialFunc, error) {
	if t.Network == "npipe" {
		return pipeDialer(t.Address)
	}
	d := &net.Dialer{Timeout: dialTimeout}
	return func(ctx context.Context, _, _ string) (net.Conn, error) {
		return d.DialContext(ctx, t.Network, t.Address)
	}, nil
}
