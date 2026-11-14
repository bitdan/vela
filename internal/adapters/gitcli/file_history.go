package gitcli

import (
	"context"
	"errors"
	"strconv"
	"time"

	"vela/internal/domain"
)

// ReadFileHistory follows one literal path from a fixed OID. Reusing that OID
// makes offset pages stable when another process adds commits or moves refs.
func (g *Executor) ReadFileHistory(ctx context.Context, root, revision, path string, offset, limit int) (domain.FileHistoryPage, error) {
	var page domain.FileHistoryPage
	if !validGitPath(path) || !validObjectID(revision) || offset < 0 || offset > 100000 || limit < 1 || limit > 500 {
		return page, errors.New("invalid file history path, fixed revision or page bounds")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// --skip can bypass the rename record before --follow rewrites its path.
	// Rewalk the bounded prefix at the same OID, then slice the parsed records.
	value, err := g.read(ctx, root, "--literal-pathspecs", "log", "--follow", "--topo-order", "--no-decorate", "--no-color", "--encoding=UTF-8", "-n", strconv.Itoa(offset+limit+1),
		"--format=tformat:%x00%H%x00%P%x00%an%x00%ae%x00%at%x00%ct%x00%s%x00%b", "--name-status", "-z", "--root", "--diff-merges=first-parent", "--no-ext-diff", "--no-textconv", "--find-renames=50%", "--no-notes", "--no-show-signature", revision, "--", path)
	if err != nil {
		return page, err
	}
	commits, err := parseHistory(value, nil)
	if err != nil {
		return page, err
	}
	for _, commit := range commits {
		if len(commit.Files) > 1 {
			return page, errors.New("history query must target one file, not a directory")
		}
	}
	if offset >= len(commits) {
		return page, nil
	}
	commits = commits[offset:]
	page.HasMore = len(commits) > limit
	if page.HasMore {
		commits = commits[:limit]
	}
	page.Commits = commits
	return page, nil
}
