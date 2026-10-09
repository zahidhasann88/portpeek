package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zahidhasann88/portpeek/internal/ports"
	"github.com/zahidhasann88/portpeek/internal/render"
)

func newKillCmd(d Deps, g *globalFlags) *cobra.Command {
	var yes, force bool
	cmd := &cobra.Command{
		Use:   "kill <port>",
		Short: "Stop the process that holds a port",
		Long: `kill stops the process that is listening on a port.

It sends SIGTERM and waits for the process to exit. With --force, it sends
SIGKILL if the process is still running after the grace period. Without --yes
it asks for confirmation. Protected targets (PID 0, PID 1, portpeek itself,
and container-owned ports) are always refused.`,
		Example: `  portpeek kill 3000
  portpeek kill 8080 --yes
  portpeek kill 9000 --yes --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			port, err := parsePort(args[0])
			if err != nil {
				return err
			}
			return runKill(cmd.Context(), d, *g, port, killFlags{yes: yes, force: force})
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask for confirmation")
	cmd.Flags().BoolVar(&force, "force", false, "send SIGKILL if SIGTERM does not stop the process")
	return cmd
}

type killFlags struct {
	yes   bool
	force bool
}

// runKill implements "portpeek kill <port>".
func runKill(ctx context.Context, d Deps, g globalFlags, port uint16, f killFlags) error {
	opts := ports.Options{TCP: g.TCP, UDP: g.UDP, Port: port}
	res, err := collect(ctx, d, g, opts)
	if err != nil {
		return err
	}
	target, err := ports.SelectTarget(res.Entries, port, opts, d.Self)
	if err != nil {
		return killError(err, port, d.GOOS)
	}

	if !f.yes {
		prompt := fmt.Sprintf("Kill PID %d (%s, user %s) holding %s port %d? [y/N] ",
			target.PID, dashIfEmpty(target.ProcessName), dashIfEmpty(target.User), target.Protocol, port)
		ok, err := confirm(d, prompt)
		if err != nil {
			return err
		}
		if !ok {
			if g.JSON {
				if err := render.JSON(d.Stdout, nil); err != nil {
					return runtimeError(err)
				}
			} else {
				_, _ = fmt.Fprintln(d.Stdout, "Aborted. Nothing was killed.")
			}
			return nil
		}
	}

	result, err := ports.Stop(d.Control, target.PID, f.force, d.Timing)
	if err != nil {
		return killError(err, port, d.GOOS)
	}
	if g.JSON {
		if err := render.JSON(d.Stdout, []ports.Entry{target}); err != nil {
			return runtimeError(err)
		}
		return nil
	}
	name := dashIfEmpty(target.ProcessName)
	switch result {
	case ports.KillExitedOnTerm:
		if d.GOOS == "windows" {
			_, err = fmt.Fprintf(d.Stdout, "Terminated PID %d (%s); it exited. Port %d is free.\n", target.PID, name, port)
			break
		}
		_, err = fmt.Fprintf(d.Stdout, "Sent SIGTERM to PID %d (%s); it exited. Port %d is free.\n", target.PID, name, port)
	case ports.KillForced:
		_, err = fmt.Fprintf(d.Stdout, "Sent SIGTERM, then SIGKILL, to PID %d (%s); it exited. Port %d is free.\n", target.PID, name, port)
	case ports.KillAlreadyGone:
		_, err = fmt.Fprintf(d.Stdout, "PID %d (%s) had already exited. Port %d is free.\n", target.PID, name, port)
	}
	if err != nil {
		return runtimeError(fmt.Errorf("write result: %w", err))
	}
	return nil
}

// killError maps selection and signalling errors to exit codes.
func killError(err error, port uint16, goos string) error {
	switch {
	case errors.Is(err, ports.ErrNotInUse):
		return notInUseError(port)
	case errors.Is(err, ports.ErrAmbiguousOwner):
		return usageError("%v", err)
	case errors.Is(err, ports.ErrOwnerUnknown), errors.Is(err, ports.ErrAccessDenied):
		if goos == "windows" {
			return runtimeError(fmt.Errorf("%w; run as Administrator to inspect or stop the owner", err))
		}
		return runtimeError(fmt.Errorf("%w; re-run with sudo to see every owner", err))
	default:
		return runtimeError(err)
	}
}

// confirm asks the user to confirm the kill. It returns false if the user
// declines. Without a terminal it is a usage error, so scripts must pass --yes.
func confirm(d Deps, prompt string) (bool, error) {
	if !d.StdinTTY {
		return false, usageError("refusing to kill without confirmation: stdin is not a terminal (pass --yes to confirm)")
	}
	if _, err := fmt.Fprint(d.Stderr, prompt); err != nil {
		return false, runtimeError(fmt.Errorf("write prompt: %w", err))
	}
	answer, err := readLine(d.Stdin)
	if err != nil {
		return false, runtimeError(fmt.Errorf("read confirmation: %w", err))
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

// readLine reads up to the first newline, returning what was read so far at
// end of input.
func readLine(r io.Reader) (string, error) {
	var sb strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				return sb.String(), nil
			}
			sb.WriteByte(buf[0])
		}
		if err != nil {
			if err == io.EOF {
				return sb.String(), nil
			}
			return sb.String(), err
		}
	}
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
