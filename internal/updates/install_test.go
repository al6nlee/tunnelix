package updates

import "testing"

func TestMacInstallationRequiresBundle(t *testing.T) {
	for _, path := range []string{"/Applications/Tunnelix.app/Contents/MacOS/Tunnelix", "/tmp/A path/Tunnelix.app/Contents/MacOS/Tunnelix"} {
		if !SupportsInstallation(path, "darwin") {
			t.Fatal(path)
		}
	}
	for _, path := range []string{"/tmp/tunnelix", "/tmp/Tunnelix.app/tunnelix", "/tmp/Contents/MacOS/tunnelix"} {
		if SupportsInstallation(path, "darwin") {
			t.Fatal(path)
		}
	}
	if !SupportsInstallation("/home/qa/tunnelix", "linux") || !SupportsInstallation(`C:\Users\qa\Tunnelix\tunnelix.exe`, "windows") {
		t.Fatal("standalone binary updates disabled")
	}
}
