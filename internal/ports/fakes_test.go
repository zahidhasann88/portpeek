package ports

import (
	"context"
	"errors"
)

type fakeSockets struct {
	socks []Socket
	err   error
}

func (f fakeSockets) Sockets(context.Context) ([]Socket, error) { return f.socks, f.err }

type fakeProcesses struct {
	procs map[int32]ProcessInfo
	errs  map[int32]error
	calls map[int32]int
}

func newFakeProcesses() *fakeProcesses {
	return &fakeProcesses{
		procs: map[int32]ProcessInfo{},
		errs:  map[int32]error{},
		calls: map[int32]int{},
	}
}

func (f *fakeProcesses) Process(_ context.Context, pid int32) (ProcessInfo, error) {
	f.calls[pid]++
	if err := f.errs[pid]; err != nil {
		return f.procs[pid], err
	}
	info, ok := f.procs[pid]
	if !ok {
		return ProcessInfo{}, errors.New("no such process")
	}
	return info, nil
}

type fakeContainers struct {
	bindings []Binding
	err      error
}

func (f fakeContainers) Bindings(context.Context) ([]Binding, error) {
	return f.bindings, f.err
}
