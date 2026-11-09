package updates

import (
	"context"
	"errors"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"regexp"
	"sync"
	"time"
	"tunnelix/internal/buildinfo"
)

type Engine interface {
	Check(context.Context) (*updater.Release, error)
	DownloadAndInstall(context.Context) error
	Restart(context.Context) error
}
type Status struct {
	Current        string `json:"current"`
	Latest         string `json:"latest"`
	Notes          string `json:"notes"`
	State          string `json:"state"`
	Code           string `json:"code"`
	Progress       int    `json:"progress"`
	CheckedAt      string `json:"checkedAt"`
	InstallEnabled bool   `json:"installEnabled"`
}
type Controller struct {
	mu                                  sync.Mutex
	engine                              Engine
	status                              Status
	automatic, background, busy, closed bool
	cancel                              context.CancelFunc
	root                                context.Context
	stop                                context.CancelFunc
	wake                                chan struct{}
	changed                             func()
}

func NewController(engine Engine, installEnabled bool, changed func()) *Controller {
	root, stop := context.WithCancel(context.Background())
	return &Controller{engine: engine, status: Status{Current: buildinfo.Version, State: "idle", InstallEnabled: installEnabled}, root: root, stop: stop, wake: make(chan struct{}, 1), changed: changed}
}
func (c *Controller) Snapshot() Status { c.mu.Lock(); defer c.mu.Unlock(); return c.status }
func (c *Controller) notify() {
	if c.changed != nil {
		c.changed()
	}
}
func (c *Controller) SetAutomatic(enabled bool) {
	c.mu.Lock()
	different := c.automatic != enabled
	c.automatic = enabled
	if !enabled && c.background && c.cancel != nil {
		c.cancel()
	}
	c.mu.Unlock()
	if enabled && different {
		select {
		case c.wake <- struct{}{}:
		default:
		}
	}
}
func (c *Controller) Run() {
	timer := time.NewTicker(6 * time.Hour)
	defer timer.Stop()
	for {
		select {
		case <-c.root.Done():
			return
		case <-timer.C:
		case <-c.wake:
		}
		c.mu.Lock()
		enabled := c.automatic
		c.mu.Unlock()
		if enabled {
			_ = c.start(true, false)
		}
	}
}
func (c *Controller) Close()          { c.mu.Lock(); c.closed = true; c.mu.Unlock(); c.stop() }
func (c *Controller) Check() error    { return c.start(false, false) }
func (c *Controller) Download() error { return c.start(false, true) }
func (c *Controller) start(background, downloadOnly bool) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("APP_CLOSED")
	}
	if c.busy || c.status.State == "restarting" {
		c.mu.Unlock()
		return errors.New("UPDATE_BUSY")
	}
	if c.status.State == "ready" {
		c.mu.Unlock()
		return nil
	}
	if background && !c.automatic {
		c.mu.Unlock()
		return nil
	}
	if downloadOnly && (c.status.State != "available" || c.status.Latest == "" || !c.status.InstallEnabled) {
		c.mu.Unlock()
		return errors.New("UPDATE_UNAVAILABLE_BUILD")
	}
	ctx, cancel := context.WithCancel(c.root)
	c.cancel = cancel
	c.background = background
	c.busy = true
	c.status.Code = ""
	c.status.Progress = 0
	c.status.State = "checking"
	if downloadOnly {
		c.status.State = "downloading"
	} else {
		c.status.Latest = ""
		c.status.Notes = ""
	}
	c.mu.Unlock()
	c.notify()
	go func() {
		defer cancel()
		defer func() {
			c.mu.Lock()
			c.busy = false
			c.cancel = nil
			c.background = false
			c.mu.Unlock()
			c.notify()
		}()
		if !downloadOnly {
			checkCtx, stop := context.WithTimeout(ctx, 30*time.Second)
			release, e := c.engine.Check(checkCtx)
			stop()
			c.mu.Lock()
			c.status.CheckedAt = time.Now().UTC().Format(time.RFC3339)
			c.mu.Unlock()
			if e != nil {
				c.fail(ctx, e, "UPDATE_CHECK_FAILED")
				return
			}
			c.mu.Lock()
			if release == nil {
				c.status.State = "up-to-date"
				c.status.Latest = ""
				c.status.Notes = ""
				c.mu.Unlock()
				return
			}
			c.status.Latest = release.Version
			c.status.Notes = release.Notes
			c.status.State = "available"
			automatic := background && c.automatic && c.status.InstallEnabled
			c.mu.Unlock()
			c.notify()
			if !automatic {
				return
			}
		}
		if ctx.Err() != nil {
			c.fail(ctx, ctx.Err(), "UPDATE_DOWNLOAD_FAILED")
			return
		}
		c.mu.Lock()
		c.status.State = "downloading"
		c.mu.Unlock()
		c.notify()
		downloadCtx, stop := context.WithTimeout(ctx, 20*time.Minute)
		e := c.engine.DownloadAndInstall(downloadCtx)
		stop()
		if e != nil {
			c.fail(ctx, e, "UPDATE_DOWNLOAD_FAILED")
			return
		}
		c.mu.Lock()
		c.status.State = "ready"
		c.status.Progress = 100
		c.mu.Unlock()
	}()
	return nil
}

var codePattern = regexp.MustCompile(`UPDATE_[A-Z_]+`)

func (c *Controller) fail(ctx context.Context, e error, fallback string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx.Err() != nil {
		c.status.State = "idle"
		if c.status.Latest != "" {
			c.status.State = "available"
		}
		c.status.Code = ""
		return
	}
	c.status.State = "error"
	c.status.Code = codePattern.FindString(e.Error())
	if c.status.Code == "" {
		c.status.Code = fallback
	}
	if c.status.Code == "UPDATE_NO_RELEASE" {
		c.status.State = "unpublished"
	}
	if c.status.Code == "UPDATE_UNAVAILABLE_BUILD" {
		c.status.State = "unsupported"
	}
}
func (c *Controller) Progress(written, total int64) {
	if total <= 0 {
		return
	}
	c.mu.Lock()
	percent := int(written * 100 / total)
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	changed := percent != c.status.Progress
	c.status.Progress = percent
	c.mu.Unlock()
	if changed {
		c.notify()
	}
}
func (c *Controller) Restart() error {
	c.mu.Lock()
	if c.busy || c.status.State != "ready" {
		c.mu.Unlock()
		return errors.New("UPDATE_NOT_READY")
	}
	c.busy = true
	c.status.State = "restarting"
	c.mu.Unlock()
	c.notify()
	e := c.engine.Restart(c.root)
	if e != nil {
		c.mu.Lock()
		c.busy = false
		c.status.State = "ready"
		c.status.Code = "UPDATE_RESTART_FAILED"
		c.mu.Unlock()
		c.notify()
		return errors.New("UPDATE_RESTART_FAILED")
	}
	return nil
}
