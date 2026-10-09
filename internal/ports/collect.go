package ports

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

// Options controls which sockets are collected.
type Options struct {
	// TCP and UDP select protocols. When both are false, both are included.
	TCP bool
	UDP bool
	// All includes non-listening sockets (for example ESTABLISHED TCP).
	All bool
	// Port, when non-zero, restricts results to sockets on that local port.
	Port uint16
}

// includesProtocol reports whether a socket of the given protocol passes the
// protocol filter.
func (o Options) includesProtocol(proto string) bool {
	if !o.TCP && !o.UDP {
		return true
	}
	return (o.TCP && proto == ProtocolTCP) || (o.UDP && proto == ProtocolUDP)
}

// Result is the output of a collection run.
type Result struct {
	Entries []Entry
	// Restricted counts entries whose owner or process details could not be
	// read, typically because they belong to another user.
	Restricted int
	// Warnings are non-fatal problems to show to the user on stderr.
	Warnings []string
}

// Collector combines socket, process, and container data into Entries.
type Collector struct {
	Sockets    PortSource
	Processes  ProcessSource
	Containers ContainerSource // optional; nil disables container lookup
}

// Collect gathers sockets and enriches them. Errors from the socket source are
// fatal. Process and container problems are handled without failing the run.
func (c Collector) Collect(ctx context.Context, opts Options) (Result, error) {
	if c.Sockets == nil {
		return Result{}, errors.New("collect ports: no socket source configured")
	}
	raw, err := c.Sockets.Sockets(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("collect ports: list sockets: %w", err)
	}

	var res Result
	res.Entries = make([]Entry, 0, len(raw))
	procs := make(map[int32]procLookup)

	for _, s := range normalizeSockets(raw, opts) {
		e := entryFromSocket(s)
		if s.PID <= 0 {
			res.Restricted++
		} else {
			lk, ok := procs[s.PID]
			if !ok {
				lk = c.lookupProcess(ctx, s.PID)
				procs[s.PID] = lk
			}
			applyProcess(&e, lk)
			if lk.denied {
				res.Restricted++
			}
		}
		res.Entries = append(res.Entries, e)
	}

	if c.Containers != nil {
		bindings, err := c.Containers.Bindings(ctx)
		switch {
		case err == nil:
			for i := range res.Entries {
				attributeContainer(&res.Entries[i], bindings)
			}
		case errors.Is(err, ErrUnavailable):
			// No container runtime: container information is silently skipped.
		default:
			res.Warnings = append(res.Warnings,
				fmt.Sprintf("container lookup failed, container details skipped: %v", err))
		}
	}

	sortEntries(res.Entries)
	return res, nil
}

// procLookup is a cached per-PID process inspection.
type procLookup struct {
	info   ProcessInfo
	known  bool // false when the process could not be inspected at all
	denied bool // true when some details were hidden by the OS
}

func (c Collector) lookupProcess(ctx context.Context, pid int32) procLookup {
	if c.Processes == nil {
		return procLookup{}
	}
	info, err := c.Processes.Process(ctx, pid)
	switch {
	case err == nil:
		return procLookup{info: info, known: true}
	case errors.Is(err, ErrAccessDenied):
		return procLookup{info: info, known: true, denied: true}
	default:
		// The process most likely exited between listing and inspection.
		return procLookup{}
	}
}

// normalizeSockets removes sockets that cannot be shown (no port, such as
// Unix domain sockets), applies the protocol, listening, and port filters,
// and drops exact duplicates reported more than once by the OS.
func normalizeSockets(raw []Socket, opts Options) []Socket {
	type key struct {
		proto, addr, state, remote string
		port, remotePort           uint16
		pid                        int32
	}
	seen := make(map[key]bool, len(raw))
	out := make([]Socket, 0, len(raw))
	for _, s := range raw {
		if s.Port == 0 {
			continue
		}
		if opts.Port != 0 && s.Port != opts.Port {
			continue
		}
		if !opts.includesProtocol(s.Protocol) {
			continue
		}
		if !opts.All && !isListeningState(s.State) {
			continue
		}
		k := key{s.Protocol, s.Address, s.State, s.RemoteIP, s.Port, s.RemotePort, s.PID}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, s)
	}
	return out
}

// isListeningState reports whether a socket state means "accepting or waiting
// for traffic": a TCP LISTEN socket or an unconnected UDP socket.
func isListeningState(state string) bool {
	return state == StateListen || state == StateUnconnect
}

func entryFromSocket(s Socket) Entry {
	return Entry{
		Port:          s.Port,
		Protocol:      s.Protocol,
		Family:        Family(s.Address),
		Address:       s.Address,
		State:         s.State,
		RemoteAddress: remoteString(s.RemoteIP, s.RemotePort),
		PID:           s.PID,
	}
}

func applyProcess(e *Entry, lk procLookup) {
	if !lk.known {
		return
	}
	e.PPID = lk.info.PPID
	e.ProcessName = lk.info.Name
	e.User = lk.info.User
	e.Command = lk.info.Command
	e.Executable = lk.info.Executable
	e.WorkingDir = lk.info.WorkingDir
	e.StartTime = lk.info.StartTime
}

// attributeContainer marks a socket as belonging to a container when it is
// owned by a container-runtime helper (or its owner is hidden) and a published
// binding matches its protocol, port, and address.
func attributeContainer(e *Entry, bindings []Binding) {
	if e.Container != nil {
		return
	}
	if e.ProcessName != "" && !IsDockerProxy(e.ProcessName) {
		return
	}
	for _, b := range bindings {
		if b.Protocol == e.Protocol && b.HostPort == e.Port && addressMatches(b.HostIP, e.Address) {
			c := b.Container
			e.Container = &c
			return
		}
	}
}

// addressMatches reports whether a binding on bindIP covers a socket on addr.
// An empty or wildcard binding address covers every local address.
func addressMatches(bindIP, addr string) bool {
	switch bindIP {
	case "", "0.0.0.0", "::":
		return true
	}
	return bindIP == addr
}

// sortEntries orders entries by port, then protocol (tcp first), address,
// and PID, so output is deterministic.
func sortEntries(entries []Entry) {
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.Port != b.Port {
			return a.Port < b.Port
		}
		if a.Protocol != b.Protocol {
			return a.Protocol == ProtocolTCP
		}
		if a.Address != b.Address {
			return a.Address < b.Address
		}
		if a.State != b.State {
			return a.State < b.State
		}
		return a.PID < b.PID
	})
}
