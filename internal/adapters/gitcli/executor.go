package gitcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type Executor struct {
	write sync.Mutex
}

func New() *Executor { return &Executor{} }

type result struct {
	stdout, stderr string
	exitCode       int
}

func (g *Executor) run(ctx context.Context, directory string, args ...string) (result, error) {
	return g.runInput(ctx, directory, "", args...)
}

func (*Executor) runInput(ctx context.Context, directory, input string, args ...string) (result, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = directory
	cmd.Stdin = strings.NewReader(input)
	cmd.WaitDelay = 2 * time.Second
	cmd.Env = append(repositoryEnvironment(), "GIT_OPTIONAL_LOCKS=0", "GIT_NO_LAZY_FETCH=1")
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
	if errors.Is(err, exec.ErrWaitDelay) && output.exitCode == 0 {
		return output, nil
	}
	if err != nil {
		commandArgs := args
		if commandArgs[0] == "-c" {
			commandArgs = commandArgs[2:]
		}
		if commandArgs[0] == "--literal-pathspecs" {
			commandArgs = commandArgs[1:]
		}
		return output, fmt.Errorf("git %s: %w", commandArgs[0], err)
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
			"GIT_CONFIG", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS", "GIT_TEMPLATE_DIR",
			"GIT_LITERAL_PATHSPECS", "GIT_GLOB_PATHSPECS", "GIT_NOGLOB_PATHSPECS", "GIT_ICASE_PATHSPECS":
			continue
		}
		clean = append(clean, entry)
	}
	return clean
}
