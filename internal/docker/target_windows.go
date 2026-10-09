//go:build windows

package docker

// defaultPipePath is the Docker Engine named pipe on Windows.
const defaultPipePath = `//./pipe/docker_engine`

func defaultTarget() Target {
	return Target{Network: "npipe", Address: defaultPipePath}
}
