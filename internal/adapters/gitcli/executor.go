package gitcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"vela/internal/domain"
)

type Executor struct {
	write sync.Mutex
}

func New() *Executor { return &Executor{} }

type result struct {
	stdout, stderr string
	exitCode       int
}

type canonicalHistoryKey struct{}

type networkPolicyKey struct{}

func withNetworkPolicy(ctx context.Context, policy domain.NetworkPolicy) context.Context {
	return context.WithValue(ctx, networkPolicyKey{}, policy)
}

func canonicalHistory(ctx context.Context) context.Context {
	return context.WithValue(ctx, canonicalHistoryKey{}, true)
}

func (g *Executor) run(ctx context.Context, directory string, args ...string) (result, error) {
	return g.runInput(ctx, directory, "", args...)
}

func (g *Executor) runInput(ctx context.Context, directory, input string, args ...string) (result, error) {
	return g.runInputWithEnv(ctx, directory, input, nil, args...)
}

func (g *Executor) runInputWithEnv(ctx context.Context, directory, input string, env []string, args ...string) (result, error) {
	return g.runInputWithEnvLimit(ctx, directory, input, env, 0, args...)
}

var errOutputLimit = errors.New("Git output exceeded the requested limit")

type boundedOutput struct {
	buffer   *bytes.Buffer
	limit    int
	exceeded bool
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	left := b.limit - b.buffer.Len()
	if len(p) > left {
		b.exceeded = true
		n, _ := b.buffer.Write(p[:left])
		return n, errOutputLimit
	}
	return b.buffer.Write(p)
}

func (*Executor) runInputWithEnvLimit(ctx context.Context, directory, input string, env []string, stdoutLimit int, args ...string) (result, error) {
	policy, _ := ctx.Value(networkPolicyKey{}).(domain.NetworkPolicy)
	if policy.DisableCredentialHelper {
		args = append([]string{"-c", "credential.helper="}, args...)
	}
	if policy.NonInteractive {
		args = append([]string{"-c", "core.askPass="}, args...)
		env = append(env, "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=Never", "GIT_ASKPASS=", "SSH_ASKPASS=", "SSH_ASKPASS_REQUIRE=never")
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = directory
	cmd.Stdin = strings.NewReader(input)
	cmd.WaitDelay = 2 * time.Second
	cmd.Env = append(repositoryEnvironment(), "GIT_OPTIONAL_LOCKS=0", "GIT_NO_LAZY_FETCH=1")
	cmd.Env = append(cmd.Env, env...)
	if ctx.Value(canonicalHistoryKey{}) == true {
		cmd.Env = append(cmd.Env, "GIT_NO_REPLACE_OBJECTS=1")
	}
	var stdout, stderr bytes.Buffer
	var capture io.Writer = &stdout
	var bounded *boundedOutput
	if stdoutLimit > 0 {
		bounded = &boundedOutput{buffer: &stdout, limit: stdoutLimit}
		capture = bounded
	}
	cmd.Stdout, cmd.Stderr = capture, &stderr
	err := cmd.Run()
	output := result{stdout: stdout.String(), stderr: stderr.String(), exitCode: -1}
	if cmd.ProcessState != nil {
		output.exitCode = cmd.ProcessState.ExitCode()
	}
	if ctx.Err() != nil {
		return output, ctx.Err()
	}
	if bounded != nil && bounded.exceeded {
		return output, errOutputLimit
	}
	if errors.Is(err, exec.ErrWaitDelay) && output.exitCode == 0 {
		return output, nil
	}
	if err != nil {
		commandArgs := args
		for len(commandArgs) > 1 {
			if commandArgs[0] == "-c" && len(commandArgs) > 2 {
				commandArgs = commandArgs[2:]
			} else if commandArgs[0] == "--literal-pathspecs" {
				commandArgs = commandArgs[1:]
			} else {
				break
			}
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
