package tunnel

import (
	"regexp"
	"strings"
)

var shellWord = regexp.MustCompile(`^[a-zA-Z0-9_@%+=:,./-]+$`)

// CommandPreview formats the same executable and argv used by the process manager.
// Authentication environment variables and all session credentials are excluded.
// Windows uses PowerShell syntax; Unix platforms use POSIX shell syntax.
func CommandPreview(executable string, args []string, platform string) string {
	quote := func(s string) string {
		if platform == "windows" {
			return "'" + strings.ReplaceAll(s, "'", "''") + "'"
		}
		if s != "" && shellWord.MatchString(s) {
			return s
		}
		return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
	}
	words := make([]string, 0, len(args)+1)
	words = append(words, quote(executable))
	for _, arg := range args {
		words = append(words, quote(arg))
	}
	prefix := ""
	if platform == "windows" {
		prefix = "& "
	}
	return prefix + strings.Join(words, " ")
}
