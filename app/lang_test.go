package app

import "testing"

func TestResolveLanguage(t *testing.T) {
	for _, c := range []struct{ setting, system, want string }{
		{"", "uk-UA", "uk"},
		{"", "uk", "uk"},
		{"", "en-US", "en"},
		{"", "ru-RU", "en"},
		{"", "", "en"},
		{"uk", "en-US", "uk"},
		{"en", "uk-UA", "en"},
	} {
		if got := ResolveLanguage(c.setting, c.system); got != c.want {
			t.Errorf("ResolveLanguage(%q, %q) = %q, want %q", c.setting, c.system, got, c.want)
		}
	}
}

// A setting other than uk / en (old version, hand-edited DB) is English.
func TestResolveLanguageUnknownSetting(t *testing.T) {
	if got := ResolveLanguage("de", "uk-UA"); got != "en" {
		t.Errorf("ResolveLanguage(de, uk-UA) = %q, want en", got)
	}
}
