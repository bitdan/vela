package gitcli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"vela/internal/domain"
)

func (g *Executor) ReadRepository(ctx context.Context, directory string) (domain.Repository, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var repo domain.Repository
	root, err := g.read(ctx, directory, "rev-parse", "--show-toplevel")
	if err != nil {
		return repo, err
	}
	repo.Root = filepath.Clean(strings.TrimSuffix(root, "\n"))
	repo.Name = filepath.Base(repo.Root)
	index, err := g.readCheckout(ctx, &repo)
	if err != nil {
		return repo, err
	}
	gitDir, err := g.gitDirectory(ctx, repo.Root)
	if err != nil {
		return repo, err
	}
	repo.GitDir = gitDir
	repo.Bisect, err = g.readBisect(ctx, repo.Root, gitDir)
	if err != nil {
		return repo, err
	}
	common, err := g.read(ctx, repo.Root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return repo, err
	}
	repo.CommonDir = filepath.Clean(strings.TrimSuffix(common, "\n"))
	repo.Worktrees, repo.WorktreesVersion, err = g.readWorktrees(ctx, repo.Root)
	if err != nil {
		return repo, err
	}
	repo.MergeHeads, err = mergeHeadsAt(gitDir)
	if err != nil {
		return repo, err
	}
	repo.Rebase, err = rebaseAt(gitDir)
	if err != nil {
		return repo, err
	}
	repo.Sequencer, err = sequencerAt(gitDir)
	if err != nil {
		return repo, err
	}
	repo.Conflicts = conflictsFromIndex(index)
	repo.Stashes, repo.StashVersion, err = g.readStashes(ctx, repo.Root)
	if err != nil {
		return repo, err
	}
	repo.WorktreeVersion, err = g.worktreeVersion(ctx, repo.Root)
	if err != nil {
		return repo, err
	}
	repo.Staged, repo.Working, err = g.readStatus(ctx, repo.Root)
	if err != nil {
		return repo, err
	}
	workingPaths := filesByPath(repo.Working)
	for _, conflict := range repo.Conflicts {
		if _, found := workingPaths[conflict.Path]; !found {
			repo.Working = append(repo.Working, domain.File{Path: conflict.Path, Status: "U"})
		}
	}
	paths, err := g.read(ctx, repo.Root, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--deduplicate")
	if err != nil {
		return repo, err
	}
	deleted, err := g.read(ctx, repo.Root, "ls-files", "-z", "--deleted", "--deduplicate")
	if err != nil {
		return repo, err
	}
	seen := make(map[string]bool)
	for _, file := range nulFields(deleted) {
		seen[file] = true
	}
	statuses := make(map[string]string)
	for _, file := range append(append([]domain.File(nil), repo.Staged...), repo.Working...) {
		statuses[file.Path] = file.Status
	}
	for _, file := range nulFields(paths) {
		if file != "" && !seen[file] {
			repo.Project = append(repo.Project, domain.File{Path: file, Status: statuses[file]})
			seen[file] = true
		}
	}
	refOutput, err := g.read(ctx, repo.Root, "for-each-ref", "--sort=refname", "--format=%(refname)%00%(objectname)%00%(*objectname)%00%(symref)%00%(upstream:remotename)%00%(upstream:remoteref)", "refs/heads", "refs/remotes", "refs/tags")
	if err != nil {
		return repo, err
	}
	refs := make(map[string][]string)
	repo.RemoteBranches = make(map[string]bool)
	repo.Refs = make(map[string]string)
	repo.Upstreams = make(map[string]domain.Upstream)
	for _, row := range strings.Split(strings.TrimSuffix(refOutput, "\n"), "\n") {
		fields := strings.Split(row, "\x00")
		if len(fields) != 6 || fields[3] != "" {
			continue
		}
		name, hash := fields[0], fields[1]
		repo.Refs[name] = hash
		switch {
		case strings.HasPrefix(name, "refs/heads/"):
			name = strings.TrimPrefix(name, "refs/heads/")
			if fields[4] != "" && strings.HasPrefix(fields[5], "refs/heads/") {
				repo.Upstreams[name] = domain.Upstream{Remote: fields[4], Branch: strings.TrimPrefix(fields[5], "refs/heads/")}
			}
		case strings.HasPrefix(name, "refs/remotes/"):
			name = strings.TrimPrefix(name, "refs/remotes/")
			if repo.Refs["refs/heads/"+name] != "" {
				name = fields[0]
			}
			repo.RemoteBranches[name] = true
		}
		if fields[2] != "" {
			hash = fields[2]
		}
		repo.Branches = append(repo.Branches, name)
		refs[hash] = append(refs[hash], name)
	}
	remoteConfig, repoVersion, err := g.readRemoteConfig(ctx, repo.Root)
	if err != nil {
		return repo, err
	}
	repo.RemoteVersion = repoVersion
	for _, row := range nulFields(remoteConfig) {
		key, address, ok := strings.Cut(row, "\n")
		if !ok || !strings.HasSuffix(key, ".url") {
			continue
		}
		if parsed, err := url.Parse(address); err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") {
			parsed.User, parsed.RawQuery, parsed.Fragment = nil, "", ""
			address = parsed.String()
		}
		repo.Remotes = append(repo.Remotes, domain.Remote{Name: strings.TrimSuffix(strings.TrimPrefix(key, "remote."), ".url"), URL: address})
	}
	logArgs := []string{"log", "--branches", "--remotes", "--tags", "--topo-order", "-n", "10000", "--encoding=UTF-8",
		"--format=tformat:%x00%H%x00%P%x00%an%x00%ae%x00%at%x00%ct%x00%s%x00%b",
		"--name-status", "-z", "--root", "--diff-merges=first-parent", "--no-ext-diff", "--no-textconv", "-M", "--no-notes", "--no-show-signature"}
	if repo.Head != "" {
		logArgs = append(logArgs, repo.Head)
		repo.HeadMessage, err = g.read(ctx, repo.Root, "show", "-s", "--format=format:%B", "--encoding=UTF-8",
			"--no-notes", "--no-show-signature", repo.Head, "--")
		if err != nil {
			return repo, err
		}
		if repo.CurrentBranch == "Detached HEAD" {
			refs[repo.Head] = append(refs[repo.Head], repo.CurrentBranch)
		}
	}
	log, err := g.read(ctx, repo.Root, append(logArgs, "--")...)
	if err != nil {
		return repo, err
	}
	repo.History, err = parseHistory(log, refs)
	return repo, err
}

func (g *Executor) readStatus(ctx context.Context, root string) (staged, working []domain.File, err error) {
	value, err := g.read(ctx, root, "-c", "core.fsmonitor=false", "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=all", "--renames")
	if err != nil {
		return nil, nil, err
	}
	return parseStatus(value)
}

func (g *Executor) read(ctx context.Context, directory string, args ...string) (string, error) {
	out, err := g.run(ctx, directory, args...)
	if err != nil && ctx.Err() == nil && out.exitCode >= 0 {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(out.stderr))
	}
	return out.stdout, err
}

func nulFields(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(value, "\x00"), "\x00")
}

func parseStatus(value string) (staged, working []domain.File, err error) {
	rows := nulFields(value)
	for i := 0; i < len(rows); i++ {
		row := rows[i]
		if len(row) < 4 || row[2] != ' ' {
			return nil, nil, errors.New("invalid Git status record")
		}
		file := domain.File{Path: row[3:]}
		x, y := row[0], row[1]
		if strings.ContainsAny(row[:2], "RC") {
			i++
			if i == len(rows) {
				return nil, nil, errors.New("missing original path in Git status")
			}
			file.OldPath = rows[i]
		}
		if row[:2] == "??" {
			file.Status = "?"
			working = append(working, file)
			continue
		}
		if x == 'U' || y == 'U' || row[:2] == "AA" || row[:2] == "DD" {
			file.Status = "U"
			working = append(working, file)
			continue
		}
		if x != ' ' {
			stagedFile := file
			stagedFile.Status = string(x)
			staged = append(staged, stagedFile)
		}
		if y != ' ' {
			if y != 'R' && y != 'C' {
				file.OldPath = ""
			}
			file.Status = string(y)
			working = append(working, file)
		}
	}
	return staged, working, nil
}
