package updates

import "path/filepath"

// macOS releases replace an entire .app. A standalone development executable
// cannot be replaced with that payload even when it embeds a release key.
func SupportsInstallation(executable, platform string) bool {
	if platform != "darwin" {
		return true
	}
	bin := filepath.Dir(executable)
	contents := filepath.Dir(bin)
	return filepath.Base(bin) == "MacOS" && filepath.Base(contents) == "Contents" && filepath.Ext(filepath.Dir(contents)) == ".app"
}
