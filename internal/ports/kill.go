package ports

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"
)

// Errors returned by target selection and signalling. The CLI maps them to
// exit codes.
var (
	ErrNotInUse         = errors.New("port is not in use")
	ErrOwnerUnknown     = errors.New("owning process is not visible")
	ErrAmbiguousOwner   = errors.New("more than one process holds the port")
	ErrProtectedProcess = errors.New("refusing to kill a protected process")
	ErrContainerOwned   = errors.New("port is published by a container")
	ErrStillRunning     = errors.New("process is still running after SIGTERM")
)

// KillTiming controls how long Stop waits after each signal.
type KillTiming struct {
	// Grace is how long to wait for the process to exit after SIGTERM.
	Grace time.Duration
	// ForceGrace is how long to wait after SIGKILL.
	ForceGrace time.Duration
	// Poll is the interval between liveness checks.
	Poll time.Duration
}

// DefaultKillTiming returns the timing used by the CLI.
func DefaultKillTiming() KillTiming {
	return KillTiming{Grace: 5 * time.Second, ForceGrace: 2 * time.Second, Poll: 100 * time.Millisecond}
}

// KillResult describes how a process ended.
type KillResult int

const (
	// KillExitedOnTerm means the process exited after SIGTERM.
	KillExitedOnTerm KillResult = iota + 1
	// KillForced means SIGKILL was needed.
	KillForced
	// KillAlreadyGone means the process had already exited.
	KillAlreadyGone
)

// SelectTarget chooses the one listening entry that kill should stop. entries
// should come from a Collector run with the same port and protocol options.
//
// It refuses when the port is free, when the owner is not visible, when more
// than one process holds the port, and when the target is protected (see
// CheckKillable).
func SelectTarget(entries []Entry, port uint16, opts Options, self int32) (Entry, error) {
	var candidates []Entry
	for _, e := range entries {
		if e.Port != port || !e.IsListening() || !opts.includesProtocol(e.Protocol) {
			continue
		}
		candidates = append(candidates, e)
	}
	if len(candidates) == 0 {
		return Entry{}, fmt.Errorf("port %d: %w", port, ErrNotInUse)
	}

	pids := make(map[int32]bool)
	var ordered []int32
	for _, e := range candidates {
		if e.PID <= 0 {
			return Entry{}, fmt.Errorf("port %d: %w: a listener on this port belongs to a process that cannot be inspected", port, ErrOwnerUnknown)
		}
		if !pids[e.PID] {
			pids[e.PID] = true
			ordered = append(ordered, e.PID)
		}
	}
	if len(ordered) > 1 {
		return Entry{}, fmt.Errorf("port %d: %w: PIDs %s; narrow the choice with --tcp or --udp",
			port, ErrAmbiguousOwner, joinPIDs(ordered))
	}

	target := candidates[0]
	for _, candidate := range candidates {
		if err := CheckKillable(candidate, self); err != nil {
			return Entry{}, err
		}
	}
	return target, nil
}

// CheckKillable applies the safety rules to one entry. It refuses PID 0 and
// PID 1, the Windows System process (PID 4), portpeek's own process, and any
// socket published by a container runtime, because stopping the container
// helper would not free the port in a safe way.
func CheckKillable(e Entry, self int32) error {
	switch {
	case e.PID <= 1:
		return fmt.Errorf("PID %d: %w", e.PID, ErrProtectedProcess)
	case runtime.GOOS == "windows" && e.PID == 4:
		return fmt.Errorf("PID %d is the Windows System process: %w", e.PID, ErrProtectedProcess)
	case self > 0 && e.PID == self:
		return fmt.Errorf("PID %d is portpeek itself: %w", e.PID, ErrProtectedProcess)
	case e.Container != nil:
		return fmt.Errorf("port %d: %w; stop the container instead: docker stop %s",
			e.Port, ErrContainerOwned, e.Container.Name)
	case IsDockerProxy(e.ProcessName):
		return fmt.Errorf("port %d: %w; stop the container that publishes it, not %s",
			e.Port, ErrContainerOwned, e.ProcessName)
	}
	return nil
}

// Stop ends pid. It sends SIGTERM and waits up to t.Grace for the process to
// exit. If force is set and the process is still running, it sends SIGKILL and
// waits up to t.ForceGrace. Without force, a process that ignores SIGTERM is
// reported as ErrStillRunning.
func Stop(ctl ProcessControl, pid int32, force bool, t KillTiming) (KillResult, error) {
	if err := ctl.Terminate(pid); err != nil {
		if gone := processGone(ctl, pid); gone {
			return KillAlreadyGone, nil
		}
		return 0, fmt.Errorf("send SIGTERM to PID %d: %w", pid, err)
	}
	exited, err := waitForExit(ctl, pid, t.Grace, t.Poll)
	if err != nil {
		return 0, err
	}
	if exited {
		return KillExitedOnTerm, nil
	}
	if !force {
		return 0, fmt.Errorf("PID %d: %w; re-run with --force to send SIGKILL", pid, ErrStillRunning)
	}

	if err := ctl.Kill(pid); err != nil {
		if gone := processGone(ctl, pid); gone {
			return KillForced, nil
		}
		return 0, fmt.Errorf("send SIGKILL to PID %d: %w", pid, err)
	}
	exited, err = waitForExit(ctl, pid, t.ForceGrace, t.Poll)
	if err != nil {
		return 0, err
	}
	if !exited {
		return 0, fmt.Errorf("PID %d: %w after SIGKILL", pid, ErrStillRunning)
	}
	return KillForced, nil
}

// processGone reports whether pid no longer exists. Errors count as "not gone"
// so that the caller reports the original failure.
func processGone(ctl ProcessControl, pid int32) bool {
	alive, err := ctl.Alive(pid)
	return err == nil && !alive
}

// waitForExit polls until pid exits or timeout elapses. It returns true if the
// process exited.
func waitForExit(ctl ProcessControl, pid int32, timeout, poll time.Duration) (bool, error) {
	if poll <= 0 {
		poll = time.Millisecond
	}
	deadline := time.Now().Add(timeout)
	for {
		alive, err := ctl.Alive(pid)
		if err != nil {
			return false, fmt.Errorf("check PID %d: %w", pid, err)
		}
		if !alive {
			return true, nil
		}
		if !time.Now().Before(deadline) {
			return false, nil
		}
		time.Sleep(poll)
	}
}

func joinPIDs(pids []int32) string {
	parts := make([]string, 0, len(pids))
	for _, p := range pids {
		parts = append(parts, fmt.Sprint(p))
	}
	return strings.Join(parts, ", ")
}
