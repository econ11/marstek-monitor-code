package i18n

import (
	"sort"
	"strings"
	"testing"
)

var pluralForms = map[string][]string{
	"uk": {"few", "many", "one", "other"},
	"en": {"one", "other"},
}

// pluralBase splits "a.b.one" into ("a.b", "one"); ok is false for a key
// without a plural suffix.
func pluralBase(key string) (base, form string, ok bool) {
	i := strings.LastIndexByte(key, '.')
	if i < 0 {
		return key, "", false
	}
	switch key[i+1:] {
	case "one", "few", "many", "other":
		return key[:i], key[i+1:], true
	}
	return key, "", false
}

func baseKeys(cat map[string]string) []string {
	set := map[string]bool{}
	for k := range cat {
		b, _, _ := pluralBase(k)
		set[b] = true
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Both catalogues name the same strings; plural forms differ per language
// but each language has all of its own.
func TestCatalogParity(t *testing.T) {
	uk, en := baseKeys(UK), baseKeys(EN)
	if strings.Join(uk, "\n") != strings.Join(en, "\n") {
		t.Errorf("key sets differ:\nuk: %v\nen: %v", uk, en)
	}
	for lang, cat := range map[string]map[string]string{"uk": UK, "en": EN} {
		forms := map[string][]string{}
		for k := range cat {
			if b, f, ok := pluralBase(k); ok {
				forms[b] = append(forms[b], f)
			}
		}
		for b, fs := range forms {
			sort.Strings(fs)
			if strings.Join(fs, " ") != strings.Join(pluralForms[lang], " ") {
				t.Errorf("%s %s: forms %v, want %v", lang, b, fs, pluralForms[lang])
			}
		}
	}
}

// v1 is LAN-only: no tray or toast text mentions the cloud.
func TestNoCloud(t *testing.T) {
	for lang, cat := range map[string]map[string]string{"uk": UK, "en": EN} {
		for k, v := range cat {
			if l := strings.ToLower(v); strings.Contains(l, "хмар") || strings.Contains(l, "cloud") {
				t.Errorf("%s %s mentions the cloud: %q", lang, k, v)
			}
		}
	}
}

func TestSetLang(t *testing.T) {
	t.Cleanup(func() { SetLang("uk") })
	SetLang("en")
	if Lang() != "en" || T("tray.menu.quit") != "Quit" {
		t.Errorf("en: Lang %q, quit %q", Lang(), T("tray.menu.quit"))
	}
	SetLang("de")
	if Lang() != "en" {
		t.Errorf("unknown language → %q, want en", Lang())
	}
	SetLang("uk")
	if Lang() != "uk" || T("tray.menu.quit") != "Вийти" {
		t.Errorf("uk: Lang %q, quit %q", Lang(), T("tray.menu.quit"))
	}
	if T("no.such.key") != "no.such.key" {
		t.Error("a missing key must come back as the key")
	}
}

func TestPlural(t *testing.T) {
	t.Cleanup(func() { SetLang("uk") })
	SetLang("uk")
	for n, want := range map[int]string{1: "one", 21: "one", 2: "few", 24: "few", 5: "many", 11: "many", 12: "many", 0: "many"} {
		if got := Plural("k", n); got != "k."+want {
			t.Errorf("uk Plural(%d) = %q, want k.%s", n, got, want)
		}
	}
	SetLang("en")
	for n, want := range map[int]string{1: "one", 0: "other", 2: "other", 21: "other"} {
		if got := Plural("k", n); got != "k."+want {
			t.Errorf("en Plural(%d) = %q, want k.%s", n, got, want)
		}
	}
}

// A key missing in the current language falls back to the other catalogue
// before the raw key.
func TestMissingKeyFallsBack(t *testing.T) {
	t.Cleanup(func() {
		delete(UK, "test.onlyUK")
		SetLang("uk")
	})
	UK["test.onlyUK"] = "лише uk"
	SetLang("en")
	if got := T("test.onlyUK"); got != "лише uk" {
		t.Errorf("en, key only in uk: %q, want the uk text", got)
	}
}
