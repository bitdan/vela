package gitcli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode/utf8"

	"vela/internal/domain"
)

const shelfSubject = "vela-shelf-v1: "

func shelfPrefix(gitDir string, deleted bool) string {
	gitDir = filepath.Clean(gitDir)
	if runtime.GOOS == "windows" {
		gitDir = strings.ToLower(gitDir)
	}
	hash := sha256.Sum256([]byte(gitDir))
	kind := "shelves"
	if deleted {
		kind = "shelf-deleted"
	}
	return fmt.Sprintf("refs/vela/%s/%x/", kind, hash[:8])
}

func validShelfID(id string) bool { return len(id) == 32 && strings.Trim(id, "0123456789abcdef") == "" }
func validShelfName(name string) bool {
	return len(name) > 0 && len(name) <= 200 && utf8.ValidString(name) && !strings.ContainsAny(name, "\x00\r\n")
}

func (g *Executor) readShelves(ctx context.Context, repo domain.Repository) ([]domain.Shelf, error) {
	out, err := g.runInputWithEnvLimit(ctx, repo.Root, "", nil, 2<<20, "for-each-ref", "--sort=-creatordate", "--format=%(refname)%00%(objectname)%00%(parent)%00%(subject)", shelfPrefix(repo.GitDir, false), shelfPrefix(repo.GitDir, true))
	if err != nil {
		return nil, err
	}
	if out.stdout == "" {
		return nil, nil
	}
	rows := strings.Split(strings.TrimSuffix(out.stdout, "\n"), "\n")
	if len(rows) > 200 {
		return nil, errors.New("shelf store exceeds 200 entries including Recently Deleted")
	}
	var shelves []domain.Shelf
	for _, row := range rows {
		fields := strings.Split(row, "\x00")
		if len(fields) != 4 {
			return nil, errors.New("invalid private shelf record")
		}
		deleted := strings.HasPrefix(fields[0], shelfPrefix(repo.GitDir, true))
		id := strings.TrimPrefix(fields[0], shelfPrefix(repo.GitDir, deleted))
		parents := strings.Fields(fields[2])
		name, ok := strings.CutPrefix(fields[3], shelfSubject)
		if !ok || !validShelfID(id) || !validObjectID(fields[1]) || len(parents) != 2 || !validObjectID(parents[0]) || !validObjectID(parents[1]) || !validShelfName(name) {
			return nil, errors.New("invalid private shelf identity or ancestry")
		}
		entry := domain.Shelf{ID: id, OID: fields[1], Base: parents[0], Index: parents[1], Title: name, Deleted: deleted}
		files := map[string]domain.File{}
		for _, tip := range []string{entry.Index, entry.OID} {
			value, err := g.read(ctx, repo.Root, "diff-tree", "--no-commit-id", "-r", "--name-status", "-z", "--no-renames", entry.Base, tip, "--")
			if err != nil {
				return nil, err
			}
			parts := nulFields(value)
			if len(parts)%2 != 0 {
				return nil, errors.New("invalid shelf file list")
			}
			for i := 0; i < len(parts); i += 2 {
				if !validGitPath(parts[i+1]) {
					return nil, errors.New("invalid shelf file path")
				}
				files[parts[i+1]] = domain.File{Path: parts[i+1], Status: parts[i]}
			}
		}
		for _, f := range files {
			entry.Files = append(entry.Files, f)
		}
		slices.SortFunc(entry.Files, func(a, b domain.File) int { return strings.Compare(a.Path, b.Path) })
		shelves = append(shelves, entry)
	}
	return shelves, nil
}

func (g *Executor) readShelfFile(ctx context.Context, root, source string, file domain.File) (domain.File, error) {
	var err error
	repo := domain.Repository{Root: root}
	repo.GitDir, err = g.gitDirectory(ctx, root)
	if err != nil {
		return file, err
	}
	shelves, err := g.readShelves(ctx, repo)
	if err != nil {
		return file, err
	}
	id := strings.TrimPrefix(strings.TrimPrefix(source, "shelf-index:"), "shelf:")
	for _, entry := range shelves {
		if entry.ID == id {
			tip := entry.OID
			if strings.HasPrefix(source, "shelf-index:") {
				tip = entry.Index
			}
			file.Old, file.New, file.DiskVersion = "", "", ""
			exists := func(rev string) (bool, error) {
				out, err := g.run(ctx, root, "--literal-pathspecs", "ls-tree", "-z", rev, "--", file.Path)
				return out.stdout != "", err
			}
			if ok, err := exists(entry.Base); err != nil {
				return file, err
			} else if ok {
				file.Old, err = g.readBlob(ctx, root, entry.Base+":"+file.Path)
				if err != nil {
					return file, err
				}
			}
			if ok, err := exists(tip); err != nil {
				return file, err
			} else if ok {
				file.New, err = g.readBlob(ctx, root, tip+":"+file.Path)
				if err != nil {
					return file, err
				}
			}
			return file, nil
		}
	}
	return file, errors.New("shelf is no longer available; refresh the repository")
}
