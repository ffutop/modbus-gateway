package filepicker

import (
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	comdlg32             = syscall.NewLazyDLL("comdlg32.dll")
	getOpenFileName      = comdlg32.NewProc("GetOpenFileNameW")
	getSaveFileName      = comdlg32.NewProc("GetSaveFileNameW")
	commDlgExtendedError = comdlg32.NewProc("CommDlgExtendedError")
	ole32                = syscall.NewLazyDLL("ole32.dll")
	coInitializeEx       = ole32.NewProc("CoInitializeEx")
	coUninitialize       = ole32.NewProc("CoUninitialize")
)

// openFileName is OPENFILENAMEW; Go lays it out as C does on amd64 and arm64.
type openFileName struct {
	structSize    uint32
	owner         uintptr
	instance      uintptr
	filter        *uint16
	customFilter  *uint16
	maxCustFilter uint32
	filterIndex   uint32
	file          *uint16
	maxFile       uint32
	fileTitle     *uint16
	maxFileTitle  uint32
	initialDir    *uint16
	title         *uint16
	flags         uint32
	fileOffset    uint16
	fileExtension uint16
	defExt        *uint16
	custData      uintptr
	hook          uintptr
	templateName  *uint16
	reserved      uintptr
	reservedDword uint32
	flagsEx       uint32
}

const (
	ofnOverwritePrompt = 0x00000002
	ofnNoChangeDir     = 0x00000008 // the app works in the configuration's directory
	ofnPathMustExist   = 0x00000800
	ofnFileMustExist   = 0x00001000
	ofnExplorer        = 0x00080000
	maxPath            = 32768

	coinitApartmentThreaded = 0x2
	coinitDisableOLE1DDE    = 0x4
)

func pick(r Request) (string, error) {
	// The Explorer-style dialog is a COM object that wants a single-threaded
	// apartment on a thread of its own.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if hr, _, _ := coInitializeEx.Call(0, coinitApartmentThreaded|coinitDisableOLE1DDE); int32(hr) >= 0 {
		defer coUninitialize.Call()
	}

	buf := make([]uint16, maxPath)
	if r.Save {
		copy(buf[:maxPath-1], utf16.Encode([]rune(r.Name)))
	}
	ofn := openFileName{
		owner:   ownerHandle(),
		filter:  &filter("YAML 配置 ("+strings.Join(Patterns, ";")+")", strings.Join(Patterns, ";"), "所有文件 (*.*)", "*.*")[0],
		file:    &buf[0],
		maxFile: maxPath,
		title:   utf16Ptr(r.Title),
		defExt:  utf16Ptr("yaml"),
		flags:   ofnExplorer | ofnNoChangeDir | ofnPathMustExist,
	}
	ofn.structSize = uint32(unsafe.Sizeof(ofn))
	if r.Dir != "" {
		ofn.initialDir = utf16Ptr(r.Dir)
	}
	proc := getOpenFileName
	if r.Save {
		proc = getSaveFileName
		ofn.flags |= ofnOverwritePrompt
	} else {
		ofn.flags |= ofnFileMustExist
	}
	if ok, _, _ := proc.Call(uintptr(unsafe.Pointer(&ofn))); ok == 0 {
		if code, _, _ := commDlgExtendedError.Call(); code != 0 {
			return "", fmt.Errorf("文件对话框失败（0x%x）", code)
		}
		return "", nil // cancelled
	}
	return syscall.UTF16ToString(buf), nil
}

// filter encodes display/pattern pairs as NUL-separated, double-NUL-ended
// UTF-16, the format lpstrFilter takes.
func filter(pairs ...string) []uint16 {
	var out []uint16
	for _, s := range pairs {
		out = append(out, utf16.Encode([]rune(s))...)
		out = append(out, 0)
	}
	return append(out, 0)
}

func utf16Ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}
