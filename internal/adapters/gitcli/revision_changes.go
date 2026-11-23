package gitcli

import (
	"context"
	"errors"
	"strings"
	"time"

	"vela/internal/domain"
)

// ReadRevisionChanges explicitly compares a merge against the selected parent.
func (g *Executor) ReadRevisionChanges(ctx context.Context, root, revision string, mainline int) ([]domain.File, error) {
	ctx, cancel := context.WithTimeout(canonicalHistory(ctx), 30*time.Second)
	defer cancel()
	if err := g.fixedCommit(ctx, root, revision); err != nil {
		return nil, err
	}
	line, err := g.read(ctx, root, "rev-list", "--parents", "--max-count=1", revision, "--")
	if err != nil {
		return nil, err
	}
	parents := strings.Fields(line)
	if len(parents) == 0 {
		return nil, errors.New("commit is unavailable")
	}
	parent := 1
	if len(parents) > 2 {
		if mainline < 1 || mainline >= len(parents) {
			return nil, errors.New("choose a merge mainline parent")
		}
		parent = mainline
	} else if mainline != 0 {
		return nil, errors.New("mainline is only supported for merges")
	}
	args := []string{"diff-tree", "--root", "--no-commit-id", "--name-status", "-r", "-z", "--find-renames=50%", "--no-ext-diff", "--no-textconv"}
	if len(parents) > 1 {
		args = append(args, parents[parent])
	}
	args = append(args, revision, "--")
	out, err := g.runInputWithEnvLimit(ctx, root, "", nil, 2<<20, args...)
	if err != nil {
		return nil, err
	}
	return parseComparisonChanges(out.stdout)
}

func parseComparisonChanges(output string) ([]domain.File, error) {
	fields := nulFields(output)
	var result []domain.File
	for i := 0; i < len(fields); {
		status := fields[i]
		i++
		if status == "" || i >= len(fields) {
			return nil, errors.New("invalid historical changed file")
		}
		file := domain.File{Status: status[:1], Path: fields[i]}
		i++
		if file.Status == "R" || file.Status == "C" {
			if i >= len(fields) {
				return nil, errors.New("invalid historical rename")
			}
			file.OldPath, file.Path = file.Path, fields[i]
			i++
		}
		if !validGitPath(file.Path) || file.OldPath != "" && !validGitPath(file.OldPath) {
			return nil, errors.New("historical changes contain unsupported paths")
		}
		result = append(result, file)
		if len(result) > 10000 {
			return nil, errors.New("historical comparison exceeds 10000 changed files")
		}
	}
	return result, nil
}
