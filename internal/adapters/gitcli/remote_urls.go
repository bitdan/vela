package gitcli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"vela/internal/domain"
)

func parseRemoteURLs(config string) []domain.Remote {
	var remotes []domain.Remote
	index := make(map[string]int)
	for _, row := range nulFields(config) {
		key, address, ok := strings.Cut(row, "\n")
		dot := strings.LastIndexByte(key, '.')
		if !ok || dot <= len("remote.") || !strings.HasPrefix(key, "remote.") {
			continue
		}
		kind := key[dot+1:]
		if kind != "url" && kind != "pushurl" {
			continue
		}
		name := key[len("remote."):dot]
		i, exists := index[name]
		if !exists {
			i = len(remotes)
			index[name] = i
			remotes = append(remotes, domain.Remote{Name: name})
		}
		entry := &remotes[i]
		if !domain.ValidRemoteLocation(address) {
			entry.URLConfigBlocked = true
			address = "[unsupported URL]"
		}
		if kind == "url" {
			entry.FetchURLs = append(entry.FetchURLs, address)
		} else {
			entry.PushURLs = append(entry.PushURLs, address)
		}
	}
	for i := range remotes {
		if len(remotes[i].FetchURLs) > 0 {
			remotes[i].URL = remotes[i].FetchURLs[0]
		}
	}
	return remotes
}

func (g *Executor) editRemoteURLs(ctx context.Context, repo domain.Repository, op domain.RepositoryOperation, path, local string) error {
	if !domain.ValidRemoteURLLists(op.FetchURLs, op.PushURLs) {
		return errors.New("enter 1–32 fetch URLs and up to 32 push URLs without credentials")
	}
	entries := parseRemoteURLs(local)
	if len(entries) != 1 || entries[0].Name != op.Remote || entries[0].URLConfigBlocked {
		return errors.New("edit unsupported URL configurations using Git")
	}
	if err := g.verifyUpdateMetadata(ctx, repo); err != nil {
		return err
	}
	stat, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !stat.Mode().IsRegular() {
		return errors.New("repository config is not a regular file")
	}
	// Hold Git's own config lock while preparing both lists. Git edits the
	// private lock copy, then a single rename publishes the complete config.
	lock := path + ".lock"
	copy, err := os.OpenFile(lock, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	published := false
	defer func() {
		if !published {
			_ = os.Remove(lock)
		}
	}()
	original, readErr := readBoundedRemoteConfig(path)
	var writeErr error
	if readErr == nil {
		_, writeErr = copy.Write(original)
	}
	if err := errors.Join(readErr, writeErr, copy.Close()); err != nil {
		return err
	}
	version, err := g.remoteManagementVersion(ctx, repo.Root)
	if err != nil {
		return err
	}
	if version != repo.RemoteManagementVersion {
		return ErrStaleIndex
	}
	for _, list := range []struct {
		key    string
		values []string
	}{{"url", op.FetchURLs}, {"pushurl", op.PushURLs}} {
		key := "remote." + op.Remote + "." + list.key
		out, err := g.run(ctx, repo.Root, "config", "--file", lock, "--unset-all", key)
		if err != nil && (out.exitCode != 5 || ctx.Err() != nil) {
			return err
		}
		for _, value := range list.values {
			if _, err := g.run(ctx, repo.Root, "config", "--file", lock, "--add", "--", key, value); err != nil {
				return err
			}
		}
	}
	if now, err := readBoundedRemoteConfig(path); err != nil {
		return err
	} else if !bytes.Equal(original, now) {
		return ErrStaleIndex
	}
	if err := os.Chmod(lock, stat.Mode().Perm()); err != nil {
		return err
	}
	fd, err := os.OpenFile(lock, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	if err := errors.Join(fd.Sync(), fd.Close()); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(lock, path); err != nil {
		return err
	}
	published = true
	return nil
}

func readBoundedRemoteConfig(path string) ([]byte, error) {
	fd, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	const limit = 4 << 20
	data, err := io.ReadAll(io.LimitReader(fd, limit+1))
	if err == nil && len(data) > limit {
		return nil, errors.New("repository config exceeds 4 MiB")
	}
	return data, err
}
