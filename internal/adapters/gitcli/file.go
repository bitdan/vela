package gitcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"vela/internal/domain"
)

const maxPreviewBytes = 2 << 20

func (g *Executor) ReadFile(ctx context.Context, root, source string, file domain.File) (domain.File, error) {
	ctx = canonicalHistory(ctx)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if !validGitPath(file.Path) || (file.OldPath != "" && !validGitPath(file.OldPath)) {
		return file, errors.New("invalid repository file path")
	}
	if strings.HasPrefix(source, "compare:") {
		return g.readComparisonFile(ctx, root, source, file)
	}
	if strings.HasPrefix(source, "shelf:") || strings.HasPrefix(source, "shelf-index:") {
		return g.readShelfFile(ctx, root, source, file)
	}
	if source == "merge-conflict" {
		return g.readMergeDiff(ctx, root, file)
	}
	if revision, ok := strings.CutPrefix(source, "revision:"); ok {
		if !validObjectID(revision) {
			return file, errors.New("invalid Git commit ID")
		}
		file.Old, file.DiskVersion = "", ""
		var err error
		file.New, err = g.readBlob(ctx, root, revision+":"+file.Path)
		return file, err
	}
	if mode, revision, ok := strings.Cut(source, ":"); ok && (mode == "local" || mode == "before") {
		if !validObjectID(revision) {
			return file, errors.New("invalid Git commit ID")
		}
		oldPath := file.Path
		if file.OldPath != "" {
			oldPath = file.OldPath
		}
		var err error
		if mode == "local" && file.Status != "D" {
			file.Old, err = g.readBlob(ctx, root, revision+":"+file.Path)
		} else if mode == "before" && file.Status != "A" {
			file.Old, err = g.readBlob(ctx, root, revision+"^1:"+oldPath)
		}
		if err != nil {
			return file, err
		}
		file.New, err = readWorkingFile(root, file.Path)
		if errors.Is(err, os.ErrNotExist) {
			err = nil
		}
		return file, err
	}
	if source == "project" {
		text, err := readWorkingFile(root, file.Path)
		file.New = text
		if err == nil {
			file.DiskVersion, err = projectFileVersion(root, file.Path, text)
		}
		return file, err
	}
	if file.Status == "U" {
		return file, errors.New("this file has unresolved conflicts")
	}
	oldPath := file.Path
	if file.OldPath != "" {
		oldPath = file.OldPath
	}
	var before, after string
	switch source {
	case "working":
		before = ":0:" + oldPath
	case "staged":
		before, after = "HEAD:"+oldPath, ":0:"+file.Path
	default:
		if !validObjectID(source) {
			return file, errors.New("invalid Git commit ID")
		}
		before, after = source+"^1:"+oldPath, source+":"+file.Path
	}
	var err error
	if file.Status != "A" && file.Status != "?" {
		if file.Old, err = g.readBlob(ctx, root, before); err != nil {
			return file, err
		}
	}
	if file.Status == "D" {
		return file, nil
	}
	if source == "working" {
		file.New, err = readWorkingFile(root, file.Path)
	} else {
		file.New, err = g.readBlob(ctx, root, after)
	}
	return file, err
}

func validGitPath(path string) bool {
	if !filepath.IsLocal(filepath.FromSlash(path)) || path == "." {
		return false
	}
	if filepath.Separator == '\\' && strings.ContainsRune(path, '\\') {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if part == ".." || part == "." || part == "" {
			return false
		}
		if filepath.Separator == '\\' {
			part = strings.TrimRight(part, " .")
		}
		if strings.EqualFold(part, ".git") {
			return false
		}
	}
	return true
}

func validObjectID(id string) bool {
	return (len(id) == 40 || len(id) == 64) && strings.Trim(id, "0123456789abcdef") == ""
}

func (g *Executor) readBlob(ctx context.Context, root, object string) (string, error) {
	size, err := g.read(ctx, root, "cat-file", "-s", object)
	if err != nil {
		return "", err
	}
	length, err := strconv.Atoi(strings.TrimSpace(size))
	if err != nil {
		return "", fmt.Errorf("invalid Git object size: %w", err)
	}
	if length > maxPreviewBytes {
		return "", errors.New("file exceeds the 2 MiB preview limit")
	}
	text, err := g.read(ctx, root, "cat-file", "blob", object)
	if err != nil {
		return "", err
	}
	return previewText(text)
}

func readWorkingFile(root, path string) (string, error) {
	dir, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer dir.Close()
	for parent := filepath.Dir(path); parent != "."; parent = filepath.Dir(parent) {
		info, err := dir.Lstat(parent)
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return "", errors.New("file is under a non-directory or symbolic link")
		}
	}
	info, err := dir.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		text, err := dir.Readlink(path)
		if err != nil {
			return "", err
		}
		return previewText(text)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("only regular files can be previewed")
	}
	file, err := dir.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxPreviewBytes+1))
	if err != nil {
		return "", err
	}
	return previewText(string(data))
}

func previewText(text string) (string, error) {
	if len(text) > maxPreviewBytes {
		return "", errors.New("file exceeds the 2 MiB preview limit")
	}
	if strings.ContainsRune(text, 0) || !utf8.ValidString(text) {
		return "", errors.New("binary or non-UTF-8 file preview is unavailable")
	}
	return text, nil
}
