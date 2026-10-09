//go:build windows

package docker

import (
	"context"
	"net"
	"strings"

	"github.com/Microsoft/go-winio"
)

func pipeDialer(path string) (DialFunc, error) {
	winPath := strings.ReplaceAll(path, "/", `\`)
	return func(ctx context.Context, _, _ string) (net.Conn, error) {
		conn, err := winio.DialPipeContext(ctx, winPath)
		if err != nil {
			return nil, &net.OpError{Op: "dial", Net: "npipe", Err: err}
		}
		return conn, nil
	}, nil
}
