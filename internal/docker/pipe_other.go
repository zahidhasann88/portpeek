//go:build !windows

package docker

import (
	"errors"
)

// pipeDialer reports that named pipes are only available on Windows.
func pipeDialer(path string) (DialFunc, error) {
	return nil, errors.New("named pipes (npipe://) are only supported on Windows: " + path)
}
