package gitcli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"vela/internal/application"
	"vela/internal/domain"
)

// PreviewImportedPatch reads a selected file once. Subsequent writes use the
// captured text, so replacing the external source cannot change confirmed input.
func (g *Executor) PreviewImportedPatch(ctx context.Context, repo domain.Repository, source, target string) (domain.ImportedPatch, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var empty domain.ImportedPatch
	if !filepath.IsAbs(source) {
		return empty, errors.New("choose an absolute patch file path")
	}
	volumeRoot := filepath.VolumeName(source) + string(os.PathSeparator)
	fsRoot, err := os.OpenRoot(volumeRoot)
	if err != nil {
		return empty, err
	}
	rel, err := filepath.Rel(volumeRoot, source)
	if err == nil {
		err = ordinaryProjectPath(fsRoot, filepath.ToSlash(rel), false)
	}
	fsRoot.Close()
	if err != nil {
		return empty, fmt.Errorf("patch source cannot traverse links: %w", err)
	}
	info, err := os.Lstat(source)
	if err != nil || info == nil || !info.Mode().IsRegular() || info.Size() > application.MaxImportedPatchBytes {
		return empty, errors.New("patch source must be a regular file of at most 2 MiB")
	}
	f, err := os.Open(source)
	if err != nil {
		return empty, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return empty, errors.New("patch source changed while opening")
	}
	raw, err := io.ReadAll(io.LimitReader(&projectContextReader{ctx: ctx, reader: f}, application.MaxImportedPatchBytes+1))
	if err != nil {
		return empty, fmt.Errorf("read patch: %w", err)
	}
	doc, err := application.ParseImportedPatch(string(raw))
	if err != nil {
		return empty, err
	}
	doc.Target = target
	return g.ReviewImportedPatch(ctx, repo, doc)
}

func (g *Executor) ReviewImportedPatch(ctx context.Context, repo domain.Repository, input domain.ImportedPatch) (domain.ImportedPatch, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	doc, err := application.RelocateImportedPatch(input.Text, input.Strip, input.Directory)
	if err != nil {
		return doc, err
	}
	doc.Target, doc.Reverse = input.Target, input.Reverse
	g.write.Lock()
	defer g.write.Unlock()
	doc.Version, err = g.importedPatchVersion(ctx, repo, doc)
	if err == nil {
		err = g.checkImportedPatch(ctx, repo.Root, doc)
	}
	if err != nil {
		doc.Version = ""
	}
	return doc, err
}

func importedPatchArgs(doc domain.ImportedPatch) []string {
	args := []string{"-c", "core.autocrlf=false", "-c", "apply.ignoreWhitespace=no", "apply", "--whitespace=nowarn", "-p" + strconv.Itoa(doc.Strip)}
	if doc.Directory != "" {
		args = append(args, "--directory="+doc.Directory)
	}
	if doc.Target == "Index only" {
		args = append(args, "--cached")
	}
	if doc.Reverse {
		args = append(args, "--reverse")
	}
	return args
}

func (g *Executor) checkImportedPatch(ctx context.Context, root string, doc domain.ImportedPatch) error {
	out, err := g.runInput(ctx, root, doc.Text, append(importedPatchArgs(doc), "--check", "-")...)
	if err != nil {
		return fmt.Errorf("patch does not apply cleanly: %w: %s", err, strings.TrimSpace(out.stderr))
	}
	return nil
}

func (g *Executor) importedPatchVersion(ctx context.Context, repo domain.Repository, doc domain.ImportedPatch) (string, error) {
	if doc.Target != "Working tree" && doc.Target != "Index only" {
		return "", errors.New("choose Working tree or Index only")
	}
	_, index, err := g.verifyCheckout(ctx, repo)
	if err != nil {
		return "", err
	}
	gitDir, err := g.gitDirectory(ctx, repo.Root)
	if err != nil {
		return "", err
	}
	common, err := g.read(ctx, repo.Root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	commonDir := filepath.Clean(strings.TrimSuffix(common, "\n"))
	if gitDir != repo.GitDir || commonDir != repo.CommonDir {
		return "", errors.New("repository metadata location changed; refresh before importing")
	}
	var metadata []os.FileInfo
	for _, dir := range []string{gitDir, commonDir} {
		info, err := os.Stat(dir)
		if err != nil {
			return "", err
		}
		metadata = append(metadata, info)
	}
	if err = g.checkCommitState(ctx, repo.Root, index); err != nil {
		return "", err
	}
	if err = g.rejectHiddenIndexFlags(ctx, repo.Root); err != nil {
		return "", err
	}
	if err = g.verifyWorktree(ctx, repo); err != nil {
		return "", err
	}
	hash := sha256.New()
	fmt.Fprintf(hash, "%s\x00%s\x00%s\x00%s\x00%s\x00%s", repo.Root, repo.Head, repo.IndexVersion, repo.WorktreeVersion, doc.Target, doc.Text)
	fmt.Fprintf(hash, "\x00reverse=%t\x00strip=%d\x00directory=%s", doc.Reverse, doc.Strip, doc.Directory)
	if err := g.fingerprintImportedPaths(ctx, repo, doc, index, metadata, hash); err != nil {
		return "", err
	}
	if _, _, err = g.verifyCheckout(ctx, repo); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), ctx.Err()
}

func (g *Executor) ApplyImportedPatch(ctx context.Context, repo domain.Repository, preview domain.ImportedPatch) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	g.write.Lock()
	defer g.write.Unlock()
	doc, err := application.RelocateImportedPatch(preview.Text, preview.Strip, preview.Directory)
	if err != nil {
		return err
	}
	doc.Target, doc.Reverse = preview.Target, preview.Reverse
	verify := func() error {
		version, err := g.importedPatchVersion(ctx, repo, doc)
		if err != nil {
			return err
		}
		if preview.Version == "" || version != preview.Version {
			return errors.New("patch target changed; reopen the import preview")
		}
		return nil
	}
	if err = verify(); err != nil {
		return err
	}
	if err = g.checkImportedPatch(ctx, repo.Root, doc); err != nil {
		return err
	}
	if err = verify(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	out, err := g.runInput(ctx, repo.Root, doc.Text, append(importedPatchArgs(doc), "-")...)
	if err != nil {
		return fmt.Errorf("patch application stopped; inspect refreshed files and index before retrying (changes may have been written): %w: %s", err, strings.TrimSpace(out.stderr))
	}
	return nil
}
