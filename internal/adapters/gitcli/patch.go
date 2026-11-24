package gitcli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"vela/internal/application"
	"vela/internal/domain"
)

var ErrStalePatch = errors.New("file, HEAD or index changed; reopen the hunk preview")

// ReadPatch produces the patch displayed to the user. Writes regenerate it under
// the executor lock and accept only a matching fingerprint and hunk ordinal.
func (g *Executor) ReadPatch(ctx context.Context, repo domain.Repository, source, path string) (domain.PatchDocument, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return g.readPatch(ctx, repo, source, path)
}

func (g *Executor) readPatch(ctx context.Context, repo domain.Repository, source, path string) (domain.PatchDocument, error) {
	if strings.HasPrefix(source, "shelf:") {
		return g.readShelfPatch(ctx, repo, source, path)
	}
	var doc domain.PatchDocument
	if !validGitPath(path) || (source != "working" && source != "staged") {
		return doc, errors.New("select a working or staged file")
	}
	_, index, err := g.verifyCheckout(ctx, repo)
	if err != nil {
		return doc, err
	}
	if err = g.checkCommitState(ctx, repo.Root, index); err != nil {
		return doc, err
	}
	staged, working, err := g.readStatus(ctx, repo.Root)
	if err != nil {
		return doc, err
	}
	files := working
	if source == "staged" {
		files = staged
	}
	file, ok := filesByPath(files)[path]
	if !ok || file.Status != "M" || file.OldPath != "" {
		return doc, errors.New("hunks require an ordinary tracked modified file; additions, deletions, renames and conflicts are unsupported")
	}
	regular := false
	for _, entry := range nulFields(index) {
		fields, name, _ := strings.Cut(entry, "\t")
		parts := strings.Fields(fields)
		if name == path && len(parts) == 3 && parts[2] == "0" && (parts[0] == "100644" || parts[0] == "100755") {
			regular = true
		}
	}
	if !regular {
		return doc, errors.New("hunks require a regular tracked file")
	}
	preview, err := g.ReadFile(ctx, repo.Root, source, file)
	if err != nil {
		return doc, err
	}
	if strings.Count(preview.Old, "\n")+strings.Count(preview.New, "\n") > 40000 {
		return doc, errors.New("hunk preview exceeds 40000 lines")
	}
	before, err := g.patchDiskVersion(ctx, repo.Root, path, source)
	if err != nil {
		return doc, err
	}
	args := []string{"--literal-pathspecs", "-c", "diff.suppressBlankEmpty=false", "diff", "--no-ext-diff", "--no-textconv", "--no-color", "--no-renames", "--no-relative", "--full-index", "--src-prefix=a/", "--dst-prefix=b/", "--unified=3", "--no-indent-heuristic", "--diff-algorithm=myers"}
	if source == "staged" {
		args = append(args, "--cached")
	}
	args = append(args, "--", path)
	raw, err := g.read(ctx, repo.Root, args...)
	if err != nil {
		return doc, err
	}
	doc, err = application.ParseTextPatch(raw, path)
	if err != nil {
		return doc, err
	}
	after, err := g.patchDiskVersion(ctx, repo.Root, path, source)
	if err != nil {
		return doc, err
	}
	if before != after {
		return doc, ErrStalePatch
	}
	if _, _, err = g.verifyCheckout(ctx, repo); err != nil {
		return doc, err
	}
	doc.Source = source
	doc.Version = fmt.Sprintf("%x", sha256.Sum256([]byte(path+"\x00"+source+"\x00"+repo.Head+"\x00"+repo.IndexVersion+"\x00"+after+"\x00"+raw)))
	return doc, nil
}

func (g *Executor) patchDiskVersion(ctx context.Context, root, path, source string) (string, error) {
	if source == "staged" {
		return "index-only", nil
	}
	file, err := g.ReadFile(ctx, root, "project", domain.File{Path: path})
	if err != nil {
		return "", err
	}
	if file.DiskVersion == "" {
		return "", errors.New("hunk target must be a regular file, not a symbolic link")
	}
	return file.DiskVersion, nil
}

func (g *Executor) applyHunk(ctx context.Context, repo domain.Repository, op domain.RepositoryOperation) error {
	source := "working"
	if op.Kind == domain.UnstageHunk {
		source = "staged"
	}
	doc, err := g.readPatch(ctx, repo, source, op.Path)
	if err != nil {
		return err
	}
	if op.PatchVersion == "" || doc.Version != op.PatchVersion {
		return ErrStalePatch
	}
	if op.HunkIndex < 0 || op.HunkIndex >= len(doc.Hunks) {
		return errors.New("invalid Git hunk selection")
	}
	if op.Kind == domain.DiscardHunk {
		file, err := g.ReadFile(ctx, repo.Root, "project", domain.File{Path: op.Path})
		if err != nil {
			return err
		}
		if strings.Contains(file.New, "\r") {
			return errors.New("discard hunk currently requires LF text; CRLF files can still be staged or unstaged")
		}
		attrs, err := g.read(ctx, repo.Root, "--literal-pathspecs", "check-attr", "-z", "filter", "working-tree-encoding", "eol", "text", "--", op.Path)
		if err != nil {
			return err
		}
		fields := nulFields(attrs)
		for i := 0; i+2 < len(fields); i += 3 {
			if fields[i+2] != "unspecified" && fields[i+2] != "unset" {
				return errors.New("discard hunk is unavailable for files with text/encoding/filter attributes")
			}
		}
	}
	patch := doc.Header + doc.Hunks[op.HunkIndex].Text
	args := []string{"-c", "core.autocrlf=false", "-c", "apply.ignoreWhitespace=no", "apply", "--whitespace=nowarn", "-p1"}
	if op.Kind != domain.DiscardHunk {
		args = append(args, "--cached")
	}
	if op.Kind != domain.StageHunk {
		args = append(args, "--reverse")
	}
	if _, err = g.runInput(ctx, repo.Root, patch, append(append([]string{}, args...), "--check", "-")...); err != nil {
		return fmt.Errorf("hunk no longer applies cleanly: %w", err)
	}
	latest, err := g.readPatch(ctx, repo, source, op.Path)
	if err != nil {
		return err
	}
	if latest.Version != doc.Version {
		return ErrStalePatch
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	_, err = g.runInput(ctx, repo.Root, patch, append(args, "-")...)
	return err
}
