package gitcli

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"vela/internal/application"
	"vela/internal/domain"
)

// ReadMissingBranchComparison captures identities without enumerating a tree diff.
func (g *Executor) ReadMissingBranchComparison(ctx context.Context, repo domain.Repository, ref string) (domain.RevisionComparison, error) {
	var result domain.RevisionComparison
	ctx, cancel := context.WithTimeout(canonicalHistory(ctx), 30*time.Second)
	defer cancel()
	if _, _, err := g.verifyCheckout(ctx, repo); err != nil {
		return result, err
	}
	left, err := g.resolveSource(ctx, repo, ref)
	if err != nil {
		return result, err
	}
	if repo.Head == "" {
		return result, errors.New("create a commit before comparing branches")
	}
	result.Left, result.Right = left, repo.Head
	result.AncestryBoundary, err = commitLogBoundary(repo.CommonDir)
	if err != nil {
		return result, err
	}
	_, _, err = g.verifyCheckout(ctx, repo)
	return result, err
}

func (g *Executor) ReadBranchComparison(ctx context.Context, repo domain.Repository, ref string, working bool) (domain.RevisionComparison, error) {
	var result domain.RevisionComparison
	ctx, cancel := context.WithTimeout(canonicalHistory(ctx), 30*time.Second)
	defer cancel()
	if _, _, err := g.verifyCheckout(ctx, repo); err != nil {
		return result, err
	}
	left, err := g.resolveSource(ctx, repo, ref)
	if err != nil {
		return result, err
	}
	result.Left, result.Right = left, repo.Head
	if repo.Head == "" {
		return result, errors.New("create a commit before comparing branches")
	}
	args := []string{"diff", "--name-status", "-z", "--no-ext-diff", "--no-textconv", "--no-color", "--no-relative", "--find-renames=50%", left}
	if working {
		if repo.ResolvingConflicts() {
			return result, errors.New("resolve conflicts before comparing working files")
		}
		if err := g.rejectHiddenIndexFlags(ctx, repo.Root); err != nil {
			return result, err
		}
		if err := g.verifyWorktree(ctx, repo); err != nil {
			return result, err
		}
		result.Right = ""
		result.WorktreeVersion = repo.WorktreeVersion
	} else {
		args = append(args, repo.Head)
	}
	args = append(args, "--")
	out, err := g.runInputWithEnvLimit(ctx, repo.Root, "", nil, 2<<20, args...)
	if err != nil {
		return result, err
	}
	fields := nulFields(out.stdout)
	for i := 0; i < len(fields); {
		status := fields[i]
		i++
		if status == "" || i >= len(fields) {
			return result, errors.New("invalid branch diff record")
		}
		file := domain.File{Status: status[:1], Path: fields[i]}
		i++
		if file.Status == "R" || file.Status == "C" {
			if i >= len(fields) {
				return result, errors.New("invalid branch rename")
			}
			file.OldPath, file.Path = file.Path, fields[i]
			i++
		}
		if !validGitPath(file.Path) || file.OldPath != "" && !validGitPath(file.OldPath) || file.Status == "U" {
			return result, errors.New("branch comparison contains unsupported paths or conflicts")
		}
		result.Files = append(result.Files, file)
		if len(result.Files) > 10000 {
			return result, errors.New("branch comparison exceeds 10000 files")
		}
	}
	result.AncestryBoundary, err = commitLogBoundary(repo.CommonDir)
	if err != nil {
		return result, err
	}
	if _, _, err := g.verifyCheckout(ctx, repo); err != nil {
		return result, err
	}
	if working {
		if err := g.verifyWorktree(ctx, repo); err != nil {
			return result, err
		}
		if err := g.rejectHiddenIndexFlags(ctx, repo.Root); err != nil {
			return result, err
		}
	}
	return result, nil
}

// ReadComparisonCommits reads HEAD..selected using complete, frozen Git ancestry.
func (g *Executor) ReadComparisonCommits(ctx context.Context, root string, comparison domain.RevisionComparison, offset, limit int) (domain.CommitLogPage, error) {
	var page domain.CommitLogPage
	if !validObjectID(comparison.Left) || !validObjectID(comparison.Right) || offset < 0 || limit < 1 || limit > 100 {
		return page, errors.New("invalid missing-commit query")
	}
	if err := application.ValidateComparisonLogFilter(comparison.Filter); err != nil {
		return page, err
	}
	ctx, cancel := context.WithTimeout(canonicalHistory(ctx), 30*time.Second)
	defer cancel()
	common, err := g.read(ctx, root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return page, err
	}
	common = strings.TrimSuffix(common, "\n")
	if err := checkLogBoundary(common, comparison.AncestryBoundary); err != nil {
		return page, err
	}
	args := comparisonLogArgs(comparison, offset, limit)
	out, err := g.runInputWithEnvLimit(ctx, root, "", nil, 4<<20, args...)
	if err != nil {
		return page, err
	}
	if err := checkLogBoundary(common, comparison.AncestryBoundary); err != nil {
		return page, err
	}
	commits, err := parseHistory(out.stdout, nil)
	if err != nil {
		return page, err
	}
	page.HasMore = len(commits) > limit
	if page.HasMore {
		commits = commits[:limit]
		domain.ReconnectCommitLog(commits)
	}
	page.Commits = commits
	return page, nil
}

func comparisonLogArgs(comparison domain.RevisionComparison, offset, limit int) []string {
	args := []string{"--literal-pathspecs", "log", "-z", "--topo-order", "--no-decorate", "--no-color", "--encoding=UTF-8", "--no-notes", "--no-show-signature", "--no-use-mailmap", "--fixed-strings", "--regexp-ignore-case", "-n", strconv.Itoa(limit + 1), "--skip=" + strconv.Itoa(offset), "--format=tformat:%x00%H%x00%P%x00%an%x00%ae%x00%at%x00%ct%x00%s%x00%b"}
	filter := comparison.Filter
	if filter.Message != "" {
		args = append(args, "--grep="+filter.Message)
	}
	if filter.Author != "" {
		args = append(args, "--author="+filter.Author)
	}
	if filter.Since != "" {
		args = append(args, "--since-as-filter="+filter.Since+"T00:00:00Z")
	}
	if filter.Until != "" {
		args = append(args, "--until="+filter.Until+"T23:59:59Z")
	}
	if filter.Path != "" {
		args = append(args, "--full-history")
	}
	args = append(args, comparison.Left, "^"+comparison.Right, "--")
	if filter.Path != "" {
		args = append(args, filter.Path)
	}
	return args
}

func (g *Executor) readComparisonFile(ctx context.Context, root, source string, file domain.File) (domain.File, error) {
	left, right, ok := domain.ParseComparisonSource(source)
	if !ok {
		return file, errors.New("invalid comparison identity")
	}
	var err error
	before := ""
	if right == "" {
		before, err = g.worktreeVersion(ctx, root)
		if err != nil {
			return file, err
		}
		if expected := domain.ComparisonWorktreeVersion(source); expected != "" && expected != before {
			return file, ErrStaleIndex
		}
	}
	oldPath := file.Path
	if file.OldPath != "" {
		oldPath = file.OldPath
	}
	file.Old, file.New, file.DiskVersion = "", "", ""
	if file.Status != "A" {
		file.Old, err = g.readBlob(ctx, root, left+":"+oldPath)
		if err != nil {
			return file, err
		}
	}
	if file.Status != "D" {
		if right == "" {
			file.New, err = readWorkingFile(root, file.Path)
		} else {
			file.New, err = g.readBlob(ctx, root, right+":"+file.Path)
		}
		if err != nil {
			return file, err
		}
	}
	if right == "" {
		after, err := g.worktreeVersion(ctx, root)
		if err != nil {
			return file, err
		}
		if before != after {
			return file, ErrStaleIndex
		}
	}
	return file, nil
}
