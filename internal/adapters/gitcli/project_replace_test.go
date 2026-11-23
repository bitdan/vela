package gitcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"vela/internal/application"
)

func replacePreview(t *testing.T, root string, paths []string) application.ProjectReplaceInput {
	t.Helper()
	q := application.ProjectSearchQuery{Text: "needle", MatchCase: true}
	preview, err := application.SearchProject(context.Background(), application.ProjectSearchInput{Root: root, Paths: paths, Query: q, Replacement: "new"}, func(_ context.Context, p string) (string, error) {
		data, err := os.ReadFile(filepath.Join(root, p))
		return string(data), err
	})
	if err != nil || preview.Limited || preview.Unreadable != 0 {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	return application.ProjectReplaceInput{Query: q, Replacement: "new", Matches: preview.Matches}
}

func TestReplaceProjectCRLFAndPreflight(t *testing.T) {
	f := newWorkflow(t)
	names := []string{"a 中文.txt", "z space.txt"}
	for _, name := range names {
		f.write(name, "needle\r\nneedle\r\n")
	}
	repo := f.read()
	input := replacePreview(t, repo.Root, names)
	head, index := f.git("rev-parse", "HEAD"), f.git("write-tree")
	result, err := f.g.ReplaceProject(context.Background(), repo, input)
	if err != nil || result.Replacements != 4 || !reflect.DeepEqual(result.Paths, names) {
		t.Fatalf("replace: %+v %v", result, err)
	}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(repo.Root, name))
		if err != nil || string(data) != "new\r\nnew\r\n" {
			t.Fatalf("CRLF %q: %q %v", name, data, err)
		}
	}
	if f.git("rev-parse", "HEAD") != head || f.git("write-tree") != index {
		t.Fatal("replacement changed HEAD/index")
	}
	for _, name := range names {
		f.write(name, "needle\r\n")
	}
	input = replacePreview(t, repo.Root, names)
	f.write(names[1], "external needle\r\n")
	result, err = f.g.ReplaceProject(context.Background(), repo, input)
	if err == nil || len(result.Paths) != 0 {
		t.Fatalf("stale batch wrote files: %+v %v", result, err)
	}
	data, _ := os.ReadFile(filepath.Join(repo.Root, names[0]))
	if string(data) != "needle\r\n" {
		t.Fatal("preflight failure partially wrote")
	}
	for _, memory := range []bool{false, true} {
		input = replacePreview(t, repo.Root, names)
		if memory {
			input.Matches[0].InMemory = true
		} else {
			input.Matches[0].RawVersion = ""
		}
		result, err = f.g.ReplaceProject(context.Background(), repo, input)
		if err == nil || len(result.Paths) != 0 {
			t.Fatalf("unsaved/unversioned preview wrote: %+v %v", result, err)
		}
	}
}

// Cancel at the first context checkpoint after a real durable rename. This
// avoids wall-clock races and still exercises the actual filesystem writer.
type cancelAfterReplacement struct {
	context.Context
	cancel context.CancelFunc
	path   string
}

func (c cancelAfterReplacement) Err() error {
	data, _ := os.ReadFile(c.path)
	if string(data) == "new\r\n" {
		c.cancel()
	}
	return c.Context.Err()
}

func TestReplaceProjectCancellationReportsDurableProgress(t *testing.T) {
	f := newWorkflow(t)
	names := []string{"a 中文.txt", "z space.txt"}
	for _, name := range names {
		f.write(name, "needle\r\n")
	}
	repo := f.read()
	input := replacePreview(t, repo.Root, names)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := f.g.ReplaceProject(ctx, repo, input)
	if !errors.Is(err, context.Canceled) || len(result.Paths) != 0 {
		t.Fatalf("pre-cancel: %+v %v", result, err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	result, err = f.g.ReplaceProject(cancelAfterReplacement{ctx, cancel, filepath.Join(repo.Root, names[0])}, repo, input)
	if !errors.Is(err, context.Canceled) || result.Replacements != 1 || !reflect.DeepEqual(result.Paths, names[:1]) {
		t.Fatalf("partial cancellation: %+v %v", result, err)
	}
	for i, want := range []string{"new\r\n", "needle\r\n"} {
		data, _ := os.ReadFile(filepath.Join(repo.Root, names[i]))
		if string(data) != want {
			t.Fatalf("file %d: %q", i, data)
		}
	}
	if f.git("rev-parse", "HEAD") != repo.Head {
		t.Fatal("cancel changed HEAD")
	}
}
