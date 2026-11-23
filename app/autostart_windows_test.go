package app

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// useTempAutostartKey points the autostart code at a throw-away HKCU key so
// the test never touches the real Run key.
func useTempAutostartKey(t *testing.T) {
	t.Helper()
	path := fmt.Sprintf(`Software\MarstekMonitorTest\Run-%d`, os.Getpid())
	old := autostartKeyPath
	autostartKeyPath = path
	t.Cleanup(func() {
		autostartKeyPath = old
		_ = registry.DeleteKey(registry.CURRENT_USER, path)
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\MarstekMonitorTest`)
	})
}

func TestAutostartRoundTrip(t *testing.T) {
	useTempAutostartKey(t)

	on, err := AutostartEnabled()
	if err != nil || on {
		t.Fatalf("before set: enabled = %v, err = %v; want false, nil", on, err)
	}

	exe := `C:\Program Files\Marstek Monitor\marstek-monitor.exe`
	if err := SetAutostart(true, exe); err != nil {
		t.Fatal(err)
	}
	on, err = AutostartEnabled()
	if err != nil || !on {
		t.Fatalf("after set: enabled = %v, err = %v; want true, nil", on, err)
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, autostartKeyPath, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	v, _, err := k.GetStringValue("MarstekMonitor")
	k.Close()
	if want := `"` + exe + `" --background`; err != nil || v != want {
		t.Fatalf("Run value = %q, err = %v; want %q", v, err, want)
	}

	if err := SetAutostart(false, exe); err != nil {
		t.Fatal(err)
	}
	on, err = AutostartEnabled()
	if err != nil || on {
		t.Fatalf("after unset: enabled = %v, err = %v; want false, nil", on, err)
	}
	// Unsetting twice is not an error.
	if err := SetAutostart(false, exe); err != nil {
		t.Fatalf("second unset: %v", err)
	}
}

// useTempUninstallKey points InstalledDir at a throw-away HKCU key so the
// test never reads or writes the real «Програми й компоненти» entry.
func useTempUninstallKey(t *testing.T) {
	t.Helper()
	path := fmt.Sprintf(`Software\MarstekMonitorTest\Uninstall-%d`, os.Getpid())
	old := uninstallKeyPath
	uninstallKeyPath = path
	t.Cleanup(func() {
		uninstallKeyPath = old
		_ = registry.DeleteKey(registry.CURRENT_USER, path)
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\MarstekMonitorTest`)
	})
}

func runValue(t *testing.T) (string, bool) {
	t.Helper()
	k, err := registry.OpenKey(registry.CURRENT_USER, autostartKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer k.Close()
	v, _, err := k.GetStringValue(autostartValue)
	return v, err == nil
}

// Review Focus 5: an older hand-copied exe in Run is repointed to the
// installed app; anything else is left alone.
func TestRepointAutostart(t *testing.T) {
	const installDir = `C:\Users\u\AppData\Local\Programs\Marstek Monitor`
	installed := installDir + `\marstek-monitor.exe`
	old := `C:\Users\u\marstek-test\marstek-monitor.exe`

	t.Run("foreign Run value is repointed", func(t *testing.T) {
		useTempAutostartKey(t)
		if err := SetAutostart(true, old); err != nil {
			t.Fatal(err)
		}
		changed, err := RepointAutostart(installed, installDir)
		if err != nil || !changed {
			t.Fatalf("RepointAutostart = %v, %v; want true, nil", changed, err)
		}
		if v, _ := runValue(t); v != `"`+installed+`" --background` {
			t.Fatalf("Run value = %q", v)
		}
		// Already ours: nothing to do.
		changed, err = RepointAutostart(installed, installDir)
		if err != nil || changed {
			t.Fatalf("second RepointAutostart = %v, %v; want false, nil", changed, err)
		}
	})

	t.Run("install dir compared without case", func(t *testing.T) {
		useTempAutostartKey(t)
		if err := SetAutostart(true, old); err != nil {
			t.Fatal(err)
		}
		changed, err := RepointAutostart(installed, strings.ToLower(installDir)+`\`)
		if err != nil || !changed {
			t.Fatalf("RepointAutostart = %v, %v; want true, nil", changed, err)
		}
	})

	t.Run("Run absent: autostart stays off", func(t *testing.T) {
		useTempAutostartKey(t)
		changed, err := RepointAutostart(installed, installDir)
		if err != nil || changed {
			t.Fatalf("RepointAutostart = %v, %v; want false, nil", changed, err)
		}
		if v, ok := runValue(t); ok {
			t.Fatalf("Run value written: %q", v)
		}
	})

	for name, tc := range map[string]struct{ exe, dir string }{
		"dev or hand-copied exe": {old, installDir},
		"not installed":          {installed, ""},
		"sibling folder prefix":  {`C:\Users\u\AppData\Local\Programs\Marstek Monitor 2\marstek-monitor.exe`, installDir},
		"subfolder of install":   {installDir + `\sub\marstek-monitor.exe`, installDir},
	} {
		t.Run(name+": untouched", func(t *testing.T) {
			useTempAutostartKey(t)
			other := `C:\Elsewhere\marstek-monitor.exe`
			if err := SetAutostart(true, other); err != nil {
				t.Fatal(err)
			}
			changed, err := RepointAutostart(tc.exe, tc.dir)
			if err != nil || changed {
				t.Fatalf("RepointAutostart = %v, %v; want false, nil", changed, err)
			}
			if v, _ := runValue(t); v != `"`+other+`" --background` {
				t.Fatalf("Run value changed: %q", v)
			}
		})
	}
}

// InstalledDir reads InstallLocation from the installer's uninstall entry.
func TestInstalledDir(t *testing.T) {
	useTempUninstallKey(t)
	dir, err := InstalledDir()
	if err != nil || dir != "" {
		t.Fatalf("no entry: InstalledDir = %q, %v; want \"\", nil", dir, err)
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, uninstallKeyPath, registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	want := `C:\Users\u\AppData\Local\Programs\Marstek Monitor`
	err = k.SetStringValue("InstallLocation", want)
	k.Close()
	if err != nil {
		t.Fatal(err)
	}
	if dir, err = InstalledDir(); err != nil || dir != want {
		t.Fatalf("InstalledDir = %q, %v; want %q", dir, err, want)
	}
}
