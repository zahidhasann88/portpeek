package ports

import (
	"context"
	"errors"
)

// ErrUnavailable is returned by a ContainerSource when the container runtime
// is not present. Callers treat it as "no container information" and do not
// report an error.
var ErrUnavailable = errors.New("source unavailable")

// ErrAccessDenied is returned when the operating system refuses to reveal or
// change information about a process.
var ErrAccessDenied = errors.New("access denied")

// PortSource lists the sockets known to the operating system.
type PortSource interface {
	Sockets(ctx context.Context) ([]Socket, error)
}

// ProcessSource returns details for a process ID. When some fields are
// unreadable it returns the partial info together with an error wrapping
// ErrAccessDenied. Other errors (for example, the process exited) mean the
// process could not be inspected at all.
type ProcessSource interface {
	Process(ctx context.Context, pid int32) (ProcessInfo, error)
}

// ContainerSource returns the host port bindings published by containers.
type ContainerSource interface {
	Bindings(ctx context.Context) ([]Binding, error)
}

// ProcessControl sends signals to processes. It exists so the kill logic can
// be tested without touching real processes.
type ProcessControl interface {
	// Terminate asks the process to exit (SIGTERM on Unix, TerminateProcess
	// on Windows).
	Terminate(pid int32) error
	// Kill forcibly ends the process (SIGKILL on Unix).
	Kill(pid int32) error
	// Alive reports whether the process still exists.
	Alive(pid int32) (bool, error)
}
