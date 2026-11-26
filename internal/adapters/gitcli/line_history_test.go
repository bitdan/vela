package gitcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"vela/internal/domain"
)

func lineHistoryQuery(t *testing.T, f *workflowFixture, path string, start, end int) (domain.Repository, domain.LineHistoryQuery) {
	t.Helper()
	r := f.read()
	file, err := f.g.ReadFile(context.Background(), f.root, "project", domain.File{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	return r, domain.LineHistoryQuery{Revision: r.Head, Path: path, Text: file.New, DiskVersion: file.DiskVersion, Start: start, End: end, Limit: 100}
}
func TestLineHistoryNativeRangePagingAndFixedIdentity(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	path := "中文 range file.txt"
	f.write(path, "one\ntwo\nthree\n")
	f.git("add", ".")
	f.git("commit", "-m", "range base")
	first := f.git("rev-parse", "HEAD")
	f.write(path, "one\n第二\nthree\n")
	f.git("commit", "-am", "range change")
	second := f.git("rev-parse", "HEAD")
	f.write(path, "one\n第二\noutside\n")
	f.git("commit", "-am", "outside range")
	r, q := lineHistoryQuery(t, f, path, 2, 2)
	q.Limit = 1
	page, err := f.g.ReadLineHistory(context.Background(), r, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 1 || page.Entries[0].Commit.Hash != second || !page.HasMore {
		t.Fatalf("wrong matching history %+v", page)
	}
	h := page.Entries[0].Hunks[0]
	if h.Before != "two\n" || h.After != "第二\n" {
		t.Fatalf("wrong range diff %+v", h)
	}
	q.Offset = 1
	page, err = f.g.ReadLineHistory(context.Background(), r, q)
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Commit.Hash != first || page.HasMore {
		t.Fatal("paging lost fixed history", page, err)
	}
	q.DiskVersion = "stale"
	if _, err = f.g.ReadLineHistory(context.Background(), r, q); err == nil {
		t.Fatal("stale disk query accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = f.g.ReadLineHistory(ctx, r, q); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel lost", err)
	}
}
func TestLineHistoryCRLFEOFAndMismatch(t *testing.T) {
	for _, text := range []string{"first\r\nsecond\r\n", "first\nsecond"} {
		t.Run(text, func(t *testing.T) {
			t.Parallel()
			f := newWorkflow(t)
			f.git("config", "core.autocrlf", "false")
			path := "行 history.txt"
			f.write(path, text)
			f.git("add", ".")
			f.git("commit", "-m", "raw text")
			r, q := lineHistoryQuery(t, f, path, 2, 2)
			page, err := f.g.ReadLineHistory(context.Background(), r, q)
			if err != nil || len(page.Entries) != 1 {
				t.Fatal(page, err)
			}
			if page.Entries[0].Hunks[0].After != strings.SplitAfter(text, "\n")[1] {
				t.Fatalf("raw line mismatch %+v", page.Entries[0].Hunks)
			}
			bad := q
			bad.Text = "edited"
			if _, err = f.g.ReadLineHistory(context.Background(), r, bad); err == nil {
				t.Fatal("edited text attributed")
			}
			f.write(path, "external\n")
			if _, err = f.g.ReadLineHistory(context.Background(), r, q); err == nil {
				t.Fatal("external text attributed")
			}
		})
	}
}

func TestLineHistoryCanonicalAndAncestryBoundary(t *testing.T) {
	f := newWorkflow(t)
	path := "range.txt"
	f.write(path, "first\n")
	f.git("add", ".")
	f.git("commit", "-m", "first")
	f.write(path, "second\n")
	f.git("commit", "-am", "second")
	r, q := lineHistoryQuery(t, f, path, 1, 1)
	f.git("replace", r.Head, r.History[len(r.History)-1].Hash)
	page, err := f.g.ReadLineHistory(context.Background(), r, q)
	if err != nil || len(page.Entries) < 2 || page.Entries[0].Commit.Hash != r.Head {
		t.Fatal("replace altered canonical history", page, err)
	}
	alternate := filepath.Join(t.TempDir(), "ancestry")
	if err := os.WriteFile(alternate, []byte(r.Head+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"GIT_GRAFT_FILE", "GIT_SHALLOW_FILE"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, alternate)
			got, err := f.g.ReadLineHistory(context.Background(), r, q)
			if err != nil || len(got.Entries) != len(page.Entries) {
				t.Fatalf("%s altered canonical line history: %v, %v", name, got, err)
			}
		})
	}
	if err := os.WriteFile(filepath.Join(r.CommonDir, "info", "grafts"), []byte(r.Head+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = f.g.ReadLineHistory(context.Background(), r, q); err == nil {
		t.Fatal("changed ancestry silently shifted range history")
	}
}
