package gitcli

import (
	"context"
	"errors"
	"strings"
)

func (g *Executor) resolveSequenceCommits(ctx context.Context, root string, tokens []string) ([]string, error) {
	for _, token := range tokens {
		if len(token) < 4 || len(token) > 64 || strings.IndexFunc(token, func(r rune) bool { return !strings.ContainsRune("0123456789abcdef", r) }) >= 0 {
			return nil, errors.New("invalid Git sequencer commit identity")
		}
	}
	out, err := g.runInputWithEnvLimit(ctx, root, strings.Join(tokens, "\n")+"\n", nil, 16<<10, "cat-file", "--batch-check=%(objectname) %(objecttype)")
	if err != nil {
		return nil, err
	}
	rows := strings.Split(strings.TrimSuffix(out.stdout, "\n"), "\n")
	if len(rows) != len(tokens) {
		return nil, errors.New("unexpected Git sequencer commit count")
	}
	for i, row := range rows {
		oid, kind, _ := strings.Cut(row, " ")
		if kind != "commit" || !validObjectID(oid) || !strings.HasPrefix(oid, tokens[i]) {
			return nil, errors.New("missing or ambiguous Git sequencer commit")
		}
		rows[i] = oid
	}
	return rows, nil
}

func (g *Executor) protectSequencePaths(ctx context.Context, root string, pending []string) error {
	commits, err := g.resolveSequenceCommits(ctx, root, pending)
	if err != nil {
		return err
	}
	guard, err := g.checkoutPathGuard(ctx, root)
	if err != nil || guard == nil {
		return err
	}
	out, err := g.runInputWithEnvLimit(ctx, root, strings.Join(commits, "\n")+"\n", nil, 16<<20,
		"diff-tree", "--stdin", "--root", "-m", "-r", "-z", "--no-commit-id", "--name-only", "--no-renames", "--no-ext-diff", "--no-textconv")
	if err != nil {
		return err
	}
	for _, path := range nulFields(out.stdout) {
		if err := guard(path); err != nil {
			return err
		}
	}
	return nil
}
