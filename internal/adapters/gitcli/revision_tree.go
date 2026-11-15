package gitcli

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"vela/internal/domain"
)

func (g *Executor) fixedCommit(ctx context.Context, root, revision string) error {
	if !validObjectID(revision) {
		return errors.New("select a full immutable commit ID")
	}
	value, err := g.read(ctx, root, "rev-parse", "--verify", "--end-of-options", revision+"^{commit}")
	if err != nil {
		return err
	}
	if strings.TrimSpace(value) != revision {
		return errors.New("select a commit, not a tag object")
	}
	return nil
}

// ReadRevisionTree lists one directory only; it never reads or modifies checkout files.
func (g *Executor) ReadRevisionTree(ctx context.Context, root, revision, directory string) (domain.RevisionTree, error) {
	ctx = canonicalHistory(ctx)
	page := domain.RevisionTree{Revision: revision, Directory: directory}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if directory != "" && !validGitPath(directory) {
		return page, errors.New("invalid historical directory")
	}
	if err := g.fixedCommit(ctx, root, revision); err != nil {
		return page, err
	}
	object := revision + "^{tree}"
	if directory != "" {
		object = revision + ":" + directory
	}
	out, err := g.runInputWithEnvLimit(ctx, root, "", nil, 2<<20, "ls-tree", "-l", "-z", object)
	if err != nil {
		return page, err
	}
	for _, record := range nulFields(out.stdout) {
		header, name, ok := strings.Cut(record, "\t")
		parts := strings.Fields(header)
		if !ok || len(parts) != 4 || strings.Contains(name, "/") || !validGitPath(name) || !validObjectID(parts[2]) {
			return page, errors.New("invalid historical tree record")
		}
		entry := domain.RevisionTreeEntry{Path: name, Mode: parts[0], Kind: parts[1], OID: parts[2]}
		if parts[3] != "-" {
			entry.Size, err = strconv.ParseInt(parts[3], 10, 64)
			if err != nil || entry.Size < 0 {
				return page, errors.New("invalid historical blob size")
			}
		}
		if directory != "" {
			entry.Path = directory + "/" + name
		}
		page.Entries = append(page.Entries, entry)
		if len(page.Entries) > 10000 {
			return page, errors.New("historical directory exceeds 10000 entries")
		}
	}
	return page, nil
}
