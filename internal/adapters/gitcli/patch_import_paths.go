package gitcli

import (
	"context"
	"errors"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"vela/internal/application"
	"vela/internal/domain"
)

type importedPathRole struct {
	existing, structural, raw, modeChange bool
	mode                                  string
	lineEnding                            string
	size                                  int
}

func importedPatchRoles(doc domain.ImportedPatch) (map[string]importedPathRole, error) {
	roles := map[string]importedPathRole{}
	for _, file := range doc.Files {
		lineEnding, err := application.ImportedPatchLineEnding(file)
		if err != nil {
			return nil, err
		}
		name, err := application.ResolveImportedPatchPath(file.Path, doc.Strip, doc.Directory)
		if err != nil {
			return nil, err
		}
		modeChange := file.OldMode != "" && file.NewMode != "" && file.OldMode != file.NewMode
		if modeChange && doc.Target == "Working tree" && runtime.GOOS == "windows" {
			return nil, errors.New("Windows does not preserve Unix executable modes; choose Index only for permission changes")
		}
		raw := file.Binary || len(file.Hunks) == 0 && (file.Kind == "P" || file.Kind == "R")
		role := importedPathRole{existing: true, structural: file.Kind != "", raw: raw, modeChange: modeChange, mode: file.OldMode, size: -1, lineEnding: lineEnding}
		if file.Binary {
			role.size = file.OldSize
		}
		if doc.Reverse {
			role.mode = file.NewMode
			if file.Binary {
				role.size = file.NewSize
			}
		}
		switch file.Kind {
		case "A":
			role.existing = doc.Reverse
			role.mode = file.NewMode
		case "D":
			role.existing = !doc.Reverse
			role.mode = file.OldMode
		case "R":
			old, err := application.ResolveImportedPatchPath(file.OldPath, doc.Strip, doc.Directory)
			if err != nil {
				return nil, err
			}
			oldRole := role
			oldRole.existing = !doc.Reverse
			oldRole.mode = file.OldMode
			roles[old] = oldRole
			role.existing = doc.Reverse
			role.mode = file.NewMode
		}
		roles[name] = role
	}
	return roles, nil
}

// validateImportedPath permits missing destination components while refusing
// links, special files, platform aliases and real (possibly separate) Git dirs.
func validateImportedPath(root *os.Root, name string, metadata []os.FileInfo) (os.FileInfo, error) {
	if err := projectPath(name, false); err != nil {
		return nil, err
	}
	parts := strings.Split(name, "/")
	var leaf os.FileInfo
	for i := range parts {
		info, err := root.Lstat(filepath.FromSlash(strings.Join(parts[:i+1], "/")))
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) || i < len(parts)-1 && !info.IsDir() {
			return nil, errors.New("patch target traverses a linked, special or non-directory path")
		}
		if i == len(parts)-1 {
			if !info.Mode().IsRegular() {
				return nil, errors.New("patch target is a directory")
			}
			leaf = info
		}
	}
	for ancestor := filepath.Dir(filepath.Join(root.Name(), filepath.FromSlash(name))); ; ancestor = filepath.Dir(ancestor) {
		info, err := os.Stat(ancestor)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if info != nil {
			for _, dir := range metadata {
				if os.SameFile(info, dir) {
					return nil, errors.New("patch targets cannot access repository metadata")
				}
			}
		}
		if filepath.Dir(ancestor) == ancestor {
			break
		}
	}
	return leaf, nil
}

func (g *Executor) fingerprintImportedPaths(ctx context.Context, repo domain.Repository, doc domain.ImportedPatch, index string, metadata []os.FileInfo, digest hash.Hash) error {
	roles, err := importedPatchRoles(doc)
	if err != nil {
		return err
	}
	paths := doc.Paths
	if len(paths) == 0 || len(paths) > 200 {
		return errors.New("patch endpoints exceed import limits")
	}
	for i, name := range paths {
		if err := historyPlatformPath(name); err != nil {
			return err
		}
		key := rebasePathKey(name)
		for _, other := range paths[:i] {
			otherKey := rebasePathKey(other)
			if key == otherKey || strings.HasPrefix(key, otherKey+"/") || strings.HasPrefix(otherKey, key+"/") {
				return errors.New("patch endpoints collide or replace a file/directory")
			}
		}
	}
	if err := rejectHistoryIndexAliases(paths, index); err != nil {
		return err
	}
	root, err := os.OpenRoot(repo.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	indexModes := map[string]string{}
	for _, record := range nulFields(index) {
		header, name, _ := strings.Cut(record, "\t")
		fields := strings.Fields(header)
		if len(fields) == 3 {
			if fields[2] != "0" {
				return errors.New("resolve index conflicts before importing")
			}
			indexModes[name] = fields[0]
		}
	}
	for _, name := range paths {
		role, ok := roles[name]
		if !ok {
			return errors.New("invalid imported endpoint plan")
		}
		info, err := validateImportedPath(root, name, metadata)
		if err != nil {
			return err
		}
		mode := indexModes[name]
		if mode != "" && (mode != "100644" && mode != "100755" || (doc.Target == "Index only" || !role.modeChange) && role.mode != "" && mode != role.mode) {
			return fmt.Errorf("%q has an unsupported or changed index mode", name)
		}
		if doc.Target == "Index only" {
			if role.existing && mode == "" || !role.existing && mode != "" {
				return fmt.Errorf("%q has an unexpected index presence", name)
			}
		} else {
			if role.existing && info == nil || !role.existing && info != nil {
				return fmt.Errorf("%q has an unexpected disk presence; existing targets will not be overwritten", name)
			}
			if !role.structural && mode == "" {
				return fmt.Errorf("%q must already be tracked", name)
			}
		}
		attrs, err := g.read(ctx, repo.Root, "--literal-pathspecs", "check-attr", "-z", "filter", "working-tree-encoding", "eol", "text", "--", name)
		if err != nil {
			return err
		}
		values := nulFields(attrs)
		if len(values) != 12 {
			return errors.New("invalid patch target attributes")
		}
		for i := 2; i < len(values); i += 3 {
			if values[i] != "unspecified" && values[i] != "unset" {
				return errors.New("patch import does not support text/encoding/filter attributes")
			}
		}
		version := "missing"
		if info != nil {
			if role.modeChange && role.existing && doc.Target == "Working tree" && (info.Mode().Perm()&0111 != 0) != (role.mode == "100755") {
				return errors.New("working file executable mode changed; refresh the patch preview")
			}
			if role.raw {
				data, fingerprint, err := readImportedDisk(ctx, root, name, info)
				if err != nil {
					return err
				}
				if doc.Target == "Working tree" && role.existing && role.size >= 0 && len(data) != role.size {
					return errors.New("binary working file size differs from patch source")
				}
				version = fingerprint
			} else {
				file, err := g.ReadFile(ctx, repo.Root, "project", domain.File{Path: name})
				if err != nil {
					return err
				}
				if file.DiskVersion == "" {
					return errors.New("patch import requires regular UTF-8 text files")
				}
				if err := validateImportedTextEOL(file.New, role, doc.Target == "Working tree"); err != nil {
					return err
				}
				version = file.DiskVersion
			}
		}
		if mode != "" {
			if role.raw {
				data, err := g.readImportedBlob(ctx, repo.Root, name)
				if err != nil {
					return err
				}
				if doc.Target == "Index only" && role.existing && role.size >= 0 && len(data) != role.size {
					return errors.New("binary index file size differs from patch source")
				}
			} else {
				text, err := g.readBlob(ctx, repo.Root, ":0:"+name)
				if err != nil {
					return err
				}
				if err := validateImportedTextEOL(text, role, doc.Target == "Index only"); err != nil {
					return err
				}
			}
		}
		fmt.Fprintf(digest, "\x00%s\x00%s\x00%s", name, mode, version)
	}
	return ctx.Err()
}

func validateImportedTextEOL(text string, role importedPathRole, target bool) error {
	ending, err := application.UniformTextLineEnding(text)
	if err != nil {
		return err
	}
	// Matching content EOL prevents insertion-only/no-context patches from
	// introducing mixed line endings into the rest of an existing target.
	if target && role.existing && ending != "" && role.lineEnding != "" && ending != role.lineEnding {
		return errors.New("patch and existing target line endings differ; mixed line endings are unsupported")
	}
	return nil
}
