package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"

	"vela/internal/application"
	"vela/internal/domain"
)

// ReplaceProject preflights the full preview before writing. Each file then
// reuses SaveFile's atomic replacement and stale disk checks under one lock.
// Cross-file atomicity is deliberately not promised: durable progress survives
// cancellation/errors and is returned to the caller.
func (g *Executor) ReplaceProject(ctx context.Context, repo domain.Repository, input application.ProjectReplaceInput) (application.ProjectReplaceResult, error) {
	var out application.ProjectReplaceResult
	if len(input.Matches) == 0 || len(input.Matches) > application.MaxProjectSearchMatches {
		return out, errors.New("preview 1 to 1,000 matches before replacing")
	}
	g.write.Lock()
	defer g.write.Unlock()
	root, err := os.OpenRoot(repo.Root)
	if err != nil {
		return out, err
	}
	defer root.Close()
	groups := map[string][]application.ProjectSearchMatch{}
	for _, m := range input.Matches {
		if m.InMemory || m.RawVersion == "" {
			return out, errors.New("save matching unsaved documents and run Find again")
		}
		if err := projectPath(m.Path, false); err != nil {
			return out, err
		}
		groups[m.Path] = append(groups[m.Path], m)
	}
	paths := make([]string, 0, len(groups))
	for name := range groups {
		paths = append(paths, name)
	}
	slices.Sort(paths)
	type preparedFile struct {
		file    domain.File
		next    string
		matches int
	}
	prepared := make([]preparedFile, 0, len(paths))
	total := 0
	for _, name := range paths {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		if err := ordinaryProjectPath(root, name, false); err != nil {
			return out, fmt.Errorf("preflight %s: %w", name, err)
		}
		info, err := root.Lstat(name)
		if err != nil {
			return out, fmt.Errorf("inspect %s: %w", name, err)
		}
		if info.Mode().Perm()&0200 == 0 {
			return out, fmt.Errorf("%s is read-only", name)
		}
		file, err := g.ReadFile(ctx, repo.Root, "project", domain.File{Path: name})
		if err != nil {
			return out, fmt.Errorf("read %s: %w", name, err)
		}
		if file.DiskVersion == "" {
			return out, fmt.Errorf("%s is not an editable regular file", name)
		}
		matches := groups[name]
		slices.SortFunc(matches, func(a, b application.ProjectSearchMatch) int { return a.Start - b.Start })
		next, err := application.ReplaceProjectText(file.New, input, matches)
		if err != nil {
			return out, fmt.Errorf("preflight %s: %w", name, err)
		}
		total += len(file.New) + len(next)
		if total > application.MaxProjectSearchBytes*2 {
			return out, errors.New("replacement batch exceeds 128 MiB combined input/output")
		}
		prepared = append(prepared, preparedFile{file: file, next: next, matches: len(matches)})
	}
	for _, item := range prepared {
		if _, err := g.saveFile(ctx, repo, item.file, item.next); err != nil {
			return out, fmt.Errorf("replace %s stopped after %d file(s); completed files are kept: %w", item.file.Path, len(out.Paths), err)
		}
		out.Paths = append(out.Paths, item.file.Path)
		out.Replacements += item.matches
	}
	return out, nil
}
