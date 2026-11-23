package app

import (
	"golang.org/x/sys/windows"
)

// SystemLanguage is the first Windows display language ("uk-UA"), "" if
// unknown.
func SystemLanguage() string {
	ls, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil || len(ls) == 0 {
		return ""
	}
	return ls[0]
}
