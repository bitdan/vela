//go:build !windows

package gitcli

import (
	"context"
	"os/exec"
)

func runGitCommand(_ context.Context, cmd *exec.Cmd) error { return cmd.Run() }
