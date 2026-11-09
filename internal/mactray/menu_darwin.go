//go:build darwin && cgo && !purego

// Package mactray keeps menu-only status items attached to their native menus.
package mactray

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
int tunnelixEnableNativeTrayMenus(void);
*/
import "C"

import "errors"

// Enable must run before Wails creates the tray. Tunnelix uses menu-only trays;
// persistent menu binding deliberately bypasses custom tray click handlers.
func Enable() error {
	if C.tunnelixEnableNativeTrayMenus() == 0 {
		return errors.New("unsupported Wails macOS tray controller")
	}
	return nil
}
