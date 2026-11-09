package locale

import (
	"os"
	"strings"
)

func System() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(key); v != "" && v != "C" && v != "POSIX" {
			if strings.HasPrefix(strings.ToLower(v), "zh") {
				return "zh-CN"
			}
			return "en"
		}
	}
	if strings.HasPrefix(strings.ToLower(native()), "zh") {
		return "zh-CN"
	}
	return "en"
}
