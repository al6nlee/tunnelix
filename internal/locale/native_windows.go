package locale

import "golang.org/x/sys/windows"

func native() string {
	langs, e := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if e == nil && len(langs) > 0 {
		return langs[0]
	}
	return "en"
}
