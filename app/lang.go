package app

import "strings"

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
