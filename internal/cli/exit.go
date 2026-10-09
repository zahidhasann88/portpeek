package cli

import "fmt"

// Exit codes. They are part of the public interface and are documented in the
// README.
const (
	// ExitOK means success.
	ExitOK = 0
	// ExitNotInUse means a specific port has no matching socket.
	ExitNotInUse = 1
	// ExitUsage means invalid arguments or flags, or an action that needs
	// confirmation when none was given.
	ExitUsage = 2
	// ExitRuntime means a permission problem or a failure while reading the
	// system or signalling a process, or a refused unsafe action.
	ExitRuntime = 3
)

// exitError carries the process exit code along with the error message.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

func usageError(format string, args ...any) error {
	return &exitError{code: ExitUsage, err: fmt.Errorf(format, args...)}
}

func runtimeError(err error) error {
	return &exitError{code: ExitRuntime, err: err}
}

func notInUseError(port uint16) error {
	return &exitError{code: ExitNotInUse, err: fmt.Errorf("port %d is not in use", port)}
}
