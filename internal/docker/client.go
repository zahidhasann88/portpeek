package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"time"

	"github.com/zahidhasann88/portpeek/internal/ports"
)

// DialFunc connects to the Docker daemon. The address argument supplied by
// net/http is ignored; the daemon location is fixed by the Target.
type DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

const (
	containersPath   = "/containers/json"
	maxResponseBytes = 16 << 20
)

// Client queries the Docker Engine API.
type Client struct {
	http *http.Client
}

// NewClient returns a Client that connects with dial and gives each request
// at most timeout.
func NewClient(dial DialFunc, timeout time.Duration) *Client {
	transport := &http.Transport{
		DialContext:       dial,
		DisableKeepAlives: true,
		Proxy:             nil, // never route the local daemon through HTTP_PROXY
	}
	return &Client{http: &http.Client{Transport: transport, Timeout: timeout}}
}

// Bindings lists running containers and returns their published host ports.
//
// When the daemon socket does not exist or refuses connections, the error
// wraps ports.ErrUnavailable. Permission problems return a distinct error so
// the caller can tell the user about them.
func (c *Client) Bindings(ctx context.Context) ([]ports.Binding, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker"+containersPath, nil)
	if err != nil {
		return nil, fmt.Errorf("build Docker request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, classifyError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("docker API %s returned %s", containersPath, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read Docker API response: %w", err)
	}
	return ParseContainers(body)
}

// classifyError maps transport errors to ErrUnavailable when the daemon is not
// reachable at all.
func classifyError(err error) error {
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		if errors.Is(err, fs.ErrPermission) {
			return fmt.Errorf("permission denied connecting to Docker (is your user in the docker group?): %w", err)
		}
		return fmt.Errorf("%w: %w", ports.ErrUnavailable, err)
	}
	return fmt.Errorf("query Docker: %w", err)
}
