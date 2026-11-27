package gitcli

import (
	"context"
	"os/exec"
	"testing"
)

func BenchmarkGitProcessStartup(b *testing.B) {
	for _, managed := range []bool{false, true} {
		name := "direct"
		if managed {
			name = "job"
		}
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				cmd := exec.CommandContext(context.Background(), "git", "--version")
				var err error
				if managed {
					err = runGitCommand(context.Background(), cmd)
				} else {
					err = cmd.Run()
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
