package main

import (
	"crypto/sha256"
	"embed"
	"errors"
	"flag"
	"fmt"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"tunnelix/internal/askpass"
	"tunnelix/internal/buildinfo"
	"tunnelix/internal/config"
	"tunnelix/internal/locale"
	"tunnelix/internal/mactray"
	"tunnelix/internal/processguard"
	"tunnelix/internal/service"
	"tunnelix/internal/tunnel"
	"tunnelix/internal/updates"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed assets/brand/tray.png
var trayIcon []byte

//go:embed assets/brand/icon.png
var appIcon []byte

//go:embed THIRD_PARTY_LICENSES.txt
var thirdPartyLicenses string

func main() {
	updater.HandleHelperMode()
	if len(os.Args) > 1 && os.Args[1] == processguard.Argument {
		os.Exit(processguard.Run(os.Args[2:]))
	}
	if os.Getenv("TUNNELIX_ASKPASS") == "1" {
		os.Exit(askpass.RunHelper())
	}
	flags := flag.NewFlagSet("Tunnelix", flag.ExitOnError)
	versionFlag := flags.Bool("version", false, "Print version and exit")
	smokeFlag := flags.Bool("smoke-test", false, "Verify frontend startup with temporary configuration")
	configFlag := flags.String("config", "", "Use a separate configuration file")
	_ = flags.Parse(os.Args[1:])
	if *versionFlag {
		fmt.Printf("Tunnelix %s (%s)\n", buildinfo.Version, buildinfo.Commit)
		return
	}
	if err := mactray.Enable(); err != nil {
		log.Fatal(err)
	}
	smoke := *smokeFlag
	path, e := config.DefaultPath()
	explicitConfig := *configFlag != ""
	if !explicitConfig && !smoke {
		if remembered, rememberErr := config.RememberedPath(); rememberErr != nil {
			log.Fatal(rememberErr)
		} else if remembered != "" {
			path = remembered
		}
	}
	if *configFlag != "" {
		path, e = filepath.Abs(*configFlag)
	}
	var smokeDir string
	if smoke {
		smokeDir, e = os.MkdirTemp("", "tunnelix-smoke-")
		if e == nil {
			path = smokeDir + string(os.PathSeparator) + "config.json"
		}
	}
	if e != nil {
		log.Fatal(e)
	}
	needsConfigSelection := false
	if !smoke && !explicitConfig {
		_, statErr := os.Stat(path)
		switch {
		case statErr == nil:
		case errors.Is(statErr, os.ErrNotExist):
			needsConfigSelection = true
		default:
			log.Fatal(statErr)
		}
	}
	helper, e := os.Executable()
	if e != nil {
		log.Fatal(e)
	}
	manager := tunnel.New(helper)
	svc, e := service.New(config.NewStore(path), manager)
	if e != nil {
		log.Fatalf("Cannot load Tunnelix configuration: %v", e)
	}
	var window *application.WebviewWindow
	var stopping atomic.Bool
	desktop := &Desktop{service: svc, manager: manager}
	app := application.New(application.Options{
		Name: "Tunnelix", Description: "SSH tunnel manager", Icon: appIcon, LogLevel: slog.LevelWarn,
		Services: []application.Service{application.NewService(svc), application.NewService(desktop)},
		Assets:   application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		OnShutdown: func() {
			stopping.Store(true)
			if desktop.updates != nil {
				desktop.updates.Close()
			}
			manager.Close()
			if smokeDir != "" {
				os.RemoveAll(smokeDir)
			}
		},
		SingleInstance: &application.SingleInstanceOptions{UniqueID: func() string {
			if smoke {
				return "app.tunnelix.smoke"
			}
			if *configFlag != "" {
				return fmt.Sprintf("app.tunnelix.desktop.%x", sha256.Sum256([]byte(path)))
			}
			return "app.tunnelix.desktop"
		}(), OnSecondInstanceLaunch: func(application.SecondInstanceData) {
			if window != nil {
				window.Show()
				window.Focus()
			}
		}},
	})
	desktop.app = app
	provider, err := updates.NewProvider()
	if err != nil {
		log.Fatal(err)
	}
	if err = app.Updater.Init(updater.Config{CurrentVersion: buildinfo.Version, Providers: []updater.Provider{provider}, PublicKey: provider.PublicKey, Window: updater.WindowNone}); err != nil {
		log.Fatal(err)
	}
	automaticAllowed := !smoke && *configFlag == ""
	desktop.updates = updates.NewController(app.Updater, automaticAllowed && len(provider.PublicKey) > 0 && updates.SupportsInstallation(helper, runtime.GOOS), func() {
		if !stopping.Load() {
			app.Event.Emit("tunnelix:update")
		}
	})
	provider.OnProgress = desktop.updates.Progress
	go desktop.updates.Run()
	if smoke {
		timeout := time.AfterFunc(30*time.Second, func() {
			os.RemoveAll(smokeDir)
			fmt.Fprintln(os.Stderr, "Tunnelix frontend smoke timed out")
			os.Exit(1)
		})
		var once sync.Once
		desktop.frontendReady = func() {
			once.Do(func() { timeout.Stop(); fmt.Println("Tunnelix frontend smoke passed"); go app.Quit() })
		}
	}

	window = app.Window.NewWithOptions(application.WebviewWindowOptions{Title: "Tunnelix", Width: 1060, Height: 720, MinWidth: 900, MinHeight: 600, URL: "/", BackgroundColour: application.NewRGB(246, 248, 252)})
	if needsConfigSelection {
		window.Hide()
	}
	tray := app.SystemTray.New()
	if runtime.GOOS == "darwin" {
		tray.SetTemplateIcon(trayIcon)
	} else {
		tray.SetIcon(appIcon)
	}
	tray.SetTooltip("Tunnelix")
	show := func() { window.Show(); window.Focus() }
	showSettings := func(check bool) {
		show()
		app.Event.Emit("tunnelix:settings")
		if check {
			_ = desktop.updates.Check()
		}
	}

	nativeMenu := app.Menu.New()
	appMenu := nativeMenu.AddSubmenu("Tunnelix")
	editMenuItem := application.NewSubMenuItem("Edit")
	nativeMenu.Append(application.NewMenuFromItems(editMenuItem))
	editMenu := editMenuItem.GetSubmenu()
	type translatedItem struct {
		item   *application.MenuItem
		en, zh string
	}
	labels := []translatedItem{{editMenuItem, "Edit", "编辑"}}
	addRole := func(menu *application.Menu, role application.Role, en, zh string) {
		item := application.NewRole(role)
		if item != nil {
			menu.Append(application.NewMenuFromItems(item))
			labels = append(labels, translatedItem{item, en, zh})
		}
	}
	openItem := appMenu.Add("Open Tunnelix").OnClick(func(*application.Context) { show() })
	labels = append(labels, translatedItem{openItem, "Open Tunnelix", "打开 Tunnelix"})
	settingsItem := appMenu.Add("Settings…").SetAccelerator("CmdOrCtrl+,").OnClick(func(*application.Context) { showSettings(false) })
	labels = append(labels, translatedItem{settingsItem, "Settings…", "设置…"})
	checkItem := appMenu.Add("Check for updates…").OnClick(func(*application.Context) { showSettings(true) })
	labels = append(labels, translatedItem{checkItem, "Check for updates…", "检查更新…"})
	addRole(appMenu, application.Hide, "Hide Tunnelix", "隐藏 Tunnelix")
	appMenu.AddSeparator()
	quitItem := appMenu.Add("Quit Tunnelix").SetAccelerator("CmdOrCtrl+Q").OnClick(func(*application.Context) { app.Quit() })
	labels = append(labels, translatedItem{quitItem, "Quit Tunnelix", "退出 Tunnelix"})
	addRole(editMenu, application.Undo, "Undo", "撤销")
	addRole(editMenu, application.Redo, "Redo", "重做")
	editMenu.AddSeparator()
	addRole(editMenu, application.Cut, "Cut", "剪切")
	addRole(editMenu, application.Copy, "Copy", "复制")
	addRole(editMenu, application.Paste, "Paste", "粘贴")
	addRole(editMenu, application.SelectAll, "Select all", "全选")
	app.Menu.Set(nativeMenu)
	menu := app.Menu.New()
	tray.SetMenu(menu)
	systemLocale := locale.System()
	refresh := func() {
		if stopping.Load() {
			return
		}
		snapshot := svc.Snapshot()
		lang := snapshot.Config.Settings.Language
		if lang == "system" {
			lang = systemLocale
		}
		zh := strings.HasPrefix(lang, "zh")
		label := func(en, cn string) string {
			if zh {
				return cn
			}
			return en
		}
		for _, entry := range labels {
			entry.item.SetLabel(label(entry.en, entry.zh))
		}
		renderTray(menu, snapshot, svc, zh, desktop.updates.Snapshot(), show, showSettings)

	}
	changed := func() {
		if stopping.Load() {
			return
		}
		desktop.updates.SetAutomatic(automaticAllowed && svc.Snapshot().Config.Settings.AutoUpdate)
		app.Event.Emit("tunnelix:changed")
		application.InvokeAsync(func() {
			if stopping.Load() {
				return
			}
			refresh()
			_, challenges := manager.Snapshot()
			if len(challenges) > 0 {
				show()
			}
		})
	}
	svc.OnChange = changed
	manager.OnChange(changed)
	app.Event.On("tunnelix:update", func(*application.CustomEvent) { application.InvokeAsync(refresh) })
	refresh()
	// Linux retains a normal close/quit path, including desktops without a tray host.
	if runtime.GOOS != "linux" {
		window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
			if !stopping.Load() {
				window.Hide()
				e.Cancel()
			}
		})
	} else {
		window.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) { app.Quit() })
	}
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		if needsConfigSelection {
			desktop.updates.SetAutomatic(false)
			go func() {
				selected, dialogErr := app.Dialog.SaveFile().
					SetMessage("Choose where Tunnelix should store its JSON configuration. / 选择 Tunnelix 保存 JSON 配置的位置。").
					SetFilename("config.json").
					AddFilter("JSON configuration", "*.json").
					CanCreateDirectories(true).
					PromptForSingleSelection()
				if stopping.Load() {
					return
				}
				if dialogErr != nil || selected == "" {
					app.Quit()
					return
				}
				if dialogErr = svc.SelectConfigPath(selected); dialogErr != nil {
					log.Printf("Cannot select Tunnelix configuration: %v", dialogErr)
					app.Quit()
					return
				}
				if dialogErr = config.RememberPath(selected); dialogErr != nil {
					log.Printf("Cannot remember Tunnelix configuration path: %v", dialogErr)
					app.Quit()
					return
				}
				window.Show()
				window.Focus()
				desktop.updates.SetAutomatic(automaticAllowed && svc.Snapshot().Config.Settings.AutoUpdate)
				go svc.AutoConnect()
			}()
			return
		}
		desktop.updates.SetAutomatic(automaticAllowed && svc.Snapshot().Config.Settings.AutoUpdate)
		go svc.AutoConnect()
	})
	var recoveryMu sync.Mutex
	var recoveryTimer *time.Timer
	scheduleRecovery := func(*application.ApplicationEvent) {
		recoveryMu.Lock()
		defer recoveryMu.Unlock()
		if recoveryTimer != nil {
			recoveryTimer.Stop()
		}
		recoveryTimer = time.AfterFunc(2*time.Second, func() {
			if !stopping.Load() {
				manager.RecoverNetwork()
			}
		})
	}
	app.Event.OnApplicationEvent(events.Common.SystemDidWake, scheduleRecovery)
	app.Event.OnApplicationEvent(events.Common.NetworkChanged, scheduleRecovery)

	if e = app.Run(); e != nil {
		manager.Close()
		log.Fatal(e)
	}
}
