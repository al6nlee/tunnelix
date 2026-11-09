// Isolated executable used only by TestRestartReplacesIsolatedExecutable.
package main

import (
	"context"
	"encoding/base64"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"net/http"
	"os"
	"tunnelix/internal/updates"
)

var version = "1.0.0"

type host struct{}

func (host) Emit(string, ...any) bool                              { return false }
func (host) OnEvent(string, func(any)) func()                      { return func() {} }
func (host) OpenWindow(updater.WindowOptions) updater.WindowHandle { panic("unexpected window") }
func (host) Quit()                                                 { os.Exit(0) }
func main() {
	updater.HandleHelperMode()
	if version == "2.0.0" {
		if e := os.WriteFile(os.Getenv("TUNNELIX_TEST_RESTART_MARKER"), []byte(version), 0600); e != nil {
			panic(e)
		}
		return
	}
	pub, e := base64.StdEncoding.DecodeString(os.Getenv("TUNNELIX_TEST_UPDATE_KEY"))
	if e != nil {
		panic(e)
	}
	p := &updates.Provider{Client: http.DefaultClient, BaseURL: os.Getenv("TUNNELIX_TEST_UPDATE_URL"), PublicKey: pub}
	u := updater.New(host{})
	if e = u.Init(updater.Config{CurrentVersion: version, Providers: []updater.Provider{p}, PublicKey: pub, Window: updater.WindowNone}); e != nil {
		panic(e)
	}
	if _, e = u.Check(context.Background()); e != nil {
		panic(e)
	}
	if e = u.DownloadAndInstall(context.Background()); e != nil {
		panic(e)
	}
	if e = u.Restart(context.Background()); e != nil {
		panic(e)
	}
}
