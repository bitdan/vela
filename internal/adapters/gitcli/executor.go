package gitcli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

type Executor struct {
	initialize sync.Mutex
}

func New() *Executor { return &Executor{} }

type result struct {
	stdout, stderr string
	exitCode       int
}

func (*Executor) run(ctx context.Context, directory string, args ...string) (result, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = directory
	cmd.Env = repositoryEnvironment()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	output := result{stdout: stdout.String(), stderr: stderr.String(), exitCode: -1}
	if cmd.ProcessState != nil {
		output.exitCode = cmd.ProcessState.ExitCode()
	}
	if ctx.Err() != nil {
		return output, ctx.Err()
	}
	if err != nil {
		return output, fmt.Errorf("git %s: %w", args[0], err)
	}
	return output, nil
}

func repositoryEnvironment() []string {
	env := os.Environ()
	clean := env[:0]
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY",
			"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_NAMESPACE", "GIT_CEILING_DIRECTORIES",
			"GIT_CONFIG", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS", "GIT_TEMPLATE_DIR":
			continue
		}
		clean = append(clean, entry)
	}
	return clean
}
