//go:build !windows

package docker

// defaultSocketPath is the Docker Engine socket on Linux and macOS.
const defaultSocketPath = "/var/run/docker.sock"

func defaultTarget() Target {
	return Target{Network: "unix", Address: defaultSocketPath}
}
