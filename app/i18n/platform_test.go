package i18n

import (
	"regexp"
	"testing"
)

// windowsWords are words a Mac user must not see (the frontend's
// catalog.test.ts has the same list).
var windowsWords = regexp.MustCompile(`(?i)Windows|трей|тре[юїя]|tray|right-click|правою кнопкою|панел(ь|і) задач|брандмауер|firewall|UAC|інсталятор|installer`)

// Every Go string a Mac can show (each key resolved as T resolves it on
// darwin) is free of Windows words.
func TestDarwinStringsHaveNoWindowsWords(t *testing.T) {
	for lang, cat := range map[string]map[string]string{"uk": UK, "en": EN} {
		for k := range cat {
			v := cat[PlatformKey(k, "darwin")]
			if windowsWords.MatchString(v) {
				t.Errorf("%s %s on darwin: %q", lang, k, v)
			}
		}
	}
}

func TestPlatformKey(t *testing.T) {
	const k = "tray.firstMinimize.toast.body"
	if got := PlatformKey(k, "darwin"); got != k+".mac" {
		t.Errorf("darwin: %q, want %q", got, k+".mac")
	}
	if got := PlatformKey(k, "windows"); got != k {
		t.Errorf("windows: %q, want %q", got, k)
	}
	if got := PlatformKey("tray.menu.quit", "darwin"); got != "tray.menu.quit" {
		t.Errorf("darwin, no .mac variant: %q", got)
	}
}

// T picks the .mac variant by the build's GOOS.
func TestTUsesPlatform(t *testing.T) {
	old := goos
	t.Cleanup(func() { goos = old; SetLang("uk") })
	SetLang("en")
	goos = "darwin"
	if got := T("tray.firstMinimize.toast.title"); got != EN["tray.firstMinimize.toast.title.mac"] {
		t.Errorf("darwin title %q", got)
	}
	goos = "windows"
	if got := T("tray.firstMinimize.toast.title"); got != EN["tray.firstMinimize.toast.title"] {
		t.Errorf("windows title %q", got)
	}
}
