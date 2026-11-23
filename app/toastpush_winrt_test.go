//go:build windows

package app

import (
	"testing"
	"unsafe"

	"github.com/go-ole/go-ole"
)

// Getters of IToastNotification2, next to the setters the app uses.
const (
	slotGetTag           = 7
	slotGetGroup         = 9
	slotGetSuppressPopup = 11
)

func getString(t *testing.T, obj unsafe.Pointer, slot int) string {
	t.Helper()
	var h ole.HString
	if err := call(obj, slot, uintptr(unsafe.Pointer(&h))); err != nil {
		t.Fatalf("getter in slot %d: %v", slot, err)
	}
	defer ole.DeleteHString(h)
	return h.String()
}

// Builds a real ToastNotification (nothing is shown) and reads back what the
// app set: this checks the interface id, the vtable slots and the string
// lengths against Windows itself.
func TestNewToastSetsTagGroupAndSuppressPopup(t *testing.T) {
	xml, err := toastXML("🕯️ Зникло світло", "14:05 · заряд 95%", "Дім · VenusE 3.0", "file:///C:/icons/toast-off.png", "bGF1bmNo", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []toastPush{
		{XML: xml, Tag: "outage-7", Group: "5100000000ff", SuppressPopup: true},
		{XML: xml, Tag: "🔋 full", Group: "станція", SuppressPopup: false}, // lengths with non-ASCII text
		{XML: xml}, // nothing set
	} {
		err := onWinRT(func() error {
			toast, err := newToast(p)
			if err != nil {
				return err
			}
			defer release(toast)
			if got := getString(t, toast, slotGetTag); got != p.Tag {
				t.Errorf("Tag = %q, want %q", got, p.Tag)
			}
			if got := getString(t, toast, slotGetGroup); got != p.Group {
				t.Errorf("Group = %q, want %q", got, p.Group)
			}
			var suppress byte
			if err := call(toast, slotGetSuppressPopup, uintptr(unsafe.Pointer(&suppress))); err != nil {
				return err
			}
			if (suppress != 0) != p.SuppressPopup {
				t.Errorf("SuppressPopup = %d, want %v", suppress, p.SuppressPopup)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("%+v: %v", p, err)
		}
	}
}

// Windows must reject a broken document instead of showing something else.
func TestNewToastRejectsBadXML(t *testing.T) {
	err := onWinRT(func() error {
		toast, err := newToast(toastPush{XML: "<toast><visual>"})
		release(toast)
		return err
	})
	if err == nil {
		t.Fatal("no error for malformed XML")
	}
}

// A panic while building a toast is reported as an error and the WinRT
// worker stays alive for the next toast.
func TestOnWinRTSurvivesPanic(t *testing.T) {
	err := onWinRT(func() error { panic("boom") })
	if err == nil {
		t.Fatal("no error after a panic")
	}
	ran := false
	if err := onWinRT(func() error { ran = true; return nil }); err != nil || !ran {
		t.Fatalf("worker did not run the next call: ran = %v, err = %v", ran, err)
	}
	// A nil COM object is an error, not a crash.
	if err := onWinRT(func() error { return call(nil, slotRelease) }); err == nil {
		t.Fatal("call(nil) returned no error")
	}
}

// Task 20c: the NotificationSetting read works against Windows itself (the
// interface and the vtable slot): for an app id Windows has never seen it
// either answers with a known value or fails, and never crashes. It only
// reads; nothing is registered or shown.
func TestToastSettingReadsWindows(t *testing.T) {
	v, err := toastSetting("MarstekMonitor.Test.Unregistered")
	if err != nil {
		t.Logf("unregistered app id: %v (an error is fine)", err)
		return
	}
	if v < 0 || v > 4 {
		t.Fatalf("NotificationSetting = %d, want 0..4", v)
	}
}
