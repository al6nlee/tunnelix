package updates

import (
	"context"
	"errors"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"sync/atomic"
	"testing"
	"time"
)

type fakeEngine struct {
	checks, downloads, restarts atomic.Int32
	block                       bool
	cancelled                   chan struct{}
}

func (f *fakeEngine) Check(ctx context.Context) (*updater.Release, error) {
	f.checks.Add(1)
	if f.block {
		<-ctx.Done()
		close(f.cancelled)
		return nil, ctx.Err()
	}
	return &updater.Release{Version: "2.0.0"}, nil
}
func (f *fakeEngine) DownloadAndInstall(context.Context) error { f.downloads.Add(1); return nil }
func (f *fakeEngine) Restart(context.Context) error {
	f.restarts.Add(1)
	return errors.New("simulated restart failure")
}
func settled(t *testing.T, c *Controller, state string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		done := !c.busy && c.status.State == state
		c.mu.Unlock()
		if done {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("want %s, got %+v", state, c.Snapshot())
}
func TestManualCheckRequiresDownload(t *testing.T) {
	f := &fakeEngine{}
	c := NewController(f, true, nil)
	defer c.Close()
	if e := c.Download(); e == nil {
		t.Fatal("download without release")
	}
	if e := c.Check(); e != nil {
		t.Fatal(e)
	}
	settled(t, c, "available")
	if f.downloads.Load() != 0 {
		t.Fatal("manual check downloaded")
	}
	if e := c.Download(); e != nil {
		t.Fatal(e)
	}
	settled(t, c, "ready")
	c.Check()
	if f.checks.Load() != 1 {
		t.Fatal("ready was overwritten")
	}
	if e := c.Restart(); e == nil || c.Snapshot().State != "ready" {
		t.Fatalf("restart failure %v", e)
	}
}
func TestAutomaticAndOptOut(t *testing.T) {
	f := &fakeEngine{}
	c := NewController(f, true, nil)
	defer c.Close()
	go c.Run()
	if e := c.start(true, false); e != nil {
		t.Fatal(e)
	}
	if f.checks.Load() != 0 {
		t.Fatal("disabled check")
	}
	c.SetAutomatic(true)
	settled(t, c, "ready")
	if f.downloads.Load() != 1 || f.restarts.Load() != 0 {
		t.Fatal("automatic must stage without restarting")
	}
}
func TestOptOutCancelsInFlightAndSingleFlight(t *testing.T) {
	f := &fakeEngine{block: true, cancelled: make(chan struct{})}
	c := NewController(f, true, nil)
	defer c.Close()
	go c.Run()
	c.SetAutomatic(true)
	deadline := time.Now().Add(3 * time.Second)
	for f.checks.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if e := c.Check(); e == nil {
		t.Fatal("overlapping check")
	}
	c.SetAutomatic(false)
	select {
	case <-f.cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("opt out did not cancel")
	}
	settled(t, c, "idle")
	if f.downloads.Load() != 0 {
		t.Fatal("download after opt out")
	}
}
func TestDevelopmentDoesNotAutoDownload(t *testing.T) {
	f := &fakeEngine{}
	c := NewController(f, false, nil)
	defer c.Close()
	go c.Run()
	c.SetAutomatic(true)
	settled(t, c, "available")
	if f.downloads.Load() != 0 {
		t.Fatal("development downloaded")
	}
}
