// Package gui provides a native Windows GUI for the PMPC Network Tester.
// It uses pure Win32 API calls via syscall to create a dark-themed interface
// that works reliably over RDP without requiring OpenGL or other GPU acceleration.
//
// Features:
//   - Dark mode theme with custom color palette
//   - Owner-drawn button with accent color
//   - Owner-drawn listbox with color-coded results (red for failures)
//   - Progress bar for test progress
//   - Resizable window layout
//   - Concurrent network testing with configurable parallelism
//
// The GUI uses a timer-based architecture to safely update UI elements
// from the main Windows message loop while tests run in background goroutines.
package gui

import (
	"encoding/csv"
	"fmt"
	"net"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/PatchMyPCTeam/PMPC-NetworkTester/packages/downloadFile"
	"github.com/PatchMyPCTeam/PMPC-NetworkTester/packages/goCMTrace"
)

const (
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_CHILD            = 0x40000000
	WS_VISIBLE          = 0x10000000
	WS_BORDER           = 0x00800000
	WS_VSCROLL          = 0x00200000
	WS_HSCROLL          = 0x00100000
	WS_TABSTOP          = 0x00010000
	WS_EX_CLIENTEDGE    = 0x00000200
	WS_CLIPCHILDREN     = 0x02000000

	WM_DESTROY         = 0x0002
	WM_SIZE            = 0x0005
	WM_CLOSE           = 0x0010
	WM_COMMAND         = 0x0111
	WM_TIMER           = 0x0113
	WM_CTLCOLORSTATIC  = 0x0138
	WM_CTLCOLORLISTBOX = 0x0134
	WM_USER            = 0x0400
	WM_SETFONT         = 0x0030

	BS_PUSHBUTTON        = 0x0000
	BS_OWNERDRAW         = 0x000B
	LBS_NOSEL            = 0x4000
	LBS_HASSTRINGS       = 0x0040
	LBS_NOINTEGRALHEIGHT = 0x0100
	LBS_OWNERDRAWFIXED   = 0x0010
	PBS_SMOOTH           = 0x01
	SS_LEFT              = 0x0000

	LB_ADDSTRING     = 0x0180
	LB_RESETCONTENT  = 0x0184
	LB_SETTOPINDEX   = 0x0197
	LB_GETCOUNT      = 0x018B
	LB_GETTEXT       = 0x0189
	LB_GETTEXTLEN    = 0x018A
	LB_SETITEMHEIGHT = 0x01A0

	PBM_SETPOS      = WM_USER + 2
	PBM_SETRANGE    = WM_USER + 1
	PBM_SETBARCOLOR = WM_USER + 9
	PBM_SETBKCOLOR  = 0x2001

	WM_DRAWITEM    = 0x002B
	WM_MEASUREITEM = 0x002C
	ODT_BUTTON     = 4
	ODT_LISTBOX    = 2
	ODS_SELECTED   = 0x0001
	ODS_DISABLED   = 0x0004

	DT_CENTER     = 0x0001
	DT_VCENTER    = 0x0004
	DT_SINGLELINE = 0x0020
	DT_LEFT       = 0x0000
	DT_NOPREFIX   = 0x0800

	SW_SHOWNORMAL = 1
	IDC_ARROW     = 32512
	BN_CLICKED    = 0

	FW_NORMAL           = 400
	FW_BOLD             = 700
	DEFAULT_CHARSET     = 1
	OUT_DEFAULT_PRECIS  = 0
	CLIP_DEFAULT_PRECIS = 0
	DEFAULT_QUALITY     = 0
	DEFAULT_PITCH       = 0
	FF_DONTCARE         = 0
	TRANSPARENT         = 1
	COLOR_WINDOW        = 5

	ID_BUTTON      = 1003
	ID_LISTBOX     = 1004
	TIMER_ID       = 1
	TIMER_INTERVAL = 100

	MAX_CONCURRENT = 20

	// Color palette (BGR format for Windows)
	COLOR_DARK     = 0x211515 // #151521
	COLOR_WHITE    = 0xFFFFFF // #FFFFFF
	COLOR_ACCENT   = 0x9BBC1B // #1BBC9B
	COLOR_DISABLED = 0x978D85 // #858D97
	COLOR_HOVER    = 0x2A1E1E // slightly lighter dark for hover/pressed
	COLOR_ERROR    = 0x6C41F1 // #F1416C (red for failures)
)

const mainLogFile = "PMPC-NetworkTester.log"

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	comctl32 = syscall.NewLazyDLL("comctl32.dll")

	procRegisterClassExW     = user32.NewProc("RegisterClassExW")
	procCreateWindowExW      = user32.NewProc("CreateWindowExW")
	procDefWindowProcW       = user32.NewProc("DefWindowProcW")
	procGetMessageW          = user32.NewProc("GetMessageW")
	procTranslateMessage     = user32.NewProc("TranslateMessage")
	procDispatchMessageW     = user32.NewProc("DispatchMessageW")
	procPostQuitMessage      = user32.NewProc("PostQuitMessage")
	procShowWindow           = user32.NewProc("ShowWindow")
	procUpdateWindow         = user32.NewProc("UpdateWindow")
	procSendMessageW         = user32.NewProc("SendMessageW")
	procSetWindowTextW       = user32.NewProc("SetWindowTextW")
	procEnableWindow         = user32.NewProc("EnableWindow")
	procLoadCursorW          = user32.NewProc("LoadCursorW")
	procLoadIconW            = user32.NewProc("LoadIconW")
	procSetTimer             = user32.NewProc("SetTimer")
	procKillTimer            = user32.NewProc("KillTimer")
	procMoveWindow           = user32.NewProc("MoveWindow")
	procGetModuleHandleW     = kernel32.NewProc("GetModuleHandleW")
	procCreateSolidBrush     = gdi32.NewProc("CreateSolidBrush")
	procSetTextColor         = gdi32.NewProc("SetTextColor")
	procSetBkColor           = gdi32.NewProc("SetBkColor")
	procSetBkMode            = gdi32.NewProc("SetBkMode")
	procCreateFontW          = gdi32.NewProc("CreateFontW")
	procDeleteObject         = gdi32.NewProc("DeleteObject")
	procSelectObject         = gdi32.NewProc("SelectObject")
	procFillRect             = user32.NewProc("FillRect")
	procDrawTextW            = user32.NewProc("DrawTextW")
	procFrameRect            = user32.NewProc("FrameRect")
	procInflateRect          = user32.NewProc("InflateRect")
	procInitCommonControlsEx = comctl32.NewProc("InitCommonControlsEx")
)

type WNDCLASSEXW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   syscall.Handle
	Icon       syscall.Handle
	Cursor     syscall.Handle
	Background syscall.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     syscall.Handle
}

type MSG struct {
	Hwnd    syscall.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

type INITCOMMONCONTROLSEX struct {
	Size uint32
	ICC  uint32
}

type RECT struct {
	Left, Top, Right, Bottom int32
}

type DRAWITEMSTRUCT struct {
	CtlType    uint32
	CtlID      uint32
	ItemID     uint32
	ItemAction uint32
	ItemState  uint32
	HwndItem   syscall.Handle
	HDC        syscall.Handle
	RcItem     RECT
	ItemData   uintptr
}

type MEASUREITEMSTRUCT struct {
	CtlType    uint32
	CtlID      uint32
	ItemID     uint32
	ItemWidth  uint32
	ItemHeight uint32
	ItemData   uintptr
}

// ConnectionInfo holds information about a network endpoint to test.
type ConnectionInfo struct {
	Product    string // Name of the product requiring this connection
	DomainName string // Hostname or domain to connect to
	Port       string // Port number for the connection
	Reason     string // Description of why this connection is needed
}

// NetworkTesterGUI is the main GUI controller for the network tester application.
// It manages the Windows GUI controls, handles user interaction, and coordinates
// the concurrent network tests.
type NetworkTesterGUI struct {
	// Window handles
	hwnd        syscall.Handle
	hInstance   syscall.Handle
	btnStart    syscall.Handle
	lblStatus   syscall.Handle
	lblTitle    syscall.Handle
	progressBar syscall.Handle
	listBox     syscall.Handle

	// GDI resources
	hFont       syscall.Handle
	hFontBold   syscall.Handle
	hIcon       syscall.Handle
	darkBrush   uintptr
	accentBrush uintptr
	hoverBrush  uintptr

	// State - use int32 for atomic ops
	isRunning int32
	testsDone int32

	// Thread-safe result queue
	resultsMu sync.Mutex
	results   []string

	// Atomic counters
	totalTests     int32
	completedTests int32
	successCount   int32
	failCount      int32

	// Pre-allocated UTF16 strings to prevent GC issues
	strRunning   *uint16
	strRunAgain  *uint16
	strStartTest *uint16

	// Current button text pointer (for drawing)
	currentBtnText *uint16
}

var globalGUI *NetworkTesterGUI

func logInfo(msg string) {
	goCMTrace.LogData(goCMTrace.LogEntry{
		File:    mainLogFile,
		Message: fmt.Sprintf("[GUI] %s", msg),
		State:   1,
	})
}

func logError(msg string) {
	goCMTrace.LogData(goCMTrace.LogEntry{
		File:    mainLogFile,
		Message: fmt.Sprintf("[GUI ERROR] %s", msg),
		State:   3,
	})
}

// NewNetworkTesterGUI creates a new GUI instance and initializes pre-allocated strings.
func NewNetworkTesterGUI() *NetworkTesterGUI {
	logInfo("Creating new NetworkTesterGUI instance")
	gui := &NetworkTesterGUI{}
	globalGUI = gui

	// Pre-allocate common strings - these stay alive for the program lifetime
	gui.strRunning, _ = syscall.UTF16PtrFromString("Running...")
	gui.strRunAgain, _ = syscall.UTF16PtrFromString("Run Again")
	gui.strStartTest, _ = syscall.UTF16PtrFromString("Start Test")
	gui.currentBtnText = gui.strStartTest

	return gui
}

// Run initializes and runs the Windows GUI message loop.
// This method blocks until the window is closed.
// IMPORTANT: This must be called from the main goroutine as Windows GUI
// requires all window operations to happen on the same OS thread.
func (g *NetworkTesterGUI) Run() {
	// Lock this goroutine to the OS thread - CRITICAL for Windows GUI
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	logInfo("Starting GUI Run()")

	// Initialize common controls
	icc := INITCOMMONCONTROLSEX{
		Size: uint32(unsafe.Sizeof(INITCOMMONCONTROLSEX{})),
		ICC:  0x00000020,
	}
	procInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&icc)))

	h, _, _ := procGetModuleHandleW.Call(0)
	g.hInstance = syscall.Handle(h)

	// Create brushes
	db, _, _ := procCreateSolidBrush.Call(COLOR_DARK)
	g.darkBrush = db
	ab, _, _ := procCreateSolidBrush.Call(COLOR_ACCENT)
	g.accentBrush = ab
	hb, _, _ := procCreateSolidBrush.Call(COLOR_HOVER)
	g.hoverBrush = hb

	// Create fonts (negative height = point size)
	fontName, _ := syscall.UTF16PtrFromString("Segoe UI")
	fontHeight := int32(-20)
	f, _, _ := procCreateFontW.Call(uintptr(fontHeight), 0, 0, 0, FW_NORMAL, 0, 0, 0,
		DEFAULT_CHARSET, OUT_DEFAULT_PRECIS, CLIP_DEFAULT_PRECIS,
		DEFAULT_QUALITY, DEFAULT_PITCH|FF_DONTCARE, uintptr(unsafe.Pointer(fontName)))
	g.hFont = syscall.Handle(f)

	fontHeightBold := int32(-24)
	fb, _, _ := procCreateFontW.Call(uintptr(fontHeightBold), 0, 0, 0, FW_BOLD, 0, 0, 0,
		DEFAULT_CHARSET, OUT_DEFAULT_PRECIS, CLIP_DEFAULT_PRECIS,
		DEFAULT_QUALITY, DEFAULT_PITCH|FF_DONTCARE, uintptr(unsafe.Pointer(fontName)))
	g.hFontBold = syscall.Handle(fb)

	// Load icon
	icon, _, _ := procLoadIconW.Call(uintptr(g.hInstance), uintptr(1))
	g.hIcon = syscall.Handle(icon)

	// Register window class
	className, _ := syscall.UTF16PtrFromString("PMPCNetworkTester")
	cur, _, _ := procLoadCursorW.Call(0, IDC_ARROW)

	wc := WNDCLASSEXW{
		Size:       uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		Style:      0,
		WndProc:    syscall.NewCallback(wndProc),
		Instance:   g.hInstance,
		Icon:       g.hIcon,
		Cursor:     syscall.Handle(cur),
		Background: syscall.Handle(g.darkBrush),
		ClassName:  className,
		IconSm:     g.hIcon,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	// Create main window
	windowTitle, _ := syscall.UTF16PtrFromString("Patch My PC Network Tester")
	hwnd, _, _ := procCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowTitle)),
		WS_OVERLAPPEDWINDOW|WS_CLIPCHILDREN,
		100, 100, 920, 680,
		0, 0, uintptr(g.hInstance), 0)
	g.hwnd = syscall.Handle(hwnd)

	g.createControls()

	procShowWindow.Call(uintptr(g.hwnd), SW_SHOWNORMAL)
	procUpdateWindow.Call(uintptr(g.hwnd))
	logInfo("Window shown, entering message loop")

	var msg MSG
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if ret == 0 || int32(ret) == -1 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	logInfo("GUI Run() completed")
}

func (g *NetworkTesterGUI) createControls() {
	staticClass, _ := syscall.UTF16PtrFromString("STATIC")
	buttonClass, _ := syscall.UTF16PtrFromString("BUTTON")
	progressClass, _ := syscall.UTF16PtrFromString("msctls_progress32")
	listboxClass, _ := syscall.UTF16PtrFromString("LISTBOX")
	startText, _ := syscall.UTF16PtrFromString("Start Test")

	// Title label
	titleText, _ := syscall.UTF16PtrFromString("Patch My PC Network Tester")
	lt, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(staticClass)), uintptr(unsafe.Pointer(titleText)),
		WS_CHILD|WS_VISIBLE|SS_LEFT, 20, 20, 860, 30,
		uintptr(g.hwnd), 1000, uintptr(g.hInstance), 0)
	g.lblTitle = syscall.Handle(lt)
	procSendMessageW.Call(uintptr(g.lblTitle), WM_SETFONT, uintptr(g.hFontBold), 1)

	// Status label
	statusText, _ := syscall.UTF16PtrFromString("Click 'Start Test' to begin network connectivity tests")
	ls, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(staticClass)), uintptr(unsafe.Pointer(statusText)),
		WS_CHILD|WS_VISIBLE|SS_LEFT, 20, 60, 860, 25,
		uintptr(g.hwnd), 1001, uintptr(g.hInstance), 0)
	g.lblStatus = syscall.Handle(ls)
	procSendMessageW.Call(uintptr(g.lblStatus), WM_SETFONT, uintptr(g.hFont), 1)

	// Progress bar
	pb, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(progressClass)), 0,
		WS_CHILD|WS_VISIBLE|PBS_SMOOTH, 20, 95, 860, 25,
		uintptr(g.hwnd), 1002, uintptr(g.hInstance), 0)
	g.progressBar = syscall.Handle(pb)
	procSendMessageW.Call(uintptr(g.progressBar), PBM_SETRANGE, 0, 100<<16)
	procSendMessageW.Call(uintptr(g.progressBar), PBM_SETBARCOLOR, 0, COLOR_ACCENT)
	procSendMessageW.Call(uintptr(g.progressBar), PBM_SETBKCOLOR, 0, COLOR_DARK)

	// Button (owner-drawn)
	bs, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(buttonClass)), uintptr(unsafe.Pointer(startText)),
		WS_CHILD|WS_VISIBLE|BS_OWNERDRAW|WS_TABSTOP, 390, 135, 140, 40,
		uintptr(g.hwnd), ID_BUTTON, uintptr(g.hInstance), 0)
	g.btnStart = syscall.Handle(bs)
	procSendMessageW.Call(uintptr(g.btnStart), WM_SETFONT, uintptr(g.hFont), 1)

	// Listbox (owner-drawn for colored items)
	lb, _, _ := procCreateWindowExW.Call(WS_EX_CLIENTEDGE, uintptr(unsafe.Pointer(listboxClass)), 0,
		WS_CHILD|WS_VISIBLE|WS_BORDER|WS_VSCROLL|WS_HSCROLL|LBS_HASSTRINGS|LBS_NOSEL|LBS_NOINTEGRALHEIGHT|LBS_OWNERDRAWFIXED,
		20, 190, 860, 420,
		uintptr(g.hwnd), ID_LISTBOX, uintptr(g.hInstance), 0)
	g.listBox = syscall.Handle(lb)
	procSendMessageW.Call(uintptr(g.listBox), WM_SETFONT, uintptr(g.hFont), 1)
	// Set item height
	procSendMessageW.Call(uintptr(g.listBox), LB_SETITEMHEIGHT, 0, 24)

	logInfo("All controls created")
}

func wndProc(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	if globalGUI == nil {
		ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
		return ret
	}

	switch msg {
	case WM_COMMAND:
		ctrlID := int(wParam & 0xFFFF)
		notifyCode := int((wParam >> 16) & 0xFFFF)
		if ctrlID == ID_BUTTON && notifyCode == BN_CLICKED {
			if atomic.LoadInt32(&globalGUI.isRunning) == 0 {
				logInfo("Starting tests from button click")
				globalGUI.startTests()
			}
		}
		ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
		return ret

	case WM_TIMER:
		if wParam == TIMER_ID {
			globalGUI.onTimer()
		}
		return 0

	case WM_CTLCOLORSTATIC:
		if globalGUI.darkBrush != 0 {
			procSetTextColor.Call(wParam, COLOR_WHITE)
			procSetBkMode.Call(wParam, TRANSPARENT)
			return globalGUI.darkBrush
		}
		ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
		return ret

	case WM_CTLCOLORLISTBOX:
		if globalGUI.darkBrush != 0 {
			procSetTextColor.Call(wParam, COLOR_WHITE)
			procSetBkColor.Call(wParam, COLOR_DARK)
			return globalGUI.darkBrush
		}
		ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
		return ret

	case WM_DRAWITEM:
		dis := (*DRAWITEMSTRUCT)(unsafe.Pointer(lParam))
		if dis != nil {
			if dis.CtlID == ID_BUTTON {
				globalGUI.drawButton(dis)
				return 1
			} else if dis.CtlID == ID_LISTBOX {
				globalGUI.drawListItem(dis)
				return 1
			}
		}
		ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
		return ret

	case WM_MEASUREITEM:
		mis := (*MEASUREITEMSTRUCT)(unsafe.Pointer(lParam))
		if mis != nil && mis.CtlID == ID_LISTBOX {
			mis.ItemHeight = 24 // Set item height for listbox
			return 1
		}
		ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
		return ret

	case WM_SIZE:
		if globalGUI != nil {
			width := int(lParam & 0xFFFF)
			height := int((lParam >> 16) & 0xFFFF)
			globalGUI.onResize(width, height)
		}
		return 0

	case WM_CLOSE, WM_DESTROY:
		logInfo("Window closing")
		procKillTimer.Call(uintptr(hwnd), TIMER_ID)
		procPostQuitMessage.Call(0)
		return 0

	default:
		ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
		return ret
	}
}

func (g *NetworkTesterGUI) drawButton(dis *DRAWITEMSTRUCT) {
	if dis == nil || g == nil {
		return
	}

	hdc := dis.HDC
	rc := dis.RcItem

	// Determine colors based on state
	var bgBrush uintptr
	var textColor uintptr

	if dis.ItemState&ODS_DISABLED != 0 {
		// Disabled: dark background with gray text
		bgBrush = g.darkBrush
		textColor = COLOR_DISABLED
	} else if dis.ItemState&ODS_SELECTED != 0 {
		// Pressed: slightly darker accent
		bgBrush = g.hoverBrush
		textColor = COLOR_WHITE
	} else {
		// Normal: accent background with white text
		bgBrush = g.accentBrush
		textColor = COLOR_WHITE
	}

	// Fill background with color
	procFillRect.Call(uintptr(hdc), uintptr(unsafe.Pointer(&rc)), bgBrush)

	// Draw text
	procSetBkMode.Call(uintptr(hdc), TRANSPARENT)
	procSetTextColor.Call(uintptr(hdc), textColor)

	// Select font
	procSelectObject.Call(uintptr(hdc), uintptr(g.hFont))

	// Get text to draw
	text := g.currentBtnText
	if text == nil {
		text = g.strStartTest
	}

	// Calculate text length
	textLen := 0
	for p := text; *p != 0; p = (*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(p)) + 2)) {
		textLen++
	}

	// Draw centered text
	procDrawTextW.Call(uintptr(hdc), uintptr(unsafe.Pointer(text)), uintptr(textLen),
		uintptr(unsafe.Pointer(&rc)), DT_CENTER|DT_VCENTER|DT_SINGLELINE)
}

func (g *NetworkTesterGUI) drawListItem(dis *DRAWITEMSTRUCT) {
	if dis == nil || g == nil {
		return
	}

	// Skip if no valid item
	if dis.ItemID == 0xFFFFFFFF {
		return
	}

	hdc := dis.HDC
	rc := dis.RcItem

	// Get the text for this item
	textLen, _, _ := procSendMessageW.Call(uintptr(g.listBox), LB_GETTEXTLEN, uintptr(dis.ItemID), 0)
	if textLen == 0 || int32(textLen) < 0 {
		return
	}

	// Allocate buffer and get text
	buf := make([]uint16, textLen+1)
	procSendMessageW.Call(uintptr(g.listBox), LB_GETTEXT, uintptr(dis.ItemID), uintptr(unsafe.Pointer(&buf[0])))

	// Convert to string to check for failure
	itemText := syscall.UTF16ToString(buf)
	isFailed := len(itemText) >= 6 && itemText[:6] == "[FAIL]"

	// Fill background
	procFillRect.Call(uintptr(hdc), uintptr(unsafe.Pointer(&rc)), g.darkBrush)

	// Set text color based on success/failure
	if isFailed {
		procSetTextColor.Call(uintptr(hdc), COLOR_ERROR)
	} else {
		procSetTextColor.Call(uintptr(hdc), COLOR_WHITE)
	}
	procSetBkMode.Call(uintptr(hdc), TRANSPARENT)

	// Select font
	procSelectObject.Call(uintptr(hdc), uintptr(g.hFont))

	// Add left padding
	rc.Left += 4

	// Draw text
	procDrawTextW.Call(uintptr(hdc), uintptr(unsafe.Pointer(&buf[0])), uintptr(textLen),
		uintptr(unsafe.Pointer(&rc)), DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_NOPREFIX)
}

func (g *NetworkTesterGUI) onResize(width, height int) {
	if g == nil {
		return
	}

	// Margins
	margin := 20
	listboxTop := 190

	// Resize listbox to fill remaining space
	if g.listBox != 0 {
		newWidth := width - (margin * 2)
		newHeight := height - listboxTop - margin
		if newWidth > 0 && newHeight > 0 {
			procMoveWindow.Call(uintptr(g.listBox), uintptr(margin), uintptr(listboxTop),
				uintptr(newWidth), uintptr(newHeight), 1)
		}
	}

	// Resize progress bar width
	if g.progressBar != 0 {
		newWidth := width - (margin * 2)
		if newWidth > 0 {
			procMoveWindow.Call(uintptr(g.progressBar), uintptr(margin), 95,
				uintptr(newWidth), 25, 1)
		}
	}

	// Center the button
	if g.btnStart != 0 {
		btnWidth := 140
		btnX := (width - btnWidth) / 2
		procMoveWindow.Call(uintptr(g.btnStart), uintptr(btnX), 135,
			uintptr(btnWidth), 40, 1)
	}

	// Resize title and status labels
	if g.lblTitle != 0 {
		newWidth := width - (margin * 2)
		procMoveWindow.Call(uintptr(g.lblTitle), uintptr(margin), 20,
			uintptr(newWidth), 30, 1)
	}
	if g.lblStatus != 0 {
		newWidth := width - (margin * 2)
		procMoveWindow.Call(uintptr(g.lblStatus), uintptr(margin), 60,
			uintptr(newWidth), 25, 1)
	}
}

func (g *NetworkTesterGUI) onTimer() {
	if g == nil {
		return
	}

	// Get queued results
	g.resultsMu.Lock()
	results := g.results
	g.results = nil
	g.resultsMu.Unlock()

	// Add to listbox
	for _, r := range results {
		if g.listBox != 0 {
			text, _ := syscall.UTF16PtrFromString(r)
			procSendMessageW.Call(uintptr(g.listBox), LB_ADDSTRING, 0, uintptr(unsafe.Pointer(text)))
		}
	}

	// Auto-scroll
	if len(results) > 0 && g.listBox != 0 {
		count, _, _ := procSendMessageW.Call(uintptr(g.listBox), LB_GETCOUNT, 0, 0)
		if count > 0 {
			procSendMessageW.Call(uintptr(g.listBox), LB_SETTOPINDEX, count-1, 0)
		}
	}

	// Update progress
	if atomic.LoadInt32(&g.isRunning) == 1 {
		total := atomic.LoadInt32(&g.totalTests)
		completed := atomic.LoadInt32(&g.completedTests)
		success := atomic.LoadInt32(&g.successCount)
		fail := atomic.LoadInt32(&g.failCount)

		if total > 0 && g.progressBar != 0 {
			progress := int(float64(completed) / float64(total) * 100)
			procSendMessageW.Call(uintptr(g.progressBar), PBM_SETPOS, uintptr(progress), 0)
		}

		if g.lblStatus != 0 {
			status := fmt.Sprintf("Testing... %d/%d completed (%d OK, %d Failed)", completed, total, success, fail)
			text, _ := syscall.UTF16PtrFromString(status)
			procSetWindowTextW.Call(uintptr(g.lblStatus), uintptr(unsafe.Pointer(text)))
		}
	}

	// Check completion
	if atomic.CompareAndSwapInt32(&g.testsDone, 1, 2) {
		logInfo("Tests done, completing")
		g.onTestsComplete()
	}
}

func (g *NetworkTesterGUI) onTestsComplete() {
	logInfo("onTestsComplete starting")

	// Stop timer first
	procKillTimer.Call(uintptr(g.hwnd), TIMER_ID)
	logInfo("Timer killed")

	// Process remaining results
	g.resultsMu.Lock()
	results := g.results
	g.results = nil
	g.resultsMu.Unlock()

	logInfo(fmt.Sprintf("Processing %d remaining results", len(results)))

	for _, r := range results {
		if g.listBox != 0 {
			text, _ := syscall.UTF16PtrFromString(r)
			procSendMessageW.Call(uintptr(g.listBox), LB_ADDSTRING, 0, uintptr(unsafe.Pointer(text)))
		}
	}

	// Auto-scroll
	if g.listBox != 0 {
		count, _, _ := procSendMessageW.Call(uintptr(g.listBox), LB_GETCOUNT, 0, 0)
		if count > 0 {
			procSendMessageW.Call(uintptr(g.listBox), LB_SETTOPINDEX, count-1, 0)
		}
	}

	// Final status
	success := atomic.LoadInt32(&g.successCount)
	fail := atomic.LoadInt32(&g.failCount)
	total := atomic.LoadInt32(&g.totalTests)

	logInfo(fmt.Sprintf("Final: success=%d, fail=%d, total=%d", success, fail, total))

	if g.lblStatus != 0 {
		status := fmt.Sprintf("Complete! %d succeeded, %d failed out of %d tests", success, fail, total)
		text, _ := syscall.UTF16PtrFromString(status)
		procSetWindowTextW.Call(uintptr(g.lblStatus), uintptr(unsafe.Pointer(text)))
	}

	if g.progressBar != 0 {
		procSendMessageW.Call(uintptr(g.progressBar), PBM_SETPOS, 100, 0)
	}

	// Reset state before enabling button
	atomic.StoreInt32(&g.isRunning, 0)

	// Reset button using pre-allocated string
	if g.btnStart != 0 {
		g.currentBtnText = g.strRunAgain
		procEnableWindow.Call(uintptr(g.btnStart), 1)
		// Force redraw
		procSendMessageW.Call(uintptr(g.btnStart), 0x000F, 0, 0) // WM_PAINT via InvalidateRect
	}

	logInfo("onTestsComplete finished")
}

func (g *NetworkTesterGUI) startTests() {
	logInfo("startTests called")

	atomic.StoreInt32(&g.isRunning, 1)
	atomic.StoreInt32(&g.testsDone, 0)

	if g.btnStart != 0 {
		g.currentBtnText = g.strRunning
		procEnableWindow.Call(uintptr(g.btnStart), 0)
	}

	if g.listBox != 0 {
		procSendMessageW.Call(uintptr(g.listBox), LB_RESETCONTENT, 0, 0)
	}

	if g.progressBar != 0 {
		procSendMessageW.Call(uintptr(g.progressBar), PBM_SETPOS, 0, 0)
	}

	atomic.StoreInt32(&g.totalTests, 0)
	atomic.StoreInt32(&g.completedTests, 0)
	atomic.StoreInt32(&g.successCount, 0)
	atomic.StoreInt32(&g.failCount, 0)

	g.resultsMu.Lock()
	g.results = nil
	g.resultsMu.Unlock()

	if g.lblStatus != 0 {
		text, _ := syscall.UTF16PtrFromString("Starting tests...")
		procSetWindowTextW.Call(uintptr(g.lblStatus), uintptr(unsafe.Pointer(text)))
	}

	// Start timer
	procSetTimer.Call(uintptr(g.hwnd), TIMER_ID, TIMER_INTERVAL, 0)

	go g.runTestsAsync()
}

func (g *NetworkTesterGUI) queueResult(text string) {
	g.resultsMu.Lock()
	g.results = append(g.results, text)
	g.resultsMu.Unlock()
}

func (g *NetworkTesterGUI) runTestsAsync() {
	logInfo("runTestsAsync started")

	defer func() {
		if r := recover(); r != nil {
			logError(fmt.Sprintf("PANIC in runTestsAsync: %v", r))
			g.queueResult(fmt.Sprintf("Error: %v", r))
		}
		atomic.StoreInt32(&g.testsDone, 1)
		logInfo("runTestsAsync finished, testsDone=1")
	}()

	// Initial test
	baseConn := ConnectionInfo{
		Product:    "Patch My PC Network Tester",
		DomainName: "patchmypc.com",
		Port:       "443",
		Reason:     "Base Functionality",
	}

	success, err := g.testConnection(baseConn)
	if success {
		g.queueResult(fmt.Sprintf("[OK] %s:%s - %s (%s)", baseConn.DomainName, baseConn.Port, baseConn.Product, baseConn.Reason))
	} else {
		g.queueResult(fmt.Sprintf("[FAIL] %s:%s - %s (%s)", baseConn.DomainName, baseConn.Port, baseConn.Product, baseConn.Reason))
	}
	g.logTestResult(baseConn, success, err)

	if !success {
		g.queueResult("Cannot proceed - base connection failed")
		return
	}

	// Download list
	g.queueResult("Downloading domain list...")

	fileName, err := downloadFile.DownloadFile("https://patchmypc.com/scupcatalog/downloads/PatchMyPC-DomainList.csv")
	if err != nil {
		logError(fmt.Sprintf("Download failed: %v", err))
		g.queueResult(fmt.Sprintf("Failed to download: %v", err))
		return
	}

	records, err := g.readData(fileName)
	if err != nil {
		logError(fmt.Sprintf("CSV read failed: %v", err))
		g.queueResult(fmt.Sprintf("Failed to read CSV: %v", err))
		return
	}

	// Build list
	var connections []ConnectionInfo
	for _, rec := range records {
		if len(rec) < 6 {
			continue
		}
		c := ConnectionInfo{
			Product:    rec[1],
			DomainName: rec[2],
			Port:       rec[4],
			Reason:     rec[5],
		}
		if g.shouldConnect(c.DomainName) {
			connections = append(connections, c)
		}
	}

	total := int32(len(connections))
	atomic.StoreInt32(&g.totalTests, total)
	g.queueResult(fmt.Sprintf("Testing %d connections...", total))

	// Run tests
	sem := make(chan struct{}, MAX_CONCURRENT)
	var wg sync.WaitGroup

	for _, conn := range connections {
		wg.Add(1)
		sem <- struct{}{}

		go func(c ConnectionInfo) {
			defer func() {
				if r := recover(); r != nil {
					atomic.AddInt32(&g.completedTests, 1)
					atomic.AddInt32(&g.failCount, 1)
				}
				<-sem
				wg.Done()
			}()

			success, err := g.testConnection(c)

			atomic.AddInt32(&g.completedTests, 1)
			if success {
				atomic.AddInt32(&g.successCount, 1)
				g.queueResult(fmt.Sprintf("[OK] %s:%s - %s (%s)", c.DomainName, c.Port, c.Product, c.Reason))
			} else {
				atomic.AddInt32(&g.failCount, 1)
				g.queueResult(fmt.Sprintf("[FAIL] %s:%s - %s (%s)", c.DomainName, c.Port, c.Product, c.Reason))
			}
			g.logTestResult(c, success, err)
		}(conn)
	}

	wg.Wait()
	logInfo("All test goroutines completed")
}

func (g *NetworkTesterGUI) testConnection(conn ConnectionInfo) (bool, error) {
	c, err := net.DialTimeout("tcp", net.JoinHostPort(conn.DomainName, conn.Port), 5*time.Second)
	if err != nil {
		return false, err
	}
	c.Close()
	return true, nil
}

func (g *NetworkTesterGUI) logTestResult(conn ConnectionInfo, success bool, err error) {
	if success {
		goCMTrace.LogData(goCMTrace.LogEntry{
			File:    mainLogFile,
			Message: fmt.Sprintf("Success: %s to %s:%s", conn.Product, conn.DomainName, conn.Port),
			State:   1,
		})
	} else {
		msg := fmt.Sprintf("Failed: %s to %s:%s", conn.Product, conn.DomainName, conn.Port)
		if err != nil {
			msg = fmt.Sprintf("Failed: %s to %s:%s - %v", conn.Product, conn.DomainName, conn.Port, err)
		}
		goCMTrace.LogData(goCMTrace.LogEntry{
			File:    mainLogFile,
			Message: msg,
			State:   3,
		})
	}
}

func (g *NetworkTesterGUI) shouldConnect(host string) bool {
	return host != "localhost" && host != "patchmypc.com" && host != ""
}

func (g *NetworkTesterGUI) readData(fileName string) ([][]string, error) {
	f, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.Read() // skip header
	return r.ReadAll()
}
