// Package render formats port entries as a human-readable table, a detailed
// per-port view, or the documented JSON schema. It only writes to the given
// io.Writer and never reads operating-system state.
package render

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/zahidhasann88/portpeek/internal/ports"
)

// ANSI styles. They are applied only when colour is enabled.
const (
	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"
	ansiDim   = "\x1b[2m"
	colGap    = 2
	unknown   = "-"
)

// TableOptions controls the list view.
type TableOptions struct {
	// Color enables ANSI styling. It is off for non-terminal output and when
	// NO_COLOR is set; the caller decides.
	Color bool
	// State adds a STATE column. It is used with --all.
	State bool
}

// Table writes the list view: one row per socket, aligned into columns.
func Table(w io.Writer, entries []ports.Entry, opts TableOptions) error {
	headers := []string{"PORT", "PROTO", "ADDRESS"}
	if opts.State {
		headers = append(headers, "STATE")
	}
	headers = append(headers, "PID", "PROCESS", "USER", "CONTAINER")

	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		row := []string{strconv.Itoa(int(e.Port)), e.Protocol, orDash(e.Address)}
		if opts.State {
			row = append(row, orDash(e.State))
		}
		row = append(row, pidText(e.PID), orDash(e.DisplayProcess()), orDash(e.User), containerText(e.Container))
		rows = append(rows, row)
	}

	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = utf8.RuneCountInString(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if n := utf8.RuneCountInString(cell); n > widths[i] {
				widths[i] = n
			}
		}
	}

	if err := writeRow(w, headers, widths, opts.Color, ansiBold); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writeRow(w, row, widths, opts.Color, ""); err != nil {
			return err
		}
	}
	return nil
}

// writeRow writes one aligned line. Unknown ("-") cells are dimmed when colour
// is on, and headerStyle (if non-empty) styles every cell in the row.
func writeRow(w io.Writer, cells []string, widths []int, color bool, headerStyle string) error {
	var b strings.Builder
	for i, cell := range cells {
		text := cell
		if color {
			switch {
			case headerStyle != "":
				text = headerStyle + cell + ansiReset
			case cell == unknown:
				text = ansiDim + cell + ansiReset
			}
		}
		b.WriteString(text)
		if i < len(cells)-1 {
			b.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(cell)+colGap))
		}
	}
	b.WriteByte('\n')
	_, err := io.WriteString(w, b.String())
	if err != nil {
		return fmt.Errorf("write table: %w", err)
	}
	return nil
}

// DetailOptions controls the single-port view.
type DetailOptions struct {
	Color bool
}

// Detail writes the detailed view. Each entry is a block of labelled fields,
// and blocks are separated by a blank line.
func Detail(w io.Writer, entries []ports.Entry, opts DetailOptions) error {
	for i, e := range entries {
		if i > 0 {
			if _, err := io.WriteString(w, "\n"); err != nil {
				return fmt.Errorf("write detail: %w", err)
			}
		}
		var b strings.Builder
		field := func(label, value string) {
			styled := label + ":"
			if opts.Color {
				styled = ansiBold + label + ":" + ansiReset
			}
			pad := 15 - utf8.RuneCountInString(label) - 1
			if pad < 1 {
				pad = 1
			}
			b.WriteString(styled + strings.Repeat(" ", pad) + value + "\n")
		}
		field("Port", strconv.Itoa(int(e.Port)))
		field("Protocol", e.Protocol)
		field("Family", e.Family)
		field("Address", orDash(e.Address))
		field("State", orDash(e.State))
		field("Remote", orDash(e.RemoteAddress))
		field("PID", pidText(e.PID))
		field("Parent PID", pidText(e.PPID))
		field("Process", orDash(e.ProcessName))
		field("User", orDash(e.User))
		field("Command", orDash(e.Command))
		field("Executable", orDash(e.Executable))
		field("Working dir", orDash(e.WorkingDir))
		field("Started", timeText(e.StartTime))
		if c := e.Container; c != nil {
			field("Container", c.Name)
			field("  ID", c.ID)
			field("  Image", c.Image)
		} else {
			field("Container", unknown)
		}
		if _, err := io.WriteString(w, b.String()); err != nil {
			return fmt.Errorf("write detail: %w", err)
		}
	}
	return nil
}

// orDash returns a printable version of s, or "-" when s is empty. Control
// characters such as newlines (which can appear in command lines) are replaced
// with spaces so that each value stays on one line.
func orDash(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
	if strings.TrimSpace(s) == "" {
		return unknown
	}
	return s
}

func pidText(pid int32) string {
	if pid <= 0 {
		return unknown
	}
	return strconv.Itoa(int(pid))
}

func containerText(c *ports.Container) string {
	if c == nil {
		return unknown
	}
	return fmt.Sprintf("%s (%s)", c.Name, c.ID)
}

func timeText(t time.Time) string {
	if t.IsZero() {
		return unknown
	}
	return t.Format(time.RFC3339)
}
