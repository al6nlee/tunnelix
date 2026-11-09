package locale

import (
	"os/exec"
	"strings"
)

func native() string {
	b, e := exec.Command("/usr/bin/defaults", "read", "-g", "AppleLocale").Output()
	if e != nil {
		return "en"
	}
	return strings.TrimSpace(string(b))
}
