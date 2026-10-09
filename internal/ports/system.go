package ports

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"syscall"
	"time"

	gnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// SystemSockets lists sockets through gopsutil, on Linux, macOS, and Windows.
type SystemSockets struct{}

var _ PortSource = SystemSockets{}

// Sockets returns all IPv4 and IPv6 TCP and UDP sockets. Owners that the
// current user cannot inspect are reported with PID 0.
func (SystemSockets) Sockets(ctx context.Context) ([]Socket, error) {
	conns, err := gnet.ConnectionsWithContext(ctx, "inet")
	if err != nil {
		return nil, fmt.Errorf("read socket table: %w", err)
	}
	out := make([]Socket, 0, len(conns))
	for _, c := range conns {
		var proto string
		switch c.Type {
		case syscall.SOCK_STREAM:
			proto = ProtocolTCP
		case syscall.SOCK_DGRAM:
			proto = ProtocolUDP
		default:
			continue
		}
		s := Socket{
			Protocol: proto,
			Address:  c.Laddr.IP,
			Port:     uint16(c.Laddr.Port), //nolint:gosec // port is at most 65535
			PID:      c.Pid,
		}
		if proto == ProtocolTCP {
			s.State = c.Status
		} else {
			s.State = StateUnconnect
		}
		if c.Raddr.Port != 0 {
			s.RemoteIP = c.Raddr.IP
			s.RemotePort = uint16(c.Raddr.Port) //nolint:gosec // port is at most 65535
			if proto == ProtocolUDP {
				s.State = StateConnected
			}
		}
		out = append(out, s)
	}
	return out, nil
}

// SystemProcesses inspects processes through gopsutil.
type SystemProcesses struct{}

var _ ProcessSource = SystemProcesses{}

// Process reads process details. Unreadable fields make the returned error
// wrap ErrAccessDenied, and the readable fields are still filled in.
func (SystemProcesses) Process(ctx context.Context, pid int32) (ProcessInfo, error) {
	p, err := process.NewProcessWithContext(ctx, pid)
	if err != nil {
		if isPermission(err) {
			return ProcessInfo{}, fmt.Errorf("open process %d: %w: %w", pid, ErrAccessDenied, err)
		}
		return ProcessInfo{}, fmt.Errorf("open process %d: %w", pid, err)
	}

	var info ProcessInfo
	var denied error
	note := func(err error) {
		if err != nil && denied == nil && isPermission(err) {
			denied = err
		}
	}

	name, err := p.NameWithContext(ctx)
	if err != nil {
		// Without a name the process is effectively gone or unreadable.
		if isPermission(err) {
			return info, fmt.Errorf("read name of process %d: %w: %w", pid, ErrAccessDenied, err)
		}
		return info, fmt.Errorf("read name of process %d: %w", pid, err)
	}
	info.Name = name
	note(err)

	info.Executable, err = p.ExeWithContext(ctx)
	note(err)
	info.Command, err = p.CmdlineWithContext(ctx)
	note(err)
	info.WorkingDir, err = p.CwdWithContext(ctx)
	note(err)
	info.User, err = p.UsernameWithContext(ctx)
	note(err)
	if ppid, err := p.PpidWithContext(ctx); err == nil {
		info.PPID = ppid
	}
	if created, err := p.CreateTimeWithContext(ctx); err == nil && created > 0 {
		info.StartTime = time.UnixMilli(created).Local()
	}

	if denied != nil {
		return info, fmt.Errorf("process %d: %w: %w", pid, ErrAccessDenied, denied)
	}
	return info, nil
}

// SystemControl signals processes through gopsutil.
type SystemControl struct{}

var _ ProcessControl = SystemControl{}

// Terminate asks a process to exit.
func (SystemControl) Terminate(pid int32) error {
	p, err := process.NewProcess(pid)
	if err != nil {
		return fmt.Errorf("open process %d: %w", pid, err)
	}
	if err := p.Terminate(); err != nil {
		return signalError(pid, err)
	}
	return nil
}

// Kill forcibly ends a process.
func (SystemControl) Kill(pid int32) error {
	p, err := process.NewProcess(pid)
	if err != nil {
		return fmt.Errorf("open process %d: %w", pid, err)
	}
	if err := p.Kill(); err != nil {
		return signalError(pid, err)
	}
	return nil
}

// Alive reports whether a process exists. A missing process is not an error.
func (SystemControl) Alive(pid int32) (bool, error) {
	ok, err := process.PidExists(pid)
	if err != nil {
		return false, fmt.Errorf("check process %d: %w", pid, err)
	}
	return ok, nil
}

func signalError(pid int32, err error) error {
	if isPermission(err) {
		return fmt.Errorf("signal process %d: %w: %w", pid, ErrAccessDenied, err)
	}
	return fmt.Errorf("signal process %d: %w", pid, err)
}

func isPermission(err error) bool {
	return errors.Is(err, fs.ErrPermission) || errors.Is(err, process.ErrorNotPermitted)
}
