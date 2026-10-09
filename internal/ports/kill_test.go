package ports

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fakeControl simulates a process table. Signals take effect immediately
// unless ignoreTerm is set (the process survives SIGTERM).
type fakeControl struct {
	alive      map[int32]bool
	ignoreTerm bool
	termErr    error
	killErr    error
	terminated []int32
	killed     []int32
}

func newFakeControl(pids ...int32) *fakeControl {
	f := &fakeControl{alive: map[int32]bool{}}
	for _, p := range pids {
		f.alive[p] = true
	}
	return f
}

func (f *fakeControl) Terminate(pid int32) error {
	f.terminated = append(f.terminated, pid)
	if f.termErr != nil {
		return f.termErr
	}
	if !f.ignoreTerm {
		delete(f.alive, pid)
	}
	return nil
}

func (f *fakeControl) Kill(pid int32) error {
	f.killed = append(f.killed, pid)
	if f.killErr != nil {
		return f.killErr
	}
	delete(f.alive, pid)
	return nil
}

func (f *fakeControl) Alive(pid int32) (bool, error) { return f.alive[pid], nil }

var fastTiming = KillTiming{Grace: 30 * time.Millisecond, ForceGrace: 30 * time.Millisecond, Poll: time.Millisecond}

func listener(proto string, port uint16, pid int32, name string) Entry {
	return Entry{Port: port, Protocol: proto, Family: FamilyIPv4, Address: "0.0.0.0", State: StateListen, PID: pid, ProcessName: name}
}

func TestSelectTargetRules(t *testing.T) {
	web := &Container{ID: "7f3a9c2e1b4d", Name: "web", Image: "nginx"}
	cases := []struct {
		name    string
		entries []Entry
		port    uint16 // defaults to the first entry's port
		opts    Options
		self    int32
		wantErr error
		wantPID int32
	}{
		{
			name:    "port not in use",
			entries: []Entry{listener(ProtocolTCP, 80, 500, "nginx")},
			port:    81,
			wantErr: ErrNotInUse,
		},
		{
			name:    "established connection is not a listener",
			entries: []Entry{{Port: 22, Protocol: ProtocolTCP, State: "ESTABLISHED", PID: 500}},
			wantErr: ErrNotInUse,
		},
		{
			name:    "owner not visible",
			entries: []Entry{listener(ProtocolTCP, 3000, 0, "")},
			wantErr: ErrOwnerUnknown,
		},
		{
			name: "two processes on the same port",
			entries: []Entry{
				listener(ProtocolTCP, 3000, 500, "node"),
				listener(ProtocolTCP, 3000, 501, "python"),
			},
			wantErr: ErrAmbiguousOwner,
		},
		{
			name: "same process on tcp and udp is fine",
			entries: []Entry{
				listener(ProtocolTCP, 3000, 500, "node"),
				listener(ProtocolUDP, 3000, 500, "node"),
			},
			wantPID: 500,
		},
		{
			name: "udp filter removes the ambiguity",
			entries: []Entry{
				listener(ProtocolTCP, 3000, 500, "node"),
				listener(ProtocolUDP, 3000, 501, "python"),
			},
			opts:    Options{UDP: true},
			wantPID: 501,
		},
		{
			name:    "pid 1 is protected",
			entries: []Entry{listener(ProtocolTCP, 80, 1, "init")},
			wantErr: ErrProtectedProcess,
		},
		{
			name:    "pid 0 is protected",
			entries: []Entry{listener(ProtocolTCP, 80, 0, "")},
			wantErr: ErrOwnerUnknown,
		},
		{
			name:    "portpeek itself is protected",
			entries: []Entry{listener(ProtocolTCP, 8000, 999, "portpeek")},
			self:    999,
			wantErr: ErrProtectedProcess,
		},
		{
			name: "container-owned port is refused",
			entries: []Entry{
				{Port: 8080, Protocol: ProtocolTCP, State: StateListen, Address: "0.0.0.0", PID: 300, ProcessName: "docker-proxy", Container: web},
			},
			wantErr: ErrContainerOwned,
		},
		{
			name:    "docker-proxy is refused even without a container match",
			entries: []Entry{listener(ProtocolTCP, 8080, 300, "docker-proxy")},
			wantErr: ErrContainerOwned,
		},
		{
			name:    "ordinary process is allowed",
			entries: []Entry{listener(ProtocolTCP, 3000, 4311, "node")},
			wantPID: 4311,
		},
		{
			name:    "other protocol filter excludes the only listener",
			entries: []Entry{listener(ProtocolTCP, 3000, 4311, "node")},
			opts:    Options{UDP: true},
			wantErr: ErrNotInUse,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			port := tc.port
			if port == 0 {
				port = tc.entries[0].Port
			}
			got, err := SelectTarget(tc.entries, port, tc.opts, tc.self)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.PID != tc.wantPID {
				t.Errorf("target PID = %d, want %d", got.PID, tc.wantPID)
			}
		})
	}
}

func TestCheckKillableMessages(t *testing.T) {
	err := CheckKillable(Entry{Port: 8080, PID: 300, Container: &Container{Name: "web"}}, 1)
	if err == nil || !strings.Contains(err.Error(), "docker stop web") {
		t.Errorf("container refusal should suggest docker stop, got %v", err)
	}
	if runtime.GOOS == "windows" {
		if err := CheckKillable(Entry{PID: 4}, 1); !errors.Is(err, ErrProtectedProcess) {
			t.Errorf("PID 4 should be protected on Windows, got %v", err)
		}
	}
}

func TestStopExitsOnSIGTERM(t *testing.T) {
	ctl := newFakeControl(42)
	got, err := Stop(ctl, 42, false, fastTiming)
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if got != KillExitedOnTerm {
		t.Errorf("result = %v, want KillExitedOnTerm", got)
	}
	if len(ctl.killed) != 0 {
		t.Errorf("SIGKILL sent %d times, want 0", len(ctl.killed))
	}
}

func TestStopWithoutForceNeverSendsSIGKILL(t *testing.T) {
	ctl := newFakeControl(42)
	ctl.ignoreTerm = true
	_, err := Stop(ctl, 42, false, fastTiming)
	if !errors.Is(err, ErrStillRunning) {
		t.Fatalf("err = %v, want ErrStillRunning", err)
	}
	if len(ctl.killed) != 0 {
		t.Errorf("SIGKILL must not be sent without --force")
	}
}

func TestStopWithForceEscalatesToSIGKILL(t *testing.T) {
	ctl := newFakeControl(42)
	ctl.ignoreTerm = true
	got, err := Stop(ctl, 42, true, fastTiming)
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if got != KillForced {
		t.Errorf("result = %v, want KillForced", got)
	}
	if len(ctl.terminated) != 1 || len(ctl.killed) != 1 {
		t.Errorf("signals: TERM %d, KILL %d; want 1 each", len(ctl.terminated), len(ctl.killed))
	}
}

func TestStopAlreadyGone(t *testing.T) {
	ctl := newFakeControl()
	ctl.termErr = errors.New("no such process")
	got, err := Stop(ctl, 42, false, fastTiming)
	if err != nil || got != KillAlreadyGone {
		t.Errorf("Stop() = %v, %v; want KillAlreadyGone, nil", got, err)
	}
}

func TestStopPermissionDeniedIsAccessDenied(t *testing.T) {
	ctl := newFakeControl(42)
	ctl.termErr = fmt.Errorf("signal process 42: %w", ErrAccessDenied)
	_, err := Stop(ctl, 42, false, fastTiming)
	if !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("err = %v, want ErrAccessDenied", err)
	}
}

func TestStopSIGKILLFailureIsReported(t *testing.T) {
	ctl := newFakeControl(42)
	ctl.ignoreTerm = true
	ctl.killErr = errors.New("operation not permitted")
	if _, err := Stop(ctl, 42, true, fastTiming); err == nil {
		t.Fatal("expected error when SIGKILL fails")
	}
}

func TestDefaultKillTimingIsSane(t *testing.T) {
	d := DefaultKillTiming()
	if d.Grace <= 0 || d.ForceGrace <= 0 || d.Poll <= 0 || d.Poll > d.Grace {
		t.Errorf("DefaultKillTiming() = %+v", d)
	}
}

func TestSelectTargetChecksEverySocket(t *testing.T) {
	entries := []Entry{
		listener(ProtocolTCP, 8080, 300, "worker"),
		{Port: 8080, Protocol: ProtocolTCP, State: StateListen, PID: 300, Container: &Container{Name: "web"}},
	}
	for _, candidates := range [][]Entry{entries, {entries[1], entries[0]}} {
		if _, err := SelectTarget(candidates, 8080, Options{}, 999); !errors.Is(err, ErrContainerOwned) {
			t.Fatalf("expected container refusal for either socket order, got %v", err)
		}
	}
}
