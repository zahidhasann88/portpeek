package cli

import (
	"os"

	"golang.org/x/term"
)

// isTerminal reports whether f is an interactive terminal. Character devices
// such as /dev/null are not terminals, so a plain file-mode check is not
// enough.
func isTerminal(f *os.File) bool {
	return f != nil && term.IsTerminal(int(f.Fd())) //nolint:gosec // file descriptors fit in int
}
