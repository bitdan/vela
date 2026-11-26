package gitcli

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"vela/internal/application"
	"vela/internal/domain"
)

func (g *Executor) ReadLineHistory(ctx context.Context, repo domain.Repository, q domain.LineHistoryQuery) (domain.LineHistoryPage, error) {
	var page domain.LineHistoryPage
	ctx, cancel := context.WithTimeout(canonicalHistory(ctx), 45*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return page, err
	}
	if err := application.ValidateLineHistoryQuery(q); err != nil {
		return page, err
	}
	if !validGitPath(q.Path) || repo.Root == "" || repo.Head != q.Revision {
		return page, errors.New("line history requires the current fixed HEAD and a Project file")
	}
	g.write.Lock()
	defer g.write.Unlock()
	verify := func() error {
		common, err := g.read(ctx, repo.Root, "rev-parse", "--path-format=absolute", "--git-common-dir")
		if err != nil {
			return err
		}
		common = strings.TrimSuffix(common, "\n")
		if filepath.Clean(common) != filepath.Clean(repo.CommonDir) {
			return errors.New("repository changed; reopen line history")
		}
		boundary, err := commitLogBoundary(common)
		if err != nil {
			return err
		}
		if boundary != repo.LogSnapshot.Boundary {
			return errors.New("repository ancestry changed; refresh the repository and reopen line history")
		}
		head, err := g.read(ctx, repo.Root, "rev-parse", "--verify", "HEAD")
		if err != nil {
			return err
		}
		if strings.TrimSpace(head) != q.Revision {
			return errors.New("HEAD changed; reopen range history")
		}
		disk, err := readWorkingFile(repo.Root, q.Path)
		if err != nil {
			return err
		}
		version, err := projectFileVersion(repo.Root, q.Path, disk)
		if err != nil {
			return err
		}
		if disk != q.Text || version != q.DiskVersion {
			return ErrStaleFile
		}
		return nil
	}
	if err := verify(); err != nil {
		return page, err
	}
	text, err := g.readBlob(ctx, repo.Root, q.Revision+":"+q.Path)
	if err != nil {
		return page, err
	}
	if text != q.Text {
		return page, errors.New("Project text differs from committed HEAD")
	}
	format := "--format=format:%x00%H%x00%P%x00%an%x00%ae%x00%at%x00%ct%x00%s%x00%b%x00"
	out, err := g.runInputWithEnvLimit(ctx, repo.Root, "", nil, 16<<20, "--literal-pathspecs", "log", "--no-decorate", "--no-color", "--encoding=UTF-8", "--no-ext-diff", "--no-textconv", "--no-notes", "--no-show-signature", format, "--skip="+strconv.Itoa(q.Offset), "-n", strconv.Itoa(q.Limit+1), "-L"+strconv.Itoa(q.Start)+","+strconv.Itoa(q.End)+":"+q.Path, q.Revision, "--")
	if err != nil {
		return page, operationError(ctx, out, err)
	}
	entries, err := parseLineHistory(out.stdout)
	if err != nil {
		return page, err
	}
	if err = verify(); err != nil {
		return page, err
	}
	page.HasMore = len(entries) > q.Limit
	if page.HasMore {
		entries = entries[:q.Limit]
	}
	page.Entries = entries
	return page, ctx.Err()
}
func parseLineHistory(value string) ([]domain.LineHistoryEntry, error) {
	if value == "" {
		return nil, nil
	}
	fields := strings.Split(value, "\x00")
	if fields[0] != "" || (len(fields)-1)%9 != 0 {
		return nil, errors.New("invalid line-history metadata framing")
	}
	var entries []domain.LineHistoryEntry
	seen := map[string]bool{}
	for i := 1; i < len(fields); i += 9 {
		commits, err := parseHistory("\x00"+strings.Join(fields[i:i+8], "\x00")+"\x00", nil)
		if err != nil || len(commits) != 1 || !validObjectID(commits[0].Hash) || seen[commits[0].Hash] {
			return nil, errors.New("invalid line-history commit")
		}
		seen[commits[0].Hash] = true
		hunks, err := application.ProjectLineHistoryPatch(strings.TrimLeft(fields[i+8], "\n"))
		if err != nil {
			return nil, err
		}
		entries = append(entries, domain.LineHistoryEntry{Commit: commits[0], Hunks: hunks})
	}
	return entries, nil
}
