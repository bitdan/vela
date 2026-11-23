package gitcli

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"vela/internal/application"
)

// readImportedDisk reads bounded raw bytes for binary/metadata operations. It
// does not pass binary data through the editor's UTF-8 preview path.
func readImportedDisk(ctx context.Context, root *os.Root, name string, expected os.FileInfo) ([]byte, string, error) {
	file, err := root.Open(filepath.FromSlash(name))
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() || !os.SameFile(info, expected) || info.Size() > application.MaxImportedBinaryBytes {
		return nil, "", errors.New("binary import target changed or exceeds 2 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(&projectContextReader{ctx: ctx, reader: file}, application.MaxImportedBinaryBytes+1))
	if err != nil {
		return nil, "", err
	}
	after, err := file.Stat()
	if err != nil {
		return nil, "", err
	}
	if len(data) > application.MaxImportedBinaryBytes || int64(len(data)) != after.Size() || info.Size() != after.Size() || info.ModTime() != after.ModTime() || info.Mode() != after.Mode() {
		return nil, "", errors.New("binary target changed while reading")
	}
	return data, diskFileVersion(after, string(data)), ctx.Err()
}

func (g *Executor) readImportedBlob(ctx context.Context, root, name string) ([]byte, error) {
	out, err := g.runInputWithEnvLimit(ctx, root, "", nil, application.MaxImportedBinaryBytes+1, "cat-file", "blob", ":0:"+name)
	if err != nil {
		return nil, err
	}
	if len(out.stdout) > application.MaxImportedBinaryBytes {
		return nil, errors.New("binary index blob exceeds 2 MiB")
	}
	return []byte(out.stdout), nil
}
