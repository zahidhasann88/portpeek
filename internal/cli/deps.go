package cli

import (
	"io"
	"os"
	"runtime"
	"time"

	"github.com/zahidhasann88/portpeek/internal/docker"
	"github.com/zahidhasann88/portpeek/internal/ports"
)

// BuildInfo is set at build time via -ldflags.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Deps holds everything the CLI needs from the outside world. Tests replace
// the sources and writers with fakes.
type Deps struct {
	Sockets    ports.PortSource
	Processes  ports.ProcessSource
	Containers ports.ContainerSource
	Control    ports.ProcessControl

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// StdoutTTY and StdinTTY report whether the streams are terminals.
	StdoutTTY bool
	StdinTTY  bool
	// Elevated reports whether the process already has administrator or root
	// rights, in which case the sudo hint is not shown.
	Elevated bool

	Getenv func(string) string
	// Self is the PID of this process. It must never be signalled.
	Self int32
	// GOOS selects platform-specific hint text.
	GOOS string

	Build  BuildInfo
	Timing ports.KillTiming
}

// NewDefaultDeps wires the real operating-system implementations.
func NewDefaultDeps(build BuildInfo) Deps {
	return Deps{
		Sockets:    ports.SystemSockets{},
		Processes:  ports.SystemProcesses{},
		Containers: docker.Source{Getenv: os.Getenv},
		Control:    ports.SystemControl{},
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		StdoutTTY:  isTerminal(os.Stdout),
		StdinTTY:   isTerminal(os.Stdin),
		Elevated:   os.Geteuid() == 0,
		Getenv:     os.Getenv,
		Self:       int32(os.Getpid()), //nolint:gosec // PIDs fit in int32 on all supported platforms
		GOOS:       runtime.GOOS,
		Build:      build,
		Timing:     ports.DefaultKillTiming(),
	}
}

// collectTimeout bounds one full collection run.
const collectTimeout = 30 * time.Second
