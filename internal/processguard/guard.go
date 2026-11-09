// Package processguard owns only the child process tree it launches. It never
// scans system processes, persists PIDs, or kills a process by a command pattern.
package processguard

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

const Argument = "--tunnelix-supervise"

// Process holds the write side of a parent-liveness pipe. A forced parent exit
// closes it in the kernel, so the supervisor can clean up without application code.
type Process struct {
	Cmd       *exec.Cmd
	alive     *os.File
	childRead *os.File
	cleanup   func()
	once      sync.Once
}

func New(helper, executable string, args, env []string) (*Process, error) {
	r, w, e := os.Pipe()
	if e != nil {
		return nil, e
	}
	cmd := exec.Command(helper, append([]string{Argument, executable}, args...)...)
	cmd.Env = env
	cmd.Stdin = r
	if e = prepare(cmd); e != nil {
		r.Close()
		w.Close()
		return nil, e
	}
	return &Process{Cmd: cmd, alive: w, childRead: r}, nil
}
func (p *Process) Start() error {
	if e := p.Cmd.Start(); e != nil {
		p.childRead.Close()
		p.alive.Close()
		return e
	}
	p.childRead.Close()
	cleanup, e := started(p.Cmd)
	if e != nil {
		p.alive.Close()
		terminate(p.Cmd)
		p.Cmd.Wait()
		return e
	}
	p.cleanup = cleanup
	// In particular, the Windows job owns the supervisor BEFORE it can spawn SSH.
	if _, e = p.alive.Write([]byte{1}); e != nil {
		p.Stop()
		p.Cmd.Wait()
		cleanup()
		return e
	}
	return nil
}
func (p *Process) Close() { p.Stop(); _ = p.childRead.Close() }
func (p *Process) Stop()  { p.once.Do(func() { _ = p.alive.Close() }) }
func (p *Process) Wait() error {
	e := p.Cmd.Wait()
	p.Stop()
	if p.cleanup != nil {
		p.cleanup()
	}
	return e
}

// Run is entered before the GUI runtime. Only this supervisor inherits the
// liveness pipe; SSH and Askpass receive null stdin, so they cannot hold it open.
func Run(args []string) int {
	if len(args) < 1 {
		return 125
	}
	var ack [1]byte
	if _, e := io.ReadFull(os.Stdin, ack[:]); e != nil || ack[0] != 1 {
		return 125
	}
	child := exec.Command(args[0], args[1:]...)
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	if e := prepare(child); e != nil {
		return fail()
	}
	if e := child.Start(); e != nil {
		return fail()
	}
	cleanup, e := started(child)
	if e != nil {
		terminate(child)
		child.Wait()
		return fail()
	}
	var once sync.Once
	originalCleanup := cleanup
	cleanup = func() { once.Do(originalCleanup) }
	defer cleanup()
	gone := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, os.Stdin); close(gone) }()
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	select {
	case e = <-done:
	case <-gone:
		cleanup()
		terminate(child)
		select {
		case e = <-done:
		case <-time.After(5 * time.Second):
			return 125
		}
	}
	if e == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(e, &exit) {
		code := exit.ExitCode()
		if code >= 0 {
			return code
		}
	}
	return 125
}
func fail() int { fmt.Fprintln(os.Stderr, "tunnelix: process supervision failed"); return 125 }
