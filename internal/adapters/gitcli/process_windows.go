package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
	"unsafe"
)

var jobKernel = syscall.NewLazyDLL("kernel32.dll")
var createGitJob = jobKernel.NewProc("CreateJobObjectW")
var assignGitJob = jobKernel.NewProc("AssignProcessToJobObject")
var terminateGitJob = jobKernel.NewProc("TerminateJobObject")
var queryGitJob = jobKernel.NewProc("QueryInformationJobObject")
var threadFirst = jobKernel.NewProc("Thread32First")
var threadNext = jobKernel.NewProc("Thread32Next")
var openGitThread = jobKernel.NewProc("OpenThread")
var resumeGitThread = jobKernel.NewProc("ResumeThread")

// Start suspended: assigning an already-running Git can miss helpers spawned
// before enrollment. No PID-based tree killing or process-name enumeration is
// used. Normal completion releases the job without terminating legitimate
// long-lived credential helpers; only cancellation terminates the job tree.
func runGitCommand(ctx context.Context, cmd *exec.Cmd) error {
	job, _, err := createGitJob.Call(0, 0)
	if job == 0 {
		return fmt.Errorf("create Git job: %w", err)
	}
	defer syscall.CloseHandle(syscall.Handle(job))
	attr := syscall.SysProcAttr{}
	if cmd.SysProcAttr != nil {
		attr = *cmd.SysProcAttr
	}
	attr.CreationFlags |= 0x4 // CREATE_SUSPENDED
	attr.HideWindow = true
	cmd.SysProcAttr = &attr
	cmd.Cancel = func() error {
		ok, _, err := terminateGitJob.Call(job, 1)
		if ok == 0 {
			return fmt.Errorf("%w: terminate job: %v", errProcessCleanup, err)
		}
		return nil
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	attachErr := attachGitProcess(job, uint32(cmd.Process.Pid))
	if attachErr == nil && ctx.Err() == nil {
		attachErr = resumeGitProcess(uint32(cmd.Process.Pid))
	}
	if attachErr != nil || ctx.Err() != nil {
		// A setup failure cannot leave an unassigned suspended process behind.
		_ = cmd.Cancel()
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil || attachErr != nil {
		// Cancellation can race assignment; repeat after enrollment and wait
		// independently of the canceled request until helper handles are gone.
		stopErr := cmd.Cancel()
		return errors.Join(attachErr, waitErr, stopErr, waitGitJob(job))
	}
	return waitErr
}

func attachGitProcess(job uintptr, pid uint32) error {
	h, err := syscall.OpenProcess(0x100|0x1, false, pid) // SET_QUOTA | TERMINATE
	if err != nil {
		return fmt.Errorf("open suspended Git: %w", err)
	}
	defer syscall.CloseHandle(h)
	ok, _, err := assignGitJob.Call(job, uintptr(h))
	if ok == 0 {
		return fmt.Errorf("assign suspended Git job: %w", err)
	}
	return nil
}

func resumeGitProcess(pid uint32) error {
	snapshot, err := syscall.CreateToolhelp32Snapshot(0x4, 0) // TH32CS_SNAPTHREAD
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(snapshot)
	entry := struct {
		Size, Usage, ID, Owner uint32
		Base, Delta            int32
		Flags                  uint32
	}{}
	entry.Size = uint32(unsafe.Sizeof(entry))
	ok, _, callErr := threadFirst.Call(uintptr(snapshot), uintptr(unsafe.Pointer(&entry)))
	for ok != 0 {
		if entry.Owner == pid {
			h, _, err := openGitThread.Call(0x2, 0, uintptr(entry.ID)) // SUSPEND_RESUME
			if h == 0 {
				return err
			}
			defer syscall.CloseHandle(syscall.Handle(h))
			count, _, err := resumeGitThread.Call(h)
			if uint32(count) == 0xffffffff {
				return err
			}
			return nil
		}
		entry.Size = uint32(unsafe.Sizeof(entry))
		ok, _, callErr = threadNext.Call(uintptr(snapshot), uintptr(unsafe.Pointer(&entry)))
	}
	return fmt.Errorf("suspended Git primary thread unavailable: %v", callErr)
}

func waitGitJob(job uintptr) error {
	limit := time.Now().Add(5 * time.Second)
	for {
		info := struct {
			Times                             [4]int64
			Faults, Total, Active, Terminated uint32
		}{}
		ok, _, err := queryGitJob.Call(job, 1, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info), 0)
		if ok == 0 {
			return fmt.Errorf("%w: query job: %v", errProcessCleanup, err)
		}
		if info.Active == 0 {
			return nil
		}
		if time.Now().After(limit) {
			return fmt.Errorf("%w: %d processes still active", errProcessCleanup, info.Active)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
