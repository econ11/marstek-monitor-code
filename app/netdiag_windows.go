package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// psTimeout bounds one diagnostic PowerShell run (it starts in about 1 s).
const psTimeout = 15 * time.Second

// firewallWait bounds the elevated PowerShell after the user said yes.
const firewallWait = 60 * time.Second

// powerShell runs a script in a hidden Windows PowerShell and returns what
// it printed (UTF-8).
func powerShell(script string) ([]byte, error) {
	ps, dir, err := systemPowerShell()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), psTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, ps, "-NoProfile", "-NonInteractive", "-EncodedCommand",
		encodeCommand("[Console]::OutputEncoding = [Text.Encoding]::UTF8\n"+script))
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("powershell: %w", err)
	}
	return out, nil
}

// systemPowerShell is the full path of Windows PowerShell and the system
// directory it runs in.
func systemPowerShell() (exe, dir string, err error) {
	dir, err = windows.GetSystemDirectory()
	if err != nil {
		return "", "", fmt.Errorf("system directory: %w", err)
	}
	return powerShellPath(dir), dir, nil
}

// systemPublicNetwork: a connected network on one of aliases (nil = any) has
// the Public profile.
func systemPublicNetwork(aliases map[string]bool) (bool, error) {
	out, err := powerShell(`Get-NetConnectionProfile | Select-Object InterfaceAlias,NetworkCategory | ConvertTo-Json`)
	if err != nil {
		return false, err
	}
	return parseNetProfilesAmong(out, aliases)
}

// systemFirewallMayBlock: the Private profile is on and no enabled rule lets
// this exe through.
func systemFirewallMayBlock() (bool, error) {
	exe, err := os.Executable()
	if err != nil {
		return false, err
	}
	profile, err := powerShell(`Get-NetFirewallProfile -Profile Private | Select-Object Enabled | ConvertTo-Json`)
	if err != nil {
		return false, err
	}
	rules, err := powerShell(rulesScript())
	if err != nil {
		return false, err
	}
	return parseFirewallFor(profile, rules, exe)
}

// systemAdapters lists the interfaces that are up, with their IPv4 networks
// and whether they have a default gateway (GetAdaptersAddresses).
func systemAdapters() ([]adapter, error) {
	size := uint32(15 << 10)
	for {
		buf := make([]byte, size)
		first := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		err := windows.GetAdaptersAddresses(windows.AF_INET, windows.GAA_FLAG_INCLUDE_GATEWAYS, 0, first, &size)
		if errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("adapters: %w", err)
		}
		var out []adapter
		for a := first; a != nil; a = a.Next {
			if a.OperStatus != windows.IfOperStatusUp || a.IfType == windows.IF_TYPE_SOFTWARE_LOOPBACK {
				continue
			}
			ad := adapter{alias: windows.UTF16PtrToString(a.FriendlyName), gateway: a.FirstGatewayAddress != nil}
			for u := a.FirstUnicastAddress; u != nil; u = u.Next {
				if ip := u.Address.IP().To4(); ip != nil {
					ad.nets = append(ad.nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(int(u.OnLinkPrefixLength), 32)})
				}
			}
			out = append(out, ad)
		}
		runtime.KeepAlive(buf)
		return out, nil
	}
}

// shellExecuteInfo is SHELLEXECUTEINFOW.
type shellExecuteInfo struct {
	cbSize      uint32
	fMask       uint32
	hwnd        uintptr
	verb        *uint16
	file        *uint16
	parameters  *uint16
	directory   *uint16
	show        int32
	instApp     uintptr
	idList      uintptr
	class       *uint16
	keyClass    uintptr
	hotKey      uint32
	iconMonitor uintptr
	process     windows.Handle
}

const (
	seeMaskNoCloseProcess = 0x00000040
	seeMaskNoAsync        = 0x00000100
)

var procShellExecuteExW = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

// systemAllowFirewall runs firewallCommand through UAC (ShellExecuteExW,
// verb "runas", owned by the window hwnd so the prompt comes to the front)
// and waits up to firewallWait for it. The user saying no is ErrCancelled; a
// failed script is an error with its exit code. When the wait runs out the
// elevated PowerShell may still be working: the error comes with running,
// closed once that process has exited (nil otherwise), so the caller keeps
// later clicks out until then.
func systemAllowFirewall(hwnd uintptr) (running <-chan struct{}, err error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	ps, sysDir, err := systemPowerShell()
	if err != nil {
		return nil, err
	}
	// ShellExecuteEx may use COM (the docs ask for an initialised thread).
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err == nil {
		defer windows.CoUninitialize()
	}
	params := "-NoProfile -NonInteractive -WindowStyle Hidden -EncodedCommand " + encodeCommand(firewallCommand(exe))
	verb, _ := windows.UTF16PtrFromString("runas")
	file, err := windows.UTF16PtrFromString(ps)
	if err != nil {
		return nil, err
	}
	dir, err := windows.UTF16PtrFromString(sysDir)
	if err != nil {
		return nil, err
	}
	args, err := windows.UTF16PtrFromString(params)
	if err != nil {
		return nil, err
	}
	info := shellExecuteInfo{fMask: seeMaskNoCloseProcess | seeMaskNoAsync, hwnd: hwnd,
		verb: verb, file: file, parameters: args, directory: dir, show: windows.SW_HIDE}
	info.cbSize = uint32(unsafe.Sizeof(info))
	if ok, _, callErr := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info))); ok == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return nil, ErrCancelled
		}
		return nil, fmt.Errorf("app: firewall rule: %w", callErr)
	}
	if info.process == 0 {
		return nil, errors.New("app: firewall rule: no process")
	}
	proc := info.process
	ev, err := windows.WaitForSingleObject(proc, uint32(firewallWait/time.Millisecond))
	if err == nil && ev == uint32(windows.WAIT_TIMEOUT) {
		// Still running elevated: wait for its exit in the background.
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer windows.CloseHandle(proc)
			windows.WaitForSingleObject(proc, windows.INFINITE)
		}()
		return done, errors.New("app: firewall rule: powershell did not finish")
	}
	defer windows.CloseHandle(proc)
	if err != nil {
		return nil, fmt.Errorf("app: firewall rule: %w", err)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(proc, &code); err != nil {
		return nil, fmt.Errorf("app: firewall rule: %w", err)
	}
	if code != 0 {
		return nil, fmt.Errorf("app: firewall rule: powershell exit code %d", code)
	}
	return nil, nil
}
