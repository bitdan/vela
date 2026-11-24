package gitcli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"vela/internal/application"
	"vela/internal/domain"
)

func (g *Executor) readShelfPatch(ctx context.Context, repo domain.Repository, source, name string) (domain.PatchDocument, error) {
	ctx = canonicalHistory(ctx)
	var empty domain.PatchDocument
	if err := projectPath(name, false); err != nil {
		return empty, err
	}
	if _, _, err := g.verifyCheckout(ctx, repo); err != nil {
		return empty, err
	}
	if err := g.verifyShelfRepository(ctx, repo); err != nil {
		return empty, err
	}
	if err := g.verifyWorktree(ctx, repo); err != nil {
		return empty, err
	}
	entries, err := g.readShelves(ctx, repo)
	if err != nil {
		return empty, err
	}
	id := strings.TrimPrefix(source, "shelf:")
	i := slices.IndexFunc(entries, func(e domain.Shelf) bool { return e.ID == id && !e.Deleted })
	if i < 0 {
		return empty, errors.New("Shelf changed; reopen the hunk preview")
	}
	entry := entries[i]
	if !slices.ContainsFunc(entry.Files, func(f domain.File) bool { return f.Path == name && f.OldPath == "" && f.Status == "M" }) {
		return empty, errors.New("Hunk restore requires an ordinary modified text file")
	}
	mode := ""
	for _, tip := range []string{entry.Base, entry.Index, entry.OID} {
		out, err := g.read(ctx, repo.Root, "--literal-pathspecs", "ls-tree", "-z", tip, "--", name)
		if err != nil {
			return empty, err
		}
		rows := nulFields(out)
		if len(rows) != 1 {
			return empty, errors.New("Shelf additions/deletions require whole-file restore")
		}
		header, path, ok := strings.Cut(rows[0], "\t")
		fields := strings.Fields(header)
		if !ok || path != name || len(fields) != 3 || fields[1] != "blob" || (fields[0] != "100644" && fields[0] != "100755") {
			return empty, errors.New("Shelf hunks require regular blobs")
		}
		if mode != "" && fields[0] != mode {
			return empty, errors.New("Shelf mode changes require whole-file restore")
		}
		mode = fields[0]
	}
	attrs, err := g.read(ctx, repo.Root, "--literal-pathspecs", "check-attr", "-z", "filter", "working-tree-encoding", "eol", "text", "--", name)
	if err != nil {
		return empty, err
	}
	values := nulFields(attrs)
	if len(values) != 12 {
		return empty, errors.New("invalid Shelf hunk attributes")
	}
	for i := 2; i < len(values); i += 3 {
		if values[i] != "unspecified" && values[i] != "unset" {
			return empty, errors.New("Shelf hunks do not support text/encoding/filter attributes")
		}
	}
	file, err := g.ReadFile(ctx, repo.Root, "project", domain.File{Path: name})
	if err != nil {
		return empty, err
	}
	if file.DiskVersion == "" || strings.Contains(file.New, "\r") {
		return empty, errors.New("Shelf hunks require a regular LF working file")
	}
	args := []string{"--literal-pathspecs", "diff", "--no-ext-diff", "--no-textconv", "--no-color", "--no-renames", "--no-relative", "--full-index", "--src-prefix=a/", "--dst-prefix=b/", "--unified=3", "--no-indent-heuristic", "--diff-algorithm=myers", entry.Base, entry.OID, "--", name}
	args = append([]string{"-c", "diff.suppressBlankEmpty=false"}, args...)
	out, err := g.runInputWithEnvLimit(ctx, repo.Root, "", nil, application.MaxImportedPatchBytes, args...)
	if err != nil {
		return empty, err
	}
	if strings.Contains(out.stdout, "\r") {
		return empty, errors.New("Shelf hunks currently require LF saved text")
	}
	doc, err := application.ParseTextPatch(out.stdout, name)
	if err != nil {
		return empty, err
	}
	if len(doc.Hunks) > application.MaxImportedPatchHunks {
		return empty, errors.New("Shelf exceeds 10000 hunks")
	}
	after, err := g.patchDiskVersion(ctx, repo.Root, name, "working")
	if err != nil {
		return empty, err
	}
	if after != file.DiskVersion {
		return empty, ErrStalePatch
	}
	doc.Source = source
	doc.Version = fmt.Sprintf("%x", sha256.Sum256([]byte(repo.Root+"\x00"+repo.Head+"\x00"+repo.IndexVersion+"\x00"+repo.WorktreeVersion+"\x00"+entry.ID+entry.OID+entry.Base+entry.Index+after+out.stdout)))
	return doc, nil
}

// projectShelfHunks creates a private, unreferenced Git object, never writes the
// real Index/worktree, and never overlays unselected saved staged contents.
func (g *Executor) projectShelfHunks(ctx context.Context, repo domain.Repository, entry domain.Shelf, files []domain.File, op domain.RepositoryOperation) (domain.Shelf, error) {
	bad := func(message string) (domain.Shelf, error) { return domain.Shelf{}, errors.New(message) }
	if op.RestoreIndex || op.RemoveApplied {
		return bad("Hunk restore does not restore saved staged state and always retains the original Shelf")
	}
	if err := application.ValidateShelfHunks(op.Paths, op.ShelfHunks); err != nil {
		return domain.Shelf{}, err
	}
	if len(op.ShelfHunks) > len(files) {
		return bad("invalid Shelf hunk paths")
	}
	known := map[string]bool{}
	for _, f := range files {
		known[f.Path] = true
		if strings.EqualFold(filepath.Base(f.Path), ".gitattributes") {
			return bad("restore attribute files separately before selecting Shelf hunks")
		}
	}
	seen := map[string]bool{}
	var patch strings.Builder
	for _, selection := range op.ShelfHunks {
		if !known[selection.Path] || seen[selection.Path] || len(selection.Hunks) == 0 || selection.Version == "" {
			return bad("select nonempty, unique Shelf hunk paths")
		}
		seen[selection.Path] = true
		doc, err := g.readShelfPatch(ctx, repo, "shelf:"+entry.ID, selection.Path)
		if err != nil {
			return domain.Shelf{}, err
		}
		if doc.Version != selection.Version {
			return domain.Shelf{}, ErrStalePatch
		}
		chosen := map[int]bool{}
		for _, n := range selection.Hunks {
			if n < 0 || n >= len(doc.Hunks) || chosen[n] {
				return bad("invalid or duplicate Shelf hunk ordinal")
			}
			chosen[n] = true
		}
		patch.WriteString(doc.Header)
		for n, h := range doc.Hunks {
			if chosen[n] {
				patch.WriteString(h.Text)
			}
		}
		if patch.Len() > application.MaxImportedPatchBytes {
			return bad("selected Shelf patch exceeds 2 MiB")
		}
	}
	var whole []domain.File
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
		if !seen[f.Path] {
			whole = append(whole, f)
		}
	}
	tip := entry.Base
	if len(whole) > 0 {
		projected, err := g.projectShelf(ctx, repo, entry, whole)
		if err != nil {
			return domain.Shelf{}, err
		}
		tip = projected.OID
	}
	tmp, err := os.MkdirTemp("", "vela-shelf-hunks-")
	if err != nil {
		return domain.Shelf{}, err
	}
	defer os.RemoveAll(tmp)
	env := []string{"GIT_INDEX_FILE=" + filepath.Join(tmp, "index")}
	run := func(input string, args ...string) (string, error) {
		out, err := g.runInputWithEnv(ctx, repo.Root, input, env, args...)
		return strings.TrimSpace(out.stdout), err
	}
	if _, err = run("", "read-tree", tip); err != nil {
		return domain.Shelf{}, err
	}
	args := []string{"-c", "core.autocrlf=false", "-c", "apply.ignoreWhitespace=no", "apply", "--cached", "--whitespace=nowarn", "-p1"}
	if _, err = run(patch.String(), append(slices.Clone(args), "--check", "-")...); err != nil {
		return domain.Shelf{}, fmt.Errorf("Shelf hunk preflight: %w", err)
	}
	if _, err = run(patch.String(), append(args, "-")...); err != nil {
		return domain.Shelf{}, err
	}
	tree, err := run("", "write-tree")
	if err != nil {
		return domain.Shelf{}, err
	}
	if err = g.verifyProjectedShelfPaths(ctx, repo.Root, entry.Base, tree, paths); err != nil {
		return domain.Shelf{}, err
	}
	baseTree, err := g.read(ctx, repo.Root, "rev-parse", entry.Base+"^{tree}")
	if err != nil {
		return domain.Shelf{}, err
	}
	indexCommit, err := g.shelfCommit(ctx, repo.Root, strings.TrimSpace(baseTree), "vela-shelf-index-v1", entry.Base)
	if err != nil {
		return domain.Shelf{}, err
	}
	commit, err := g.shelfCommit(ctx, repo.Root, tree, shelfSubject+entry.Title, entry.Base, indexCommit)
	if err != nil {
		return domain.Shelf{}, err
	}
	entry.OID, entry.Index, entry.Files = commit, indexCommit, slices.Clone(files)
	return entry, nil
}
