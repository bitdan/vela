package gitcli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"vela/internal/domain"
)

func (g *Executor) manageRemote(ctx context.Context, repo domain.Repository, op domain.RepositoryOperation) error {
	if !domain.ValidRemoteName(op.Remote) {
		return errors.New("invalid remote name")
	}
	if op.Kind != domain.RemoveRemote && !domain.ValidRemoteLocation(op.Target) {
		return errors.New("enter a supported remote URL without credentials or an absolute local path")
	}
	_, version, err := g.readRemoteConfig(ctx, repo.Root)
	if err != nil {
		return err
	}
	if version != repo.RemoteVersion {
		return ErrStaleIndex
	}
	managementVersion, err := g.remoteManagementVersion(ctx, repo.Root)
	if err != nil {
		return err
	}
	if repo.RemoteManagementVersion == "" || managementVersion != repo.RemoteManagementVersion {
		return ErrStaleIndex
	}
	names, err := g.read(ctx, repo.Root, "remote")
	if err != nil {
		return err
	}
	exists := slices.Contains(strings.Split(strings.TrimSpace(names), "\n"), op.Remote)
	if op.Kind == domain.AddRemote {
		if exists {
			return errors.New("a remote with this name already exists")
		}
		return g.runOperation(ctx, repo.Root, "remote", "add", "--", op.Remote, op.Target)
	}
	if !exists {
		return errors.New("select an existing remote")
	}
	scopes, err := g.read(ctx, repo.Root, "config", "--null", "--show-scope", "--show-origin", "--get-regexp", "^remote\\."+regexp.QuoteMeta(op.Remote)+"\\.")
	if err != nil {
		return err
	}
	scopeFields := nulFields(scopes)
	configPath, err := g.read(ctx, repo.Root, "rev-parse", "--path-format=absolute", "--git-path", "config")
	if err != nil {
		return err
	}
	configPath = filepath.Clean(strings.TrimSuffix(configPath, "\n"))
	for i := 0; i < len(scopeFields); i += 3 {
		if scopeFields[i] != "local" {
			return errors.New("this remote is inherited; manage it in its original Git configuration")
		}
		if i+2 >= len(scopeFields) || !strings.HasPrefix(scopeFields[i+1], "file:") {
			return errors.New("unsupported remote configuration origin")
		}
		origin := strings.TrimPrefix(scopeFields[i+1], "file:")
		if !filepath.IsAbs(origin) {
			origin = filepath.Join(repo.Root, origin)
		}
		if filepath.Clean(origin) != configPath {
			return errors.New("this remote is included from another config file; manage it in its original Git configuration")
		}
	}
	// Do not mutate an inherited/global remote through a local management UI.
	local, err := g.run(ctx, repo.Root, "config", "--local", "--null", "--get-regexp", "^remote\\."+regexp.QuoteMeta(op.Remote)+"\\.")
	if err != nil {
		if local.exitCode == 1 && ctx.Err() == nil {
			return errors.New("this remote is inherited; manage it in its original Git configuration")
		}
		return err
	}
	if op.Kind == domain.EditRemote {
		urls, err := g.read(ctx, repo.Root, "config", "--null", "--get-all", "remote."+op.Remote+".url")
		if err != nil {
			return err
		}
		if len(nulFields(urls)) != 1 {
			return errors.New("this remote has multiple URLs; edit it using Git")
		}
		return g.runOperation(ctx, repo.Root, "remote", "set-url", "--", op.Remote, op.Target)
	}
	fetch, err := g.read(ctx, repo.Root, "config", "--null", "--get-all", "remote."+op.Remote+".fetch")
	if err != nil {
		return err
	}
	for _, spec := range nulFields(fetch) {
		if strings.TrimPrefix(spec, "+") != "refs/heads/*:refs/remotes/"+op.Remote+"/*" {
			return errors.New("this remote has custom fetch mappings; remove it using Git after reviewing their refs")
		}
	}
	// Removing a remote also removes its tracking refs. Validate their exact
	// identities, including refs created or advanced after the confirmation.
	actual, err := g.read(ctx, repo.Root, "for-each-ref", "--format=%(refname)%00%(objectname)", "refs/remotes/"+op.Remote+"/")
	if err != nil {
		return err
	}
	expected := make(map[string]string)
	for ref, oid := range repo.Refs {
		if strings.HasPrefix(ref, "refs/remotes/"+op.Remote+"/") {
			expected[ref] = oid
		}
	}
	live := make(map[string]string)
	for _, row := range strings.Split(strings.TrimSuffix(actual, "\n"), "\n") {
		if name, oid, ok := strings.Cut(row, "\x00"); ok {
			live[name] = oid
		}
	}
	if !maps.Equal(expected, live) {
		return ErrStaleIndex
	}
	return g.runOperation(ctx, repo.Root, "remote", "remove", "--", op.Remote)
}

// Include scope and origin as well as every remote setting and branch tracking
// setting: git remote remove may delete more than the transport URL/refspec.
func (g *Executor) remoteManagementVersion(ctx context.Context, root string) (string, error) {
	out, err := g.run(ctx, root, "config", "--null", "--show-scope", "--show-origin", "--get-regexp", `^(remote\..*|branch\..*)$`)
	if err != nil && (out.exitCode != 1 || ctx.Err() != nil) {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(out.stdout))), nil
}
