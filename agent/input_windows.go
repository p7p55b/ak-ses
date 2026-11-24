package main

import (
	"syscall"
	"unsafe"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	procSetCursorPos = user32.NewProc("SetCursorPos")
	procMouseEvent   = user32.NewProc("mouse_event")
	procKeybdEvent   = user32.NewProc("keybd_event")
	procSendInput    = user32.NewProc("SendInput")
)

const (
	MOUSEEVENTF_MOVE       = 0x0001
	MOUSEEVENTF_LEFTDOWN   = 0x0002
	MOUSEEVENTF_LEFTUP     = 0x0004
	MOUSEEVENTF_RIGHTDOWN  = 0x0008
	MOUSEEVENTF_RIGHTUP    = 0x0010
	MOUSEEVENTF_MIDDLEDOWN = 0x0020
	MOUSEEVENTF_MIDDLEUP   = 0x0040
	MOUSEEVENTF_WHEEL      = 0x0800
	MOUSEEVENTF_ABSOLUTE   = 0x8000

	KEYEVENTF_KEYDOWN = 0x0000
	KEYEVENTF_KEYUP   = 0x0002
	KEYEVENTF_UNICODE = 0x0004

	INPUT_KEYBOARD = 1
)

type KEYBDINPUT struct {
	WVk         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type INPUT struct {
	Type uint32
	Ki   KEYBDINPUT
	_pad [8]byte // padding to match Windows 64-bit union alignment
}

func MoveMouse(x, y int) {
	procSetCursorPos.Call(uintptr(x), uintptr(y))
}

func MouseButton(button string, down bool) {
	var flag uintptr
	switch button {
	case "left":
		if down {
			flag = MOUSEEVENTF_LEFTDOWN
		} else {
			flag = MOUSEEVENTF_LEFTUP
		}
	case "right":
		if down {
			flag = MOUSEEVENTF_RIGHTDOWN
		} else {
			flag = MOUSEEVENTF_RIGHTUP
		}
	case "middle":
		if down {
			flag = MOUSEEVENTF_MIDDLEDOWN
		} else {
			flag = MOUSEEVENTF_MIDDLEUP
		}
	}
	if flag != 0 {
		procMouseEvent.Call(flag, 0, 0, 0, 0)
	}
}

func MouseScroll(delta int) {
	procMouseEvent.Call(uintptr(MOUSEEVENTF_WHEEL), 0, 0, uintptr(delta), 0)
}

func KeyInput(vkCode uint16, down bool) {
	var flags uintptr
	if !down {
		flags = KEYEVENTF_KEYUP
	}
	procKeybdEvent.Call(uintptr(vkCode), 0, flags, 0)
}

func SendUnicodeChar(char rune) {
	var inputs [2]INPUT

	// KeyDown
	inputs[0].Type = INPUT_KEYBOARD
	inputs[0].Ki = KEYBDINPUT{
		WVk:     0,
		WScan:   uint16(char),
		DwFlags: KEYEVENTF_UNICODE,
	}

	// KeyUp
	inputs[1].Type = INPUT_KEYBOARD
	inputs[1].Ki = KEYBDINPUT{
		WVk:     0,
		WScan:   uint16(char),
		DwFlags: KEYEVENTF_UNICODE | KEYEVENTF_KEYUP,
	}

	procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		uintptr(unsafe.Sizeof(inputs[0])),
	)
}
