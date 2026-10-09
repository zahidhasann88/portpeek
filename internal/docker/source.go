package docker

import (
	"context"
	"fmt"
	"time"

	"github.com/zahidhasann88/portpeek/internal/ports"
)

// Source implements ports.ContainerSource by querying the local daemon. The
// endpoint is resolved on every call, so DOCKER_HOST is read at run time.
type Source struct {
	// Getenv reads environment variables. It is injectable for tests.
	Getenv func(string) string
	// Timeout bounds each API request. Zero means 3 seconds.
	Timeout time.Duration
}

var _ ports.ContainerSource = Source{}

// Bindings implements ports.ContainerSource.
func (s Source) Bindings(ctx context.Context) ([]ports.Binding, error) {
	target, err := ResolveTarget(s.Getenv)
	if err != nil {
		return nil, fmt.Errorf("resolve Docker endpoint: %w", err)
	}
	dial, err := target.dialer()
	if err != nil {
		return nil, fmt.Errorf("docker endpoint %s: %w", target.Address, err)
	}
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return NewClient(dial, timeout).Bindings(ctx)
}
