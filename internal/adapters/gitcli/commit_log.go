package gitcli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"vela/internal/domain"
)

const commitLogPageSize = 500

// ReadCommitLog walks only the frozen OIDs, so ref changes do not shift offsets.
// Output and page size are bounded independently; an oversized page fails without
// changing the snapshot or silently dropping commit/file metadata.
func (g *Executor) ReadCommitLog(ctx context.Context, root string, snapshot domain.CommitLogSnapshot, offset, limit int) (domain.CommitLogPage, error) {
	var page domain.CommitLogPage
	if offset < 0 || limit < 1 || limit > commitLogPageSize {
		return page, errors.New("invalid commit log page bounds")
	}
	for _, tip := range snapshot.Tips {
		if !validObjectID(tip) {
			return page, errors.New("commit log requires fixed commit OIDs")
		}
	}
	if err := ctx.Err(); err != nil {
		return page, err
	}
	if len(snapshot.Tips) == 0 {
		return page, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	common, err := g.read(ctx, root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return page, err
	}
	common = strings.TrimSuffix(common, "\n")
	if err := checkLogBoundary(common, snapshot.Boundary); err != nil {
		return page, err
	}
	args := []string{"log", "--stdin", "--topo-order", "--no-decorate", "--no-color", "-n", strconv.Itoa(limit + 1), "--skip=" + strconv.Itoa(offset), "--encoding=UTF-8",
		"--format=tformat:%x00%H%x00%P%x00%an%x00%ae%x00%at%x00%ct%x00%s%x00%b", "--name-status", "-z", "--root", "--diff-merges=first-parent", "--no-ext-diff", "--no-textconv", "-M", "--no-notes", "--no-show-signature", "--"}
	out, err := g.runInputWithEnvLimit(ctx, root, strings.Join(snapshot.Tips, "\n")+"\n", []string{"GIT_NO_REPLACE_OBJECTS=1"}, 16<<20, args...)
	if err != nil {
		return page, err
	}
	if err := checkLogBoundary(common, snapshot.Boundary); err != nil {
		return page, err
	}
	commits, err := parseHistory(out.stdout, snapshot.Refs)
	if err != nil {
		return page, err
	}
	page.HasMore = len(commits) > limit
	if page.HasMore {
		commits = commits[:limit]
		// Drop any edge to the lookahead record, which is not loaded yet.
		domain.ReconnectCommitLog(commits)
	}
	page.Commits = commits
	return page, nil
}

func checkLogBoundary(common, expected string) error {
	current, err := commitLogBoundary(common)
	if err != nil {
		return err
	}
	if current != expected {
		return errors.New("repository ancestry changed; refresh the Log before loading more commits")
	}
	return nil
}

// Shallow boundaries and legacy grafts can change ancestry without moving tips.
// Check their bytes before and after the query rather than accepting shifted pages.
func commitLogBoundary(common string) (string, error) {
	h := sha256.New()
	for _, name := range []string{"shallow", "info/grafts"} {
		data, err := os.ReadFile(filepath.Join(common, name))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("read commit log ancestry boundary: %w", err)
		}
		fmt.Fprintf(h, "%s:%d:", name, len(data))
		h.Write(data)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
