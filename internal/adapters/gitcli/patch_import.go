package gitcli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

func importedPatchArgs(target string) []string {
	args := []string{"-c", "core.autocrlf=false", "-c", "apply.ignoreWhitespace=no", "apply", "--whitespace=nowarn", "-p1"}
	if target == "Index only" {
		args = append(args, "--cached")
	}
	return args
}

func (g *Executor) checkImportedPatch(ctx context.Context, root string, doc domain.ImportedPatch) error {
	out, err := g.runInput(ctx, root, doc.Text, append(importedPatchArgs(doc.Target), "--check", "-")...)
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
	paths, err := historyPaths(doc.Paths)
	if err != nil {
		return "", err
	}
	if err = rejectHistoryIndexAliases(paths, index); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(repo.Root)
	if err != nil {
		return "", err
	}
	defer root.Close()
	hash := sha256.New()
	fmt.Fprintf(hash, "%s\x00%s\x00%s\x00%s\x00%s\x00%s", repo.Root, repo.Head, repo.IndexVersion, repo.WorktreeVersion, doc.Target, doc.Text)
	for _, path := range paths {
		if err = projectPath(path, false); err != nil {
			return "", err
		}
		if err = ordinaryProjectPath(root, path, false); err != nil {
			return "", err
		}
		// Compare actual directory identities too: separate Git directories and
		// Windows short-name aliases need not contain a literal .git component.
		for ancestor := filepath.Dir(filepath.Join(repo.Root, filepath.FromSlash(path))); ; ancestor = filepath.Dir(ancestor) {
			info, err := os.Stat(ancestor)
			if err != nil {
				return "", err
			}
			for _, dir := range metadata {
				if os.SameFile(info, dir) {
					return "", errors.New("patch targets cannot access repository metadata")
				}
			}
			if filepath.Dir(ancestor) == ancestor {
				break
			}
		}
		regular := false
		for _, record := range nulFields(index) {
			header, name, _ := strings.Cut(record, "\t")
			fields := strings.Fields(header)
			if name == path && len(fields) == 3 && fields[2] == "0" && fields[0] == doc.Modes[path] && (fields[0] == "100644" || fields[0] == "100755") {
				regular = true
			}
		}
		if !regular {
			return "", fmt.Errorf("%q must already be a regular tracked file with the patch's unchanged mode", path)
		}
		attrs, err := g.read(ctx, repo.Root, "--literal-pathspecs", "check-attr", "-z", "filter", "working-tree-encoding", "eol", "text", "--", path)
		if err != nil {
			return "", err
		}
		values := nulFields(attrs)
		if len(values) != 12 {
			return "", errors.New("invalid patch target attributes")
		}
		for i := 2; i < len(values); i += 3 {
			if values[i] != "unspecified" && values[i] != "unset" {
				return "", errors.New("patch import does not support text/encoding/filter attributes")
			}
		}
		file, err := g.ReadFile(ctx, repo.Root, "project", domain.File{Path: path})
		if err != nil {
			return "", err
		}
		if file.DiskVersion == "" || strings.Contains(file.New, "\r") {
			return "", errors.New("patch import currently requires existing regular LF text files")
		}
		if doc.Target == "Index only" {
			text, err := g.readBlob(ctx, repo.Root, ":0:"+path)
			if err != nil {
				return "", err
			}
			if strings.Contains(text, "\r") {
				return "", errors.New("patch import currently requires LF text in the index")
			}
		}
		fmt.Fprintf(hash, "\x00%s\x00%s", path, file.DiskVersion)
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
	doc, err := application.ParseImportedPatch(preview.Text)
	if err != nil {
		return err
	}
	doc.Target = preview.Target
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
	out, err := g.runInput(ctx, repo.Root, doc.Text, append(importedPatchArgs(doc.Target), "-")...)
	if err != nil {
		return fmt.Errorf("patch application stopped; inspect refreshed files and index before retrying (changes may have been written): %w: %s", err, strings.TrimSpace(out.stderr))
	}
	return nil
}
