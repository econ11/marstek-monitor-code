package app

import (
	"strings"

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

// ResolveLanguage is the language in use: the user's setting ("uk" or "en";
// any other value is "en"), or, when it is "", Ukrainian for a Ukrainian
// Windows and English otherwise.
func ResolveLanguage(setting, system string) string {
	switch setting {
	case "uk", "en":
		return setting
	case "":
	default:
		return "en"
	}
	if l := strings.ToLower(system); l == "uk" || strings.HasPrefix(l, "uk-") {
		return "uk"
	}
	return "en"
}
