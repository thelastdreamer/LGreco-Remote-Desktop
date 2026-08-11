//go:build windows

package input

import (
	"unsafe"

	"github.com/lgreco/remote-desktop/agent/internal/capture"
	"golang.org/x/sys/windows"
)

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procSetCursorPos     = user32.NewProc("SetCursorPos")
	procMouseEvent       = user32.NewProc("mouse_event")
	procKeybdEvent       = user32.NewProc("keybd_event")
)

const (
	mouseeventfLeftDown  = 0x0002
	mouseeventfLeftUp    = 0x0004
	mouseeventfRightDown = 0x0008
	mouseeventfRightUp   = 0x0010
	mouseeventfWheel     = 0x0800
	keyeventfKeyUp       = 0x0002
)

func Apply(ev Event) error {
	w, h := capture.ScreenSize()
	x := int(ev.X * float64(w))
	y := int(ev.Y * float64(h))

	switch ev.Type {
	case "mousemove":
		_, _, _ = procSetCursorPos.Call(uintptr(x), uintptr(y))
	case "mousedown":
		_, _, _ = procSetCursorPos.Call(uintptr(x), uintptr(y))
		flag := uintptr(mouseeventfLeftDown)
		if ev.Button == 2 {
			flag = mouseeventfRightDown
		}
		_, _, _ = procMouseEvent.Call(flag, 0, 0, 0, 0)
	case "mouseup":
		_, _, _ = procSetCursorPos.Call(uintptr(x), uintptr(y))
		flag := uintptr(mouseeventfLeftUp)
		if ev.Button == 2 {
			flag = mouseeventfRightUp
		}
		_, _, _ = procMouseEvent.Call(flag, 0, 0, 0, 0)
	case "wheel":
		delta := int(ev.Delta)
		if delta == 0 {
			delta = 120
		}
		_, _, _ = procMouseEvent.Call(mouseeventfWheel, 0, 0, uintptr(delta), 0)
	case "keydown":
		vk := virtualKey(ev)
		_, _, _ = procKeybdEvent.Call(uintptr(vk), 0, 0, 0)
	case "keyup":
		vk := virtualKey(ev)
		_, _, _ = procKeybdEvent.Call(uintptr(vk), 0, keyeventfKeyUp, 0)
	}
	_ = unsafe.Sizeof(0)
	return nil
}

func virtualKey(ev Event) byte {
	if ev.KeyCode > 0 && ev.KeyCode < 256 {
		return byte(ev.KeyCode)
	}
	if len(ev.Key) == 1 {
		ch := ev.Key[0]
		if ch >= 'a' && ch <= 'z' {
			return ch - 32
		}
		return ch
	}
	switch ev.Key {
	case "Enter":
		return 0x0D
	case "Backspace":
		return 0x08
	case "Tab":
		return 0x09
	case "Escape":
		return 0x1B
	case "ArrowLeft":
		return 0x25
	case "ArrowUp":
		return 0x26
	case "ArrowRight":
		return 0x27
	case "ArrowDown":
		return 0x28
	default:
		return 0
	}
}
