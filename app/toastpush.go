package app

import (
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"runtime/debug"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
)

// This file shows a toast through the Windows Runtime directly. The Wails
// notification service (and wintoast under it) can neither tag a toast, so
// that "grid back" replaces "grid lost", nor keep a silenced one from popping
// up: both are properties of the ToastNotification object. The Wails service
// still registers the app with Windows and receives the clicks.

// WinRT interface ids and the vtable slots used. Slots 0-2 are IUnknown,
// 3-5 IInspectable; the methods of an interface start at 6.
var (
	iidXMLDocument   = ole.NewGUID("f7f3a506-1e87-42d6-bcfb-b8c809fa5494") // IXmlDocument
	iidXMLDocumentIO = ole.NewGUID("6cd0e74e-ee65-4489-9ebf-ca43e87ba637") // IXmlDocumentIO
	iidToastManager5 = ole.NewGUID("d6f5f569-d40d-407c-8989-88cab42cfd14") // IToastNotificationManagerStatics5
	iidToastFactory  = ole.NewGUID("04124b20-82c6-4229-b109-fd9ed4662b53") // IToastNotificationFactory
	iidToast         = ole.NewGUID("997e2675-059e-4e60-8b06-1760917c8b80") // IToastNotification
	iidToast2        = ole.NewGUID("9dfb9fd1-143a-490e-90bf-b9fba7132de7") // IToastNotification2
)

const (
	slotQueryInterface = 0
	slotRelease        = 2

	slotLoadXML                   = 6  // IXmlDocumentIO
	slotGetDefault                = 6  // IToastNotificationManagerStatics5
	slotCreateToastNotifierWithID = 7  // IToastNotificationManagerForUser
	slotCreateToastNotification   = 6  // IToastNotificationFactory
	slotPutTag                    = 6  // IToastNotification2
	slotPutGroup                  = 8  // IToastNotification2
	slotPutSuppressPopup          = 10 // IToastNotification2
	slotShow                      = 6  // IToastNotifier
	slotGetSetting                = 8  // IToastNotifier (get_Setting)

	roInitMultiThreaded = 1
	// maxTagUnits: Windows limits Tag and Group to 64 characters.
	maxTagUnits = 64
)

// toastPush is one toast to show. Tag and Group identify it in the
// notification centre: a later toast with the same pair replaces it.
type toastPush struct {
	AppID         string
	XML           string
	Tag, Group    string
	SuppressPopup bool // straight to the notification centre, no banner
}

var (
	winrtOnce  sync.Once
	winrtQueue chan func()
	winrtErr   error
)

// onWinRT runs fn on the one OS thread that is initialised for the Windows
// Runtime (COM objects must be used on an initialised thread, and goroutines
// move between threads).
func onWinRT(fn func() error) error {
	winrtOnce.Do(func() {
		winrtQueue = make(chan func())
		ready := make(chan struct{})
		go func() {
			runtime.LockOSThread()
			winrtErr = ole.RoInitialize(roInitMultiThreaded)
			if oe, ok := winrtErr.(*ole.OleError); ok && oe.Code() == 1 { // S_FALSE: already initialised
				winrtErr = nil
			}
			close(ready)
			for f := range winrtQueue {
				f()
			}
		}()
		<-ready
	})
	if winrtErr != nil {
		return fmt.Errorf("RoInitialize: %w", winrtErr)
	}
	done := make(chan error, 1)
	winrtQueue <- func() {
		// A failed toast must never take the tray app down with it.
		defer func() {
			if v := recover(); v != nil {
				slog.Error("winrt: panic", "panic", v, "stack", string(debug.Stack()))
				done <- fmt.Errorf("winrt: panic: %v", v)
			}
		}()
		done <- fn()
	}
	return <-done
}

// pushToast shows the toast.
func pushToast(p toastPush) error {
	return onWinRT(func() error { return showToast(p) })
}

// newToast builds the ToastNotification (as IToastNotification2) with its
// tag, group and banner flag set. The caller releases it. WinRT thread only.
func newToast(p toastPush) (unsafe.Pointer, error) {
	docObj, err := ole.RoActivateInstance("Windows.Data.Xml.Dom.XmlDocument")
	if err != nil {
		return nil, fmt.Errorf("XmlDocument: %w", err)
	}
	defer docObj.Release()
	docIO, err := queryInterface(unsafe.Pointer(docObj), iidXMLDocumentIO)
	if err != nil {
		return nil, fmt.Errorf("IXmlDocumentIO: %w", err)
	}
	defer release(docIO)
	if err := callWithString(docIO, slotLoadXML, p.XML); err != nil {
		return nil, fmt.Errorf("LoadXml: %w", err)
	}
	doc, err := queryInterface(unsafe.Pointer(docObj), iidXMLDocument)
	if err != nil {
		return nil, fmt.Errorf("IXmlDocument: %w", err)
	}
	defer release(doc)

	factory, err := ole.RoGetActivationFactory("Windows.UI.Notifications.ToastNotification", iidToastFactory)
	if err != nil {
		return nil, fmt.Errorf("ToastNotification factory: %w", err)
	}
	defer factory.Release()
	var toast unsafe.Pointer
	if err := call(unsafe.Pointer(factory), slotCreateToastNotification,
		uintptr(doc), uintptr(unsafe.Pointer(&toast))); err != nil {
		return nil, fmt.Errorf("CreateToastNotification: %w", err)
	}
	defer release(toast)
	toast2, err := queryInterface(toast, iidToast2)
	if err != nil {
		return nil, fmt.Errorf("IToastNotification2: %w", err)
	}
	if err := setToastPlace(toast2, p); err != nil {
		release(toast2)
		return nil, err
	}
	return toast2, nil
}

func setToastPlace(toast2 unsafe.Pointer, p toastPush) error {
	if p.Tag != "" {
		if err := callWithString(toast2, slotPutTag, clipUnits(p.Tag, maxTagUnits)); err != nil {
			return fmt.Errorf("put_Tag: %w", err)
		}
		if p.Group != "" {
			if err := callWithString(toast2, slotPutGroup, clipUnits(p.Group, maxTagUnits)); err != nil {
				return fmt.Errorf("put_Group: %w", err)
			}
		}
	}
	var suppress uintptr
	if p.SuppressPopup {
		suppress = 1
	}
	if err := call(toast2, slotPutSuppressPopup, suppress); err != nil {
		return fmt.Errorf("put_SuppressPopup: %w", err)
	}
	return nil
}

// showToast builds the toast and hands it to the notifier of our app id.
// WinRT thread only.
func showToast(p toastPush) error {
	toast2, err := newToast(p)
	if err != nil {
		return err
	}
	defer release(toast2)
	toast, err := queryInterface(toast2, iidToast)
	if err != nil {
		return fmt.Errorf("IToastNotification: %w", err)
	}
	defer release(toast)
	return withNotifier(p.AppID, func(notifier unsafe.Pointer) error {
		if err := call(notifier, slotShow, uintptr(toast)); err != nil {
			return fmt.Errorf("Show: %w", err)
		}
		return nil
	})
}

// withNotifier calls fn with the IToastNotifier of the app id. WinRT thread
// only.
func withNotifier(appID string, fn func(notifier unsafe.Pointer) error) error {
	statics, err := ole.RoGetActivationFactory("Windows.UI.Notifications.ToastNotificationManager", iidToastManager5)
	if err != nil {
		return fmt.Errorf("ToastNotificationManager: %w", err)
	}
	defer statics.Release()
	var manager unsafe.Pointer
	if err := call(unsafe.Pointer(statics), slotGetDefault, uintptr(unsafe.Pointer(&manager))); err != nil {
		return fmt.Errorf("GetDefault: %w", err)
	}
	defer release(manager)
	var notifier unsafe.Pointer
	if err := callWithString(manager, slotCreateToastNotifierWithID, appID,
		uintptr(unsafe.Pointer(&notifier))); err != nil {
		return fmt.Errorf("CreateToastNotifierWithId: %w", err)
	}
	defer release(notifier)
	return fn(notifier)
}

// toastsEnabled is NotificationSetting.Enabled (Windows.UI.Notifications).
// Every other value is a reason the toasts are not shown: 1 off for this
// app, 2 off for all apps, 3 group policy, 4 manifest.
const toastsEnabled = 0

// toastSetting reads the NotificationSetting of the app id's notifier: whether
// Windows shows its toasts, and if not why. It only reads.
func toastSetting(appID string) (int32, error) {
	var v int32
	err := onWinRT(func() error {
		return withNotifier(appID, func(notifier unsafe.Pointer) error {
			if err := call(notifier, slotGetSetting, uintptr(unsafe.Pointer(&v))); err != nil {
				return fmt.Errorf("get_Setting: %w", err)
			}
			return nil
		})
	})
	return v, err
}

// ToastsOff reports whether Windows blocks the toasts of the app id (for
// the app, for every app, by policy or manifest).
func ToastsOff(appID string) (bool, error) {
	v, err := toastSetting(appID)
	if err != nil {
		return false, err
	}
	return v != toastsEnabled, nil
}

// call invokes the method in the given vtable slot of a COM object.
//
// Out parameters are passed as uintptr(unsafe.Pointer(&v)). The directive
// below makes such arguments escape to the heap and stay alive for the call;
// without it v would sit on the goroutine stack, which Go may move before the
// system call, and Windows would write into the old stack.
//
//go:uintptrescapes
func call(obj unsafe.Pointer, slot int, args ...uintptr) error {
	if obj == nil {
		return errors.New("winrt: call on a nil object")
	}
	vtable := *(*unsafe.Pointer)(obj)
	method := *(*uintptr)(unsafe.Add(vtable, uintptr(slot)*unsafe.Sizeof(uintptr(0))))
	hr, _, _ := syscall.SyscallN(method, append([]uintptr{uintptr(obj)}, args...)...)
	if int32(hr) < 0 {
		return ole.NewError(hr)
	}
	return nil
}

// callWithString is call with s as the first argument, passed as an HSTRING.
// It takes out parameters the same way as call.
//
//go:uintptrescapes
func callWithString(obj unsafe.Pointer, slot int, s string, more ...uintptr) error {
	h, err := newHString(s)
	if err != nil {
		return err
	}
	defer ole.DeleteHString(h)
	return call(obj, slot, append([]uintptr{uintptr(h)}, more...)...)
}

func queryInterface(obj unsafe.Pointer, iid *ole.GUID) (unsafe.Pointer, error) {
	var out unsafe.Pointer
	err := call(obj, slotQueryInterface, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
	return out, err
}

func release(obj unsafe.Pointer) {
	if obj != nil {
		_ = call(obj, slotRelease)
	}
}

var procWindowsCreateString = windows.NewLazySystemDLL("combase.dll").NewProc("WindowsCreateString")

// newHString makes a WinRT string with its real UTF-16 length.
// (ole.NewHString passes the rune count, which cuts the end off any text
// with characters outside the BMP, such as emoji.)
func newHString(s string) (ole.HString, error) {
	u := utf16.Encode([]rune(s))
	if len(u) == 0 {
		return 0, nil // the null HSTRING is the empty string
	}
	var h ole.HString
	hr, _, _ := procWindowsCreateString.Call(
		uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)), uintptr(unsafe.Pointer(&h)))
	runtime.KeepAlive(u)
	if int32(hr) < 0 {
		return 0, ole.NewError(hr)
	}
	return h, nil
}

// clipUnits shortens s to at most n UTF-16 code units without splitting a
// character.
func clipUnits(s string, n int) string {
	units := 0
	for i, r := range s {
		units += utf16.RuneLen(r)
		if units > n {
			return s[:i]
		}
	}
	return s
}
