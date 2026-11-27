package gitcli

import (
	"context"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestProcessSnapshotResumesSuspendedGit(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) == 8 && unsafe.Sizeof(pssGitThread{}) != 120 || unsafe.Sizeof(uintptr(0)) == 4 && unsafe.Sizeof(pssGitThread{}) != 92 {
		t.Fatal("PSS_THREAD_ENTRY ABI size mismatch")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "--version")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x4, HideWindow: true}
	var output strings.Builder
	cmd.Stdout = &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := resumeGitProcessSnapshot(uint32(cmd.Process.Pid)); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil || !strings.HasPrefix(output.String(), "git version ") {
		t.Fatalf("snapshot did not resume owned Git: %q %v", output.String(), err)
	}
}
