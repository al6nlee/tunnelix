package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"tunnelix/internal/config"
	"tunnelix/internal/service"
	"tunnelix/internal/tunnel"
	"tunnelix/internal/updates"
)

type Desktop struct {
	manager       *tunnel.Manager
	updates       *updates.Controller
	frontendReady func()
	app           *application.App
	service       *service.Service
}

func (d *Desktop) ChooseFile() (string, error) {
	return d.app.Dialog.OpenFile().ShowHiddenFiles(true).PromptForSingleSelection()
}
func (d *Desktop) ExportFile() error {
	path, e := d.app.Dialog.SaveFile().SetFilename("tunnelix-config.json").AddFilter("JSON", "*.json").PromptForSingleSelection()
	if e != nil || path == "" {
		return e
	}
	return config.NewStore(path).Save(d.service.Snapshot().Config)
}

// FrontendReady acknowledges a successful React render and Go binding round trip.
func (d *Desktop) FrontendReady() {
	if d.frontendReady != nil {
		d.frontendReady()
	}
}

func (d *Desktop) UpdateStatus() updates.Status { return d.updates.Snapshot() }
func (d *Desktop) CheckForUpdates() error       { return d.updates.Check() }
func (d *Desktop) DownloadUpdate() error        { return d.updates.Download() }
func (d *Desktop) RestartUpdate() error {
	if e := d.manager.ReserveRestart(); e != nil {
		return e
	}
	if e := d.updates.Restart(); e != nil {
		d.manager.ReleaseRestart()
		return e
	}
	return nil
}
func (d *Desktop) ThirdPartyLicenses() string { return thirdPartyLicenses }
