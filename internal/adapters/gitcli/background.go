package gitcli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"vela/internal/application"
	"vela/internal/domain"
)

func (g *Executor) CheckRepositoryRemote(ctx context.Context, repo domain.Repository, policy application.BackgroundRemotePolicy) (application.BackgroundRemoteResult, error) {
	var result application.BackgroundRemoteResult
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	policy.Network.NonInteractive = true
	ctx = withNetworkPolicy(ctx, policy.Network)
	g.write.Lock()
	defer g.write.Unlock()
	if _, _, err := g.verifyCheckout(ctx, repo); err != nil {
		return result, err
	}
	if policy.AutoFetch {
		seen := map[string]bool{}
		for _, remote := range repo.Remotes {
			if seen[remote.Name] {
				continue
			}
			seen[remote.Name] = true
			if len(seen) > 16 {
				return result, errors.New("automatic fetch supports at most 16 remotes; fetch additional remotes manually")
			}
			if err := g.verifyRemote(ctx, repo, remote.Name, false); err != nil {
				return result, err
			}
			supported, err := g.backgroundRemoteSupported(ctx, repo.Root, remote.Name)
			if err != nil {
				return result, err
			}
			if !supported {
				result.Skipped = append(result.Skipped, remote.Name)
				continue
			}
			if err := g.fetchBranches(ctx, repo.Root, remote.Name, "*", policy.Network); err != nil {
				return result, fmt.Errorf("automatic fetch stopped at %s: %w", remote.Name, err)
			}
			result.Fetched = append(result.Fetched, remote.Name)
		}
		result.Message = fmt.Sprintf("Automatically fetched %d remotes", len(result.Fetched))
		if len(result.Skipped) > 0 {
			result.Message += " · SSH/custom transports require manual Fetch: " + strings.Join(result.Skipped, ", ")
		}
		return result, nil
	}
	if !policy.CheckIncoming {
		return result, nil
	}
	upstream := repo.Upstreams[repo.CurrentBranch]
	if upstream.Remote == "" || upstream.Remote == "." || upstream.Branch == "" {
		result.Message = "Incoming check: no remote upstream for the current branch"
		return result, nil
	}
	if err := g.verifyRemote(ctx, repo, upstream.Remote, false); err != nil {
		return result, err
	}
	supported, err := g.backgroundRemoteSupported(ctx, repo.Root, upstream.Remote)
	if err != nil {
		return result, err
	}
	if !supported {
		result.Skipped = []string{upstream.Remote}
		result.Message = "Automatic incoming checks skip SSH/custom transports; use manual Fetch"
		return result, nil
	}
	if err := g.checkBranchName(ctx, repo.Root, upstream.Branch); err != nil {
		return result, err
	}
	ref := "refs/heads/" + upstream.Branch
	out, err := g.runInputWithEnvLimit(ctx, repo.Root, "", nil, 64<<10, "ls-remote", "--exit-code", "--heads", "--refs", "--", upstream.Remote, ref)
	if err != nil {
		if out.exitCode == 2 && ctx.Err() == nil {
			result.Changed, result.Message = true, "Remote upstream is missing; Fetch to review tracking references"
			return result, nil
		}
		return result, operationError(ctx, out, err)
	}
	rows := strings.Split(strings.TrimSuffix(out.stdout, "\n"), "\n")
	if len(rows) != 1 {
		return result, errors.New("remote returned an ambiguous upstream reference")
	}
	oid, name, ok := strings.Cut(rows[0], "\t")
	if !ok || name != ref || !validObjectID(oid) {
		return result, errors.New("invalid remote upstream response")
	}
	result.Changed = oid != repo.Refs["refs/remotes/"+upstream.Remote+"/"+upstream.Branch]
	result.Message = "Remote upstream matches the last fetched reference"
	if result.Changed {
		result.Message = "Remote upstream changed; Fetch to inspect incoming or rewritten history"
	}
	return result, nil
}

// Resolve insteadOf before classifying the transport. Do not overwrite users'
// SSH/Plink configuration with an invented ssh command merely to suppress UI.
func (g *Executor) backgroundRemoteSupported(ctx context.Context, root, remote string) (bool, error) {
	vcs, err := g.run(ctx, root, "config", "--get", "remote."+remote+".vcs")
	if err != nil && (vcs.exitCode != 1 || ctx.Err() != nil) {
		return false, err
	}
	if strings.TrimSpace(vcs.stdout) != "" {
		return false, nil
	}
	out, err := g.runInputWithEnvLimit(ctx, root, "", nil, 64<<10, "remote", "get-url", "--all", remote)
	if err != nil {
		return false, errors.New("could not resolve the remote transport for automatic operation")
	}
	for _, raw := range strings.Split(strings.TrimSuffix(out.stdout, "\n"), "\n") {
		if raw == "" {
			return false, nil
		}
		if !strings.Contains(raw, "://") && (filepath.IsAbs(raw) || !strings.Contains(raw, ":")) {
			continue
		}
		location, err := url.Parse(raw)
		if err != nil {
			return false, nil
		}
		switch strings.ToLower(location.Scheme) {
		case "http", "https", "git", "file":
		default:
			return false, nil
		}
	}
	return true, nil
}
