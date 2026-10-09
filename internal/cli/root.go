// Package cli implements the portpeek command line. It parses arguments,
// calls the port collector, and hands results to the render package. It
// contains no operating-system logic of its own; that is injected via Deps.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zahidhasann88/portpeek/internal/ports"
	"github.com/zahidhasann88/portpeek/internal/render"
)

// globalFlags are the flags shared by every command.
type globalFlags struct {
	JSON     bool
	TCP      bool
	UDP      bool
	All      bool
	NoDocker bool
	NoColor  bool
}

// Run executes the CLI with args (without the program name) and returns the
// process exit code.
func Run(args []string, d Deps) int {
	root := newRootCmd(d)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		return reportError(d.Stderr, err)
	}
	return ExitOK
}

// reportError prints err to stderr and returns its exit code. Errors that did
// not come from a command (unknown flags, wrong argument count) are usage
// errors.
func reportError(w io.Writer, err error) int {
	var ee *exitError
	if errors.As(err, &ee) {
		if ee.code == ExitUsage {
			_, _ = fmt.Fprintf(w, "portpeek: %v\nRun 'portpeek --help' for usage.\n", ee.err)
		} else {
			_, _ = fmt.Fprintf(w, "portpeek: %v\n", ee.err)
		}
		return ee.code
	}
	_, _ = fmt.Fprintf(w, "portpeek: %v\nRun 'portpeek --help' for usage.\n", err)
	return ExitUsage
}

func newRootCmd(d Deps) *cobra.Command {
	var g globalFlags
	root := &cobra.Command{
		Use:   "portpeek [port]",
		Short: "Show which processes and containers use local network ports",
		Long: `portpeek lists the listening sockets on this machine together with the
process, user, and Docker container that owns each one.

Run with no arguments to list all listening sockets. Pass a port number to
see details for that port. Use "portpeek kill <port>" to stop the process
that holds a port.`,
		Example: `  portpeek
  portpeek --tcp --json
  portpeek 3000
  portpeek kill 3000 --yes`,
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       versionString(d.Build),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return runList(cmd.Context(), d, g)
			}
			port, err := parsePort(args[0])
			if err != nil {
				return err
			}
			return runPort(cmd.Context(), d, g, port)
		},
	}
	root.SetVersionTemplate("portpeek {{.Version}}\n")
	root.SetOut(d.Stdout)
	root.SetErr(d.Stderr)

	pf := root.PersistentFlags()
	pf.BoolVar(&g.JSON, "json", false, "print results as JSON (see README for the schema)")
	pf.BoolVar(&g.TCP, "tcp", false, "show only TCP sockets")
	pf.BoolVar(&g.UDP, "udp", false, "show only UDP sockets")
	pf.BoolVar(&g.All, "all", false, "include non-listening sockets such as established connections")
	pf.BoolVar(&g.NoDocker, "no-docker", false, "do not query the Docker daemon for container information")
	pf.BoolVar(&g.NoColor, "no-color", false, "disable coloured output (also disabled by NO_COLOR and non-terminal output)")

	root.AddCommand(newVersionCmd(d))
	root.AddCommand(newKillCmd(d, &g))
	return root
}

func versionString(b BuildInfo) string {
	return fmt.Sprintf("%s (commit %s, built %s)", orDefault(b.Version, "dev"), orDefault(b.Commit, "none"), orDefault(b.Date, "unknown"))
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func newVersionCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version, commit, and build date",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(d.Stdout, "portpeek %s\n", versionString(d.Build))
			if err != nil {
				return runtimeError(fmt.Errorf("write version: %w", err))
			}
			return nil
		},
	}
}

// parsePort validates a port argument.
func parsePort(arg string) (uint16, error) {
	n, err := strconv.ParseUint(strings.TrimSpace(arg), 10, 16)
	if err != nil || n == 0 {
		return 0, usageError("invalid port %q: must be a number from 1 to 65535", arg)
	}
	return uint16(n), nil
}

// optionsFrom builds collector options from global flags.
func optionsFrom(g globalFlags, port uint16) ports.Options {
	return ports.Options{TCP: g.TCP, UDP: g.UDP, All: g.All, Port: port}
}

// collect runs the collector with the injected sources.
func collect(parent context.Context, d Deps, g globalFlags, opts ports.Options) (ports.Result, error) {
	ctx, cancel := context.WithTimeout(parent, collectTimeout)
	defer cancel()

	c := ports.Collector{Sockets: d.Sockets, Processes: d.Processes}
	if !g.NoDocker {
		c.Containers = d.Containers
	}
	res, err := c.Collect(ctx, opts)
	if err != nil {
		return res, runtimeError(err)
	}
	return res, nil
}

// colorEnabled applies the colour rules: never for JSON, never for
// non-terminal output, never when NO_COLOR is set (non-empty) or --no-color is
// given.
func colorEnabled(d Deps, g globalFlags) bool {
	return !g.NoColor && !g.JSON && d.StdoutTTY && d.Getenv("NO_COLOR") == ""
}

func runList(ctx context.Context, d Deps, g globalFlags) error {
	res, err := collect(ctx, d, g, optionsFrom(g, 0))
	if err != nil {
		return err
	}
	printWarnings(d, res)
	if g.JSON {
		if err := render.JSON(d.Stdout, res.Entries); err != nil {
			return runtimeError(err)
		}
	} else {
		opts := render.TableOptions{Color: colorEnabled(d, g), State: g.All}
		if err := render.Table(d.Stdout, res.Entries, opts); err != nil {
			return runtimeError(err)
		}
	}
	printPermissionHint(d, res)
	return nil
}

func runPort(ctx context.Context, d Deps, g globalFlags, port uint16) error {
	res, err := collect(ctx, d, g, optionsFrom(g, port))
	if err != nil {
		return err
	}
	printWarnings(d, res)
	if len(res.Entries) == 0 {
		if g.JSON {
			if err := render.JSON(d.Stdout, nil); err != nil {
				return runtimeError(err)
			}
		}
		return notInUseError(port)
	}
	if g.JSON {
		if err := render.JSON(d.Stdout, res.Entries); err != nil {
			return runtimeError(err)
		}
	} else {
		if err := render.Detail(d.Stdout, res.Entries, render.DetailOptions{Color: colorEnabled(d, g)}); err != nil {
			return runtimeError(err)
		}
	}
	printPermissionHint(d, res)
	return nil
}

// printWarnings writes non-fatal collection problems to stderr.
func printWarnings(d Deps, res ports.Result) {
	for _, w := range res.Warnings {
		_, _ = fmt.Fprintf(d.Stderr, "portpeek: warning: %s\n", w)
	}
}

// printPermissionHint prints one concise hint when some process details were
// hidden by the operating system.
func printPermissionHint(d Deps, res ports.Result) {
	if res.Restricted == 0 || d.Elevated {
		return
	}
	if d.GOOS == "windows" {
		_, _ = fmt.Fprintln(d.Stderr, "hint: some process details are hidden; run as Administrator to show them (shown as \"-\").")
		return
	}
	_, _ = fmt.Fprintln(d.Stderr, "hint: some process details are hidden (other users); re-run with sudo to show them (shown as \"-\").")
}
