package gitcli

import (
	"context"
	"errors"
	"maps"
	"path/filepath"
	"strings"

	"vela/internal/domain"
)

func (g *Executor) renameRemote(ctx context.Context, repo domain.Repository, op domain.RepositoryOperation, names, configPath string) error {
	if !domain.ValidRemoteName(op.Target) {
		return errors.New("enter a valid new remote name")
	}
	for _, name := range strings.Fields(names) {
		if strings.EqualFold(name, op.Target) {
			return errors.New("a remote with this name already exists")
		}
	}
	if err := g.verifyUpdateMetadata(ctx, repo); err != nil {
		return err
	}
	// Refuse upstream/push references in a config Git cannot rewrite here.
	uses, err := g.run(ctx, repo.Root, "config", "--null", "--show-scope", "--show-origin", "--get-regexp", `^(branch\..*\.(remote|pushremote)|remote\.pushdefault)$`)
	if err != nil && (uses.exitCode != 1 || ctx.Err() != nil) {
		return err
	}
	fields := nulFields(uses.stdout)
	if len(fields)%3 != 0 {
		return errors.New("invalid tracking configuration")
	}
	for i := 0; i < len(fields); i += 3 {
		_, value, _ := strings.Cut(fields[i+2], "\n")
		if value != op.Remote {
			continue
		}
		origin, file := strings.CutPrefix(fields[i+1], "file:")
		if !filepath.IsAbs(origin) {
			origin = filepath.Join(repo.Root, origin)
		}
		if fields[i] != "local" || !file || filepath.Clean(origin) != configPath {
			return errors.New("tracking configuration is inherited; rename this remote using Git")
		}
	}
	old, next := "refs/remotes/"+op.Remote, "refs/remotes/"+op.Target
	expected := make(map[string]string)
	for ref, oid := range repo.Refs {
		if strings.HasPrefix(ref, old+"/") {
			expected[ref] = oid
		}
	}
	for ref, sym := range repo.RemoteSymrefs {
		if strings.HasPrefix(ref, old+"/") {
			expected[ref] = sym
		}
	}
	out, err := g.read(ctx, repo.Root, "for-each-ref", "--format=%(refname)%00%(objectname)%00%(symref)", "refs/remotes/")
	if err != nil {
		return err
	}
	live := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		ref := strings.Split(line, "\x00")
		if len(ref) != 3 {
			continue
		}
		lower := strings.ToLower(ref[0])
		if lower == strings.ToLower(next) || strings.HasPrefix(lower, strings.ToLower(next)+"/") {
			return errors.New("tracking references already occupy the destination remote")
		}
		if ref[0] == old {
			return ErrStaleIndex
		}
		if strings.HasPrefix(ref[0], old+"/") {
			if ref[2] == "" {
				live[ref[0]] = ref[1]
			} else {
				live[ref[0]] = ref[2]
			}
		}
	}
	if !maps.Equal(live, expected) {
		return ErrStaleIndex
	}
	if version, err := g.remoteManagementVersion(ctx, repo.Root); err != nil {
		return err
	} else if version != repo.RemoteManagementVersion {
		return ErrStaleIndex
	}
	return g.runOperation(ctx, repo.Root, "remote", "rename", "--", op.Remote, op.Target)
}
