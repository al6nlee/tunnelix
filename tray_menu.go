package main

import (
	"fmt"
	"github.com/wailsapp/wails/v3/pkg/application"
	"tunnelix/internal/buildinfo"
	"tunnelix/internal/service"
	traymodel "tunnelix/internal/tray"
	"tunnelix/internal/updates"
)

func renderTray(menu *application.Menu, snapshot service.Snapshot, svc *service.Service, zh bool, update updates.Status, show func(), settings func(bool)) {
	label := func(en, cn string) string {
		if zh {
			return cn
		}
		return en
	}
	model := traymodel.Build(snapshot.Config, snapshot.Statuses, zh)
	menu.Clear()
	menu.Add("Tunnelix · v" + buildinfo.Version).SetEnabled(false)
	menu.Add(model.Summary).SetEnabled(false)
	menu.Add(label("Open main window", "打开主窗口")).OnClick(func(*application.Context) { show() })
	menu.AddSeparator()
	if model.Total == 0 {
		menu.Add(label("No tunnels yet — create one in the main window", "暂无隧道，请在主窗口新建")).SetEnabled(false)
	}
	for _, entry := range model.Entries {
		item := entry
		connection := menu.AddSubmenu(item.Name + " · " + item.State)
		connection.Add(item.Action).OnClick(func(*application.Context) {
			go func() {
				if item.Active {
					svc.Disconnect(item.ID)
				} else if svc.Connect(item.ID) != nil {
					application.InvokeAsync(show)
				}
			}()
		})
		connection.Add(label("View details…", "查看详情…")).OnClick(func(*application.Context) { show(); application.Get().Event.Emit("tunnelix:detail", item.ID) })
	}
	if model.ViewAll != "" {
		menu.Add(model.ViewAll).OnClick(func(*application.Context) {
			show()
			application.Get().Event.Emit("tunnelix:show-all")
		})
	}
	if model.Total > 0 {
		menu.AddSeparator()
		menu.Add(label("Connect all", "连接全部")).SetEnabled(model.Active < model.Total).OnClick(func(*application.Context) {
			go func() {
				if svc.ConnectAll() != nil {
					application.InvokeAsync(show)
				}
			}()
		})
		menu.Add(label("Disconnect all", "断开全部")).SetEnabled(model.Active > 0).OnClick(func(*application.Context) { go svc.DisconnectAll() })
	}
	menu.AddSeparator()
	menu.Add(label("Settings…", "设置…")).OnClick(func(*application.Context) { settings(false) })
	updateLabel := label("Check for updates…", "检查更新…")
	if update.State == "ready" {
		updateLabel = label("Update ready…", "更新已就绪…")
	} else if update.State == "downloading" {
		updateLabel = fmt.Sprintf(label("Downloading update · %d%%", "正在下载更新 · %d%%"), update.Progress)
	}
	menu.Add(updateLabel).OnClick(func(*application.Context) { settings(update.State != "ready" && update.State != "downloading") })
	menu.AddSeparator()
	menu.Add(label("Quit Tunnelix", "退出 Tunnelix")).OnClick(func(*application.Context) { application.Get().Quit() })
	menu.Update()
}
