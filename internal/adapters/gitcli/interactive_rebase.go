package gitcli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"vela/internal/application"
	"vela/internal/domain"
)

const (
	rebasePlanEnvironment  = "VELA_INTERNAL_REBASE_PLAN"
	rebaseTokenEnvironment = "VELA_INTERNAL_REBASE_TOKEN"
	rebasePlanFile         = "vela-interactive-rebase.json"
)

type interactiveRebasePlan struct {
	Token    string            `json:"token"`
	Initial  []string          `json:"initial"`
	Final    []rebaseTodoEntry `json:"final"`
	Todo     string            `json:"todo"`
	Reword   map[string]string `json:"reword,omitempty"`
	Squash   map[string]bool   `json:"squash,omitempty"`
	Original map[string]string `json:"original"`
}

type rebaseTodoEntry struct {
	Action string `json:"action"`
	Commit string `json:"commit"`
}

func (g *Executor) interactiveRebase(ctx context.Context, repo domain.Repository, op domain.RepositoryOperation) error {
	ctx = canonicalHistory(ctx)
	if !op.ConfirmRewrite {
		return errors.New("confirm the interactive Rebase sequence and rewritten descendants")
	}
	preview, err := g.commitRewritePreview(ctx, repo, domain.InteractiveRebase, operationRebaseSelection(op.RebaseSteps))
	if err != nil {
		return err
	}
	if op.RebaseVersion == "" || op.RebaseVersion != preview.Version {
		return errors.New("interactive Rebase preview changed; preview again")
	}
	if err := application.ValidateInteractiveRebasePlan(preview, op.RebaseSteps); err != nil {
		return err
	}
	latest, err := g.commitRewritePreview(ctx, repo, domain.InteractiveRebase, operationRebaseSelection(op.RebaseSteps))
	if err != nil {
		return err
	}
	if latest.Version != preview.Version || !slices.Equal(latest.Selected, preview.Selected) || !slices.Equal(latest.Replay, preview.Replay) {
		return errors.New("interactive Rebase range changed before Git started")
	}
	temporary, err := os.MkdirTemp("", "vela-interactive-rebase-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	plan, err := prepareInteractiveRebasePlan(preview, op.RebaseSteps)
	if err != nil {
		return err
	}
	environment, err := interactiveRebaseEnvironment(temporary, plan)
	if err != nil {
		return err
	}
	onto := preview.Parent
	if onto == "" {
		onto, err = g.read(ctx, repo.Root, "hash-object", "-t", "tree", "--stdin")
		if err != nil {
			return err
		}
		onto = strings.TrimSpace(onto)
	}
	if err := g.protectWorktreePaths(ctx, repo.Root, repo.Head, onto); err != nil {
		return err
	}
	application.ReportTaskProgress(ctx, "Running interactive Rebase")
	args := []string{"--interactive", "--merge", "--no-autostash", "--no-autosquash", "--no-rebase-merges"}
	if preview.Parent == "" {
		args = append(args, "--root")
	} else {
		args = append(args, "--onto", preview.Parent, preview.Parent)
	}
	out, err := g.runInputWithEnv(ctx, repo.Root, "", environment, append([]string{"-c", "submodule.recurse=false", "rebase"}, args...)...)
	return operationError(ctx, out, err)
}

func operationRebaseSelection(steps []domain.InteractiveRebaseStep) []string {
	selection := make([]string, len(steps))
	for index, step := range steps {
		selection[index] = step.Commit
	}
	return selection
}

func prepareInteractiveRebasePlan(preview domain.CommitRewritePreview, steps []domain.InteractiveRebaseStep) (interactiveRebasePlan, error) {
	plan := interactiveRebasePlan{Reword: make(map[string]string), Squash: make(map[string]bool), Original: make(map[string]string)}
	subjects := make(map[string]string, len(preview.RebaseCommits))
	for _, commit := range preview.RebaseCommits {
		plan.Original[commit.Commit] = commit.Message
		subjects[commit.Commit], _, _ = strings.Cut(commit.Message, "\n")
	}
	for _, commit := range preview.Replay {
		plan.Original[commit] = preview.RebaseMessages[commit]
	}
	for index := len(preview.Selected) - 1; index >= 0; index-- {
		plan.Initial = append(plan.Initial, preview.Selected[index])
	}
	for index := len(preview.Replay) - 1; index >= 0; index-- {
		plan.Initial = append(plan.Initial, preview.Replay[index])
	}
	var todo strings.Builder
	for _, step := range steps {
		action := map[domain.InteractiveRebaseAction]string{
			domain.RebasePick: "pick", domain.RebaseReword: "reword", domain.RebaseSquash: "squash", domain.RebaseFixup: "fixup", domain.RebaseDrop: "drop",
		}[step.Action]
		if action == "" {
			return interactiveRebasePlan{}, errors.New("unsupported interactive Rebase action")
		}
		id, err := rebaseTodoID(step.Commit, plan.Initial)
		if err != nil {
			return interactiveRebasePlan{}, err
		}
		fmt.Fprintf(&todo, "%s %s %s\n", action, id, subjects[step.Commit])
		if step.Action == domain.RebaseReword {
			plan.Reword[step.Commit] = step.Message
		}
		if step.Action == domain.RebaseSquash {
			plan.Squash[step.Commit] = true
		}
	}
	for index := len(preview.Replay) - 1; index >= 0; index-- {
		id, err := rebaseTodoID(preview.Replay[index], plan.Initial)
		if err != nil {
			return interactiveRebasePlan{}, err
		}
		fmt.Fprintf(&todo, "pick %s\n", id)
	}
	plan.Todo = todo.String()
	plan.Final = expectedRebaseTodo(steps, preview.Replay)
	parsed, err := parseRebaseTodo(plan.Todo, plan.Initial)
	if err != nil || !slices.Equal(parsed, plan.Final) {
		return interactiveRebasePlan{}, errors.New("confirmed interactive Rebase todo failed verification")
	}
	return plan, nil
}

func rebaseTodoID(commit string, allowed []string) (string, error) {
	for length := 8; length <= len(commit); length++ {
		unique := true
		for _, other := range allowed {
			if other != commit && strings.HasPrefix(other, commit[:length]) {
				unique = false
				break
			}
		}
		if unique {
			return commit[:length], nil
		}
	}
	return "", errors.New("cannot uniquely identify an interactive Rebase commit")
}

func expectedRebaseTodo(steps []domain.InteractiveRebaseStep, replay []string) []rebaseTodoEntry {
	var expected []rebaseTodoEntry
	for _, step := range steps {
		action := map[domain.InteractiveRebaseAction]string{
			domain.RebasePick: "pick", domain.RebaseReword: "reword", domain.RebaseSquash: "squash", domain.RebaseFixup: "fixup", domain.RebaseDrop: "drop",
		}[step.Action]
		expected = append(expected, rebaseTodoEntry{Action: action, Commit: step.Commit})
	}
	for index := len(replay) - 1; index >= 0; index-- {
		expected = append(expected, rebaseTodoEntry{Action: "pick", Commit: replay[index]})
	}
	return expected
}

func parseRebaseTodo(todo string, allowed []string) ([]rebaseTodoEntry, error) {
	var entries []rebaseTodoEntry
	for _, line := range strings.Split(todo, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || (fields[0] != "pick" && fields[0] != "p" && fields[0] != "reword" && fields[0] != "r" &&
			fields[0] != "squash" && fields[0] != "s" && fields[0] != "fixup" && fields[0] != "f" && fields[0] != "drop" && fields[0] != "d") {
			return nil, errors.New("Git supplied an unsupported Rebase todo command")
		}
		matched := ""
		for _, commit := range allowed {
			if strings.HasPrefix(commit, fields[1]) {
				if matched != "" {
					return nil, errors.New("Git supplied an ambiguous commit ID")
				}
				matched = commit
			}
		}
		if matched == "" {
			return nil, errors.New("Git supplied a commit outside the confirmed range")
		}
		action := map[string]string{"p": "pick", "r": "reword", "s": "squash", "f": "fixup", "d": "drop"}[fields[0]]
		if action == "" {
			action = fields[0]
		}
		entries = append(entries, rebaseTodoEntry{Action: action, Commit: matched})
	}
	return entries, nil
}

func interactiveRebaseEnvironment(directory string, plan interactiveRebasePlan) ([]string, error) {
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return nil, err
	}
	plan.Token = hex.EncodeToString(token)
	data, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(directory, "confirmed-plan.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return nil, err
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return []string{
		rebasePlanEnvironment + "=" + path,
		rebaseTokenEnvironment + "=" + plan.Token,
		"GIT_SEQUENCE_EDITOR=" + quoteGitEditor(executable) + " internal-rebase-editor sequence",
		"GIT_EDITOR=" + quoteGitEditor(executable) + " internal-rebase-editor message",
	}, nil
}

func quoteGitEditor(executable string) string {
	return "'" + strings.ReplaceAll(executable, "'", "'\\''") + "'"
}

func RunInteractiveRebaseEditor(args []string) error {
	if len(args) != 2 || (args[0] != "sequence" && args[0] != "message" && args[0] != "recovery-message") {
		return errors.New("invalid internal Rebase editor invocation")
	}
	data, err := os.ReadFile(os.Getenv(rebasePlanEnvironment))
	if err != nil {
		return err
	}
	var plan interactiveRebasePlan
	if err := json.Unmarshal(data, &plan); err != nil || plan.Token == "" || plan.Token != os.Getenv(rebaseTokenEnvironment) {
		return errors.New("invalid confirmed Rebase plan")
	}
	target, err := filepath.Abs(args[1])
	if err != nil {
		return err
	}
	if args[0] == "recovery-message" {
		return runRebaseRecoveryMessage(plan, target)
	}
	if args[0] == "message" {
		if filepath.Base(target) != "COMMIT_EDITMSG" {
			return errors.New("Git requested an unexpected Rebase message file")
		}
		index, err := stoppedRebaseStep(plan, filepath.Dir(target))
		if err != nil {
			return err
		}
		entry := plan.Final[index]
		if message, found := plan.Reword[entry.Commit]; found && entry.Action == "reword" {
			original, readErr := os.ReadFile(target)
			if readErr != nil {
				return readErr
			}
			if !strings.HasPrefix(string(original), plan.Original[entry.Commit]) {
				return errors.New("Git Reword message does not match the confirmed original commit")
			}
			return os.WriteFile(target, []byte(message), 0600)
		}
		if plan.Squash[entry.Commit] && entry.Action == "squash" {
			return nil
		}
		return errors.New("Git requested a message outside the confirmed Rebase plan")
	}
	initial, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	actual, err := parseRebaseTodo(string(initial), plan.Initial)
	expectedInitial := make([]rebaseTodoEntry, len(plan.Initial))
	for index, commit := range plan.Initial {
		expectedInitial[index] = rebaseTodoEntry{Action: "pick", Commit: commit}
	}
	if err != nil || !slices.Equal(actual, expectedInitial) {
		return errors.New("Git initial Rebase todo does not match the confirmed commit range")
	}
	confirmed, err := parseRebaseTodo(plan.Todo, plan.Initial)
	if err != nil || !slices.Equal(confirmed, plan.Final) {
		return errors.New("confirmed Rebase todo is invalid")
	}
	metadata, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(target), rebasePlanFile), metadata, 0600); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(plan.Todo), 0600)
}

func runRebaseRecoveryMessage(plan interactiveRebasePlan, target string) error {
	if filepath.Base(target) != "COMMIT_EDITMSG" {
		return errors.New("Git requested an unexpected Rebase recovery message file")
	}
	// Git moves a step into "done" before running it, so the stopped step is
	// the last done entry, never the head of the remaining todo.
	index, err := stoppedRebaseStep(plan, filepath.Dir(target))
	if err != nil {
		return err
	}
	entry := plan.Final[index]
	commit := entry.Commit
	switch entry.Action {
	case "pick":
		return os.WriteFile(target, []byte(plan.Original[commit]), 0600)
	case "reword":
		message, found := plan.Reword[commit]
		if !found {
			return errors.New("stopped Reword commit has no confirmed replacement")
		}
		return os.WriteFile(target, []byte(message), 0600)
	case "squash":
		message := interactiveRebaseMessageBefore(plan, index)
		if message == "" {
			return errors.New("Squash has no preceding confirmed commit message")
		}
		message = strings.TrimSuffix(message, "\n") + "\n\n" + plan.Original[commit]
		return os.WriteFile(target, []byte(message), 0600)
	default:
		return errors.New("stopped Rebase action does not accept a message")
	}
}

// stoppedRebaseStep returns the index in the confirmed plan of the step Git is
// currently running, verifying that Git's done list is a prefix of the plan.
func stoppedRebaseStep(plan interactiveRebasePlan, gitDirectory string) (int, error) {
	done, err := os.ReadFile(filepath.Join(gitDirectory, "rebase-merge", "done"))
	if err != nil {
		return 0, err
	}
	completed, err := parseRebaseTodo(string(done), plan.Initial)
	if err != nil {
		return 0, err
	}
	if len(completed) == 0 || len(completed) > len(plan.Final) || !slices.Equal(completed, plan.Final[:len(completed)]) {
		return 0, errors.New("Git Rebase done steps do not match the confirmed plan")
	}
	return len(completed) - 1, nil
}

func interactiveRebaseMessageBefore(plan interactiveRebasePlan, end int) string {
	message := ""
	for index := 0; index < end; index++ {
		entry := plan.Final[index]
		switch entry.Action {
		case "pick":
			message = plan.Original[entry.Commit]
		case "reword":
			message = plan.Reword[entry.Commit]
		case "squash":
			if message != "" {
				message = strings.TrimSuffix(message, "\n") + "\n\n" + plan.Original[entry.Commit]
			}
		}
	}
	return message
}

func (g *Executor) runInteractiveRebaseRecovery(ctx context.Context, root, planPath string) error {
	data, err := os.ReadFile(planPath)
	if err != nil {
		return err
	}
	var plan interactiveRebasePlan
	if err := json.Unmarshal(data, &plan); err != nil || plan.Token == "" {
		return errors.New("invalid native interactive Rebase recovery plan")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	environment := []string{
		rebasePlanEnvironment + "=" + planPath,
		rebaseTokenEnvironment + "=" + plan.Token,
		"GIT_SEQUENCE_EDITOR=:",
		"GIT_EDITOR=" + quoteGitEditor(executable) + " internal-rebase-editor recovery-message",
	}
	out, err := g.runInputWithEnv(ctx, root, "", environment, "-c", "submodule.recurse=false", "rebase", "--continue")
	return operationError(ctx, out, err)
}
