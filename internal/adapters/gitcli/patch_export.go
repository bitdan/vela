package gitcli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"vela/internal/domain"
)

const maxPatchExportBytes = 16 << 20

func (g *Executor) ReadChangesPatch(ctx context.Context, repo domain.Repository, source string, files []domain.File) (domain.PatchExport, error) {
	return g.readChangesPatch(ctx, repo, source, files, maxPatchExportBytes)
}

func (g *Executor) readChangesPatch(ctx context.Context, repo domain.Repository, source string, files []domain.File, limit int) (domain.PatchExport, error) {
	var doc domain.PatchExport
	if len(files) == 0 || len(files) > 100 || limit < 1 {
		return doc, errors.New("select between 1 and 100 changed files")
	}
	if source != "working" && source != "staged" && !validObjectID(source) {
		return doc, errors.New("select working, staged or fixed-commit changes")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, index, err := g.verifyCheckout(ctx, repo)
	if err != nil {
		return doc, err
	}
	if source == "working" || source == "staged" {
		if err = g.checkCommitState(ctx, repo.Root, index); err != nil {
			return doc, err
		}
		staged, working, err := g.readStatus(ctx, repo.Root)
		if err != nil {
			return doc, err
		}
		actual := filesByPath(working)
		if source == "staged" {
			actual = filesByPath(staged)
		}
		for _, file := range files {
			current, ok := actual[file.Path]
			if !ok || current.Status != file.Status || current.OldPath != file.OldPath || file.Status == "U" || file.Status == "?" {
				return doc, errors.New("selected changes are stale, conflicted or untracked; refresh or stage new files first")
			}
		}
	}
	seen := map[string]bool{}
	var paths []string
	pathBytes := 0
	for _, file := range files {
		for _, path := range []string{file.Path, file.OldPath} {
			if path == "" {
				continue
			}
			if !validGitPath(path) {
				return doc, errors.New("invalid patch file path")
			}
			if !seen[path] {
				seen[path] = true
				paths = append(paths, path)
				pathBytes += len(path)
			}
		}
		doc.Paths = append(doc.Paths, file.Path)
	}
	if pathBytes > 16384 {
		return doc, errors.New("patch selection paths exceed the command-size limit; select fewer files")
	}
	args := []string{"--literal-pathspecs", "-c", "diff.suppressBlankEmpty=false", "-c", "core.quotePath=true"}
	if source == "working" || source == "staged" {
		args = append(args, "diff")
		if source == "staged" {
			args = append(args, "--cached")
		}
	} else {
		args = append(args, "show", "--format=", "--patch", "--root", "--diff-merges=first-parent")
	}
	args = append(args, "--binary", "--full-index", "--no-ext-diff", "--no-textconv", "--no-color", "--no-relative", "--src-prefix=a/", "--dst-prefix=b/", "--find-renames=50%", "--submodule=short", "--ignore-submodules=none")
	if validObjectID(source) {
		args = append(args, source)
	}
	args = append(append(args, "--"), paths...)
	read := func() (string, error) {
		out, err := g.runInputWithEnvLimit(ctx, repo.Root, "", nil, limit, args...)
		if errors.Is(err, errOutputLimit) {
			return "", fmt.Errorf("patch exceeds the 16 MiB export limit: %w", err)
		}
		return out.stdout, err
	}
	text, err := read()
	if err != nil {
		return doc, err
	}
	if text == "" {
		return doc, errors.New("selected files have no exportable Git patch")
	}
	if !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
		return doc, errors.New("non-UTF-8 or raw-NUL textual patches cannot be previewed or copied; use Git to export this file")
	}
	if source == "working" {
		again, err := read()
		if err != nil {
			return doc, err
		}
		if again != text {
			return doc, ErrStalePatch
		}
	}
	if _, _, err = g.verifyCheckout(ctx, repo); err != nil {
		return doc, err
	}
	doc.Text, doc.Source, doc.Revision = text, source, repo.Head
	if validObjectID(source) {
		doc.Revision = source
	}
	return doc, nil
}
