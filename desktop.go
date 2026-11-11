package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"path/filepath"
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
	window        *application.WebviewWindow
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

func (d *Desktop) RelocateConfig() (bool, error) {
	current := d.service.Snapshot().ConfigPath
	path, e := d.app.Dialog.SaveFile().AttachToWindow(d.window).
		SetMessage("Save the current configuration to a new location. The original is kept. / 将当前配置保存到新位置，保留原文件。").
		SetDirectory(filepath.Dir(current)).SetFilename(filepath.Base(current)).
		AddFilter("JSON configuration", "*.json").CanCreateDirectories(true).PromptForSingleSelection()
	if e != nil || path == "" {
		return false, e
	}
	return true, d.service.ChangeConfigPath(path, true)
}

func (d *Desktop) SwitchConfig() (bool, error) {
	path, e := d.app.Dialog.OpenFile().AttachToWindow(d.window).
		SetMessage("Use an existing configuration. Connections will not start automatically. / 使用已有配置，不会立即自动连接。").
		SetDirectory(filepath.Dir(d.service.Snapshot().ConfigPath)).
		AddFilter("JSON configuration", "*.json").PromptForSingleSelection()
	if e != nil || path == "" {
		return false, e
	}
	return true, d.service.ChangeConfigPath(path, false)
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
