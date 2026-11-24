package gitcli

import (
	"fmt"
	"syscall"
	"unsafe"
)

var captureGitSnapshot = jobKernel.NewProc("PssCaptureSnapshot")
var freeGitSnapshot = jobKernel.NewProc("PssFreeSnapshot")
var createGitWalk = jobKernel.NewProc("PssWalkMarkerCreate")
var freeGitWalk = jobKernel.NewProc("PssWalkMarkerFree")
var walkGitSnapshot = jobKernel.NewProc("PssWalkSnapshot")

// pssGitThread matches PSS_THREAD_ENTRY. FILETIME uses pairs of DWORDs, whose
// alignment is four bytes (including on amd64).
// https://learn.microsoft.com/windows/win32/api/processsnapshot/ns-processsnapshot-pss_thread_entry
type pssGitThread struct {
	ExitStatus                                 uint32
	TEB                                        uintptr
	ProcessID, ThreadID                        uint32
	Affinity                                   uintptr
	Priority, BasePriority                     int32
	LastArgument                               uintptr
	LastSyscall                                uint16
	CreateTime, ExitTime, KernelTime, UserTime [2]uint32
	StartAddress                               uintptr
	CaptureTime                                [2]uint32
	Flags                                      uint32
	SuspendCount, ContextSize                  uint16
	Context                                    uintptr
}

// Capture only this suspended process's thread IDs instead of enumerating all
// system threads on every Git command. PSS is available since Windows 8.1.
func resumeGitProcessSnapshot(pid uint32) error {
	for _, proc := range []*syscall.LazyProc{captureGitSnapshot, freeGitSnapshot, createGitWalk, freeGitWalk, walkGitSnapshot} {
		if err := proc.Find(); err != nil {
			return err
		}
	}
	process, err := syscall.OpenProcess(0x400|0x10, false, pid) // QUERY_INFORMATION | VM_READ
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(process)
	var snapshot uintptr
	code, _, _ := captureGitSnapshot.Call(uintptr(process), 0x80, 0, uintptr(unsafe.Pointer(&snapshot))) // PSS_CAPTURE_THREADS, IDs only
	if code != 0 {
		return fmt.Errorf("capture Git threads: %w", syscall.Errno(code))
	}
	current, err := syscall.GetCurrentProcess()
	if err != nil {
		return err
	}
	defer freeGitSnapshot.Call(uintptr(current), snapshot)
	var marker uintptr
	code, _, _ = createGitWalk.Call(0, uintptr(unsafe.Pointer(&marker)))
	if code != 0 {
		return fmt.Errorf("create Git thread walk: %w", syscall.Errno(code))
	}
	defer freeGitWalk.Call(marker)
	entry := pssGitThread{}
	code, _, _ = walkGitSnapshot.Call(snapshot, 3, marker, uintptr(unsafe.Pointer(&entry)), unsafe.Sizeof(entry)) // PSS_WALK_THREADS
	if code != 0 {
		return fmt.Errorf("read Git primary thread: %w", syscall.Errno(code))
	}
	if entry.ProcessID != pid || entry.ThreadID == 0 {
		return fmt.Errorf("Git thread snapshot has another process identity")
	}
	thread, _, callErr := openGitThread.Call(0x2, 0, uintptr(entry.ThreadID))
	if thread == 0 {
		return callErr
	}
	defer syscall.CloseHandle(syscall.Handle(thread))
	count, _, callErr := resumeGitThread.Call(thread)
	if uint32(count) == 0xffffffff {
		return callErr
	}
	return nil
}
