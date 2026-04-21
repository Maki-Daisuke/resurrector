//go:build windows

// alert_on_wm_close is a small Windows test program that blocks in a GUI
// message loop until it receives WM_CLOSE, then shows an alert dialog so WM_CLOSE
// delivery can be verified manually.
package main

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32             = windows.NewLazySystemDLL("user32.dll")
	kernel32           = windows.NewLazySystemDLL("kernel32.dll")
	procCreateWindowEx = user32.NewProc("CreateWindowExW")
	procDefWindowProc  = user32.NewProc("DefWindowProcW")
	procDestroyWindow  = user32.NewProc("DestroyWindow")
	procDispatchMsg    = user32.NewProc("DispatchMessageW")
	procGetMessage     = user32.NewProc("GetMessageW")
	procLoadCursor     = user32.NewProc("LoadCursorW")
	procMessageBox     = user32.NewProc("MessageBoxW")
	procPostQuitMsg    = user32.NewProc("PostQuitMessage")
	procRegisterClass  = user32.NewProc("RegisterClassExW")
	procShowWindow     = user32.NewProc("ShowWindow")
	procTranslateMsg   = user32.NewProc("TranslateMessage")
	procUpdateWindow   = user32.NewProc("UpdateWindow")
	procGetModule      = kernel32.NewProc("GetModuleHandleW")
)

const (
	csHRedraw     = 0x0002
	csVRedraw     = 0x0001
	cwUseDefault  = ^uintptr(0x7fffffff)
	idcArrow      = 32512
	idOK          = 1
	mbOKCancel    = 0x00000001
	mbIconInfo    = 0x00000040
	ssCenter      = 0x00000001
	swShow        = 5
	swShowDefault = 10
	wmClose       = 0x0010
	wmDestroy     = 0x0002
	wsOverlapped  = 0x00000000
	wsCaption     = 0x00C00000
	wsChild       = 0x40000000
	wsSysMenu     = 0x00080000
	wsVisible     = 0x10000000
)

type point struct {
	x int32
	y int32
}

type msg struct {
	hwnd    windows.Handle
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
}

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     windows.Handle
	hIcon         windows.Handle
	hCursor       windows.Handle
	hbrBackground windows.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       windows.Handle
}

func main() {
	runtime.LockOSThread()

	instance, err := getModuleHandle()
	must(err)

	className := mustUTF16Ptr("AlertOnWMCloseWindow")
	windowTitle := mustUTF16Ptr("alert_on_wm_close")
	staticClass := mustUTF16Ptr("STATIC")
	labelText := mustUTF16Ptr("Please close this app.")

	cursor, _, err := procLoadCursor.Call(0, uintptr(idcArrow))
	if cursor == 0 {
		must(err)
	}

	wndProc := syscall.NewCallback(windowProc)
	class := wndClassEx{
		cbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		style:         csHRedraw | csVRedraw,
		lpfnWndProc:   wndProc,
		hInstance:     instance,
		hCursor:       windows.Handle(cursor),
		lpszClassName: className,
	}

	atom, _, err := procRegisterClass.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		must(fmt.Errorf("RegisterClassExW: %w", err))
	}

	windowStyle := uintptr(wsOverlapped | wsCaption | wsSysMenu | wsVisible)
	hwnd, _, err := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowTitle)),
		windowStyle,
		cwUseDefault,
		cwUseDefault,
		480,
		160,
		0,
		0,
		uintptr(instance),
		0,
	)
	if hwnd == 0 {
		must(fmt.Errorf("CreateWindowExW: %w", err))
	}

	labelStyle := uintptr(wsChild | wsVisible | ssCenter)
	label, _, err := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(staticClass)),
		uintptr(unsafe.Pointer(labelText)),
		labelStyle,
		40,
		45,
		380,
		24,
		hwnd,
		0,
		uintptr(instance),
		0,
	)
	if label == 0 {
		must(fmt.Errorf("CreateWindowExW(static): %w", err))
	}

	procShowWindow.Call(hwnd, swShowDefault)
	procUpdateWindow.Call(hwnd)

	var message msg
	for {
		ret, _, err := procGetMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		switch int32(ret) {
		case -1:
			must(fmt.Errorf("GetMessageW: %w", err))
		case 0:
			return
		default:
			procTranslateMsg.Call(uintptr(unsafe.Pointer(&message)))
			procDispatchMsg.Call(uintptr(unsafe.Pointer(&message)))
		}
	}
}

func windowProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmClose:
		// When the app was launched with SW_HIDE, make the window visible again
		// so the confirmation dialog is reliably surfaced for manual verification.
		procShowWindow.Call(hwnd, swShow)
		procUpdateWindow.Call(hwnd)
		if showInfoDialog(
			hwnd,
			"alert_on_wm_close",
			"WM_CLOSE received. Close this app?",
		) == idOK {
			procDestroyWindow.Call(hwnd)
		}
		return 0
	case wmDestroy:
		procPostQuitMsg.Call(0)
		return 0
	default:
		ret, _, _ := procDefWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
		return ret
	}
}

func showInfoDialog(owner uintptr, title, body string) uintptr {
	titlePtr := mustUTF16Ptr(title)
	bodyPtr := mustUTF16Ptr(body)
	ret, _, _ := procMessageBox.Call(
		owner,
		uintptr(unsafe.Pointer(bodyPtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		mbOKCancel|mbIconInfo,
	)
	return ret
}

func getModuleHandle() (windows.Handle, error) {
	h, _, err := procGetModule.Call(0)
	if h == 0 {
		return 0, err
	}
	return windows.Handle(h), nil
}

func mustUTF16Ptr(s string) *uint16 {
	ptr, err := windows.UTF16PtrFromString(s)
	must(err)
	return ptr
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
