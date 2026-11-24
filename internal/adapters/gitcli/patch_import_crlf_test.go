package gitcli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vela/internal/application"
)

func crlfImportFixture(t *testing.T) (*workflowFixture, string, string, string, string) {
	t.Helper()
	f := newWorkflow(t)
	f.git("config", "core.autocrlf", "false")
	name := "文件 name.txt"
	var lines []string
	for i := 0; i < 32; i++ {
		lines = append(lines, fmt.Sprintf("line %02d", i))
	}
	base := strings.Join(lines, "\r\n") + "\r\n"
	first := strings.Replace(base, "line 02", "chosen first", 1)
	both := strings.Replace(first, "line 29", "unchecked second", 1)
	f.write(name, base)
	f.git("add", "--", name)
	f.git("commit", "-m", "CRLF baseline")
	f.write(name, both)
	raw, err := f.g.read(context.Background(), f.root, "diff", "--no-ext-diff", "--no-textconv", "--", name)
	if err != nil {
		t.Fatal(err)
	}
	f.write(name, base)
	return f, raw, base, first, both
}

func TestImportedCRLFApplyReverseAndHunks(t *testing.T) {
	for _, target := range []string{"Working tree", "Index only"} {
		for _, partial := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/partial=%t", target, partial), func(t *testing.T) {
				t.Parallel()
				f, raw, base, first, both := crlfImportFixture(t)
				ctx := context.Background()
				name := "文件 name.txt"
				path := filepath.Join(t.TempDir(), "raw.patch")
				if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
					t.Fatal(err)
				}
				repo := f.read()
				head, index := repo.Head, f.git("write-tree")
				doc, err := f.g.PreviewImportedPatch(ctx, repo, path, target)
				if err != nil {
					t.Fatal(err)
				}
				expected := both
				if partial {
					doc, err = application.SelectImportedPatch(raw, map[string][]int{name: {0}})
					if err != nil {
						t.Fatal(err)
					}
					doc.Target = target
					doc, err = f.g.ReviewImportedPatch(ctx, repo, doc)
					if err != nil {
						t.Fatal(err)
					}
					expected = first
				}
				if err := f.g.ApplyImportedPatch(ctx, repo, doc); err != nil {
					t.Fatal(err)
				}
				disk, err := os.ReadFile(filepath.Join(f.root, name))
				if err != nil {
					t.Fatal(err)
				}
				blob, err := f.g.read(ctx, f.root, "show", ":0:"+name)
				if err != nil {
					t.Fatal(err)
				}
				if target == "Working tree" {
					if string(disk) != expected || blob != base || f.git("write-tree") != index {
						t.Fatal("worktree CRLF apply changed bytes/index")
					}
				} else {
					if string(disk) != base || blob != expected {
						t.Fatal("Index CRLF apply changed bytes/disk")
					}
				}
				if f.git("rev-parse", "HEAD") != head {
					t.Fatal("apply moved HEAD")
				}
				repo = f.read()
				doc.Reverse = true
				doc, err = f.g.ReviewImportedPatch(ctx, repo, doc)
				if err != nil {
					t.Fatal(err)
				}
				if err := f.g.ApplyImportedPatch(ctx, repo, doc); err != nil {
					t.Fatal(err)
				}
				disk, _ = os.ReadFile(filepath.Join(f.root, name))
				blob, err = f.g.read(ctx, f.root, "show", ":0:"+name)
				if err != nil || string(disk) != base || blob != base || f.git("write-tree") != index || f.git("rev-parse", "HEAD") != head {
					t.Fatal("reverse did not restore exact original bytes")
				}
			})
		}
	}
}

func TestImportedCRLFStaleCancelAndUnsafeText(t *testing.T) {
	t.Parallel()
	f, raw, base, _, _ := crlfImportFixture(t)
	ctx := context.Background()
	name := "文件 name.txt"
	input, err := application.ParseImportedPatch(raw)
	if err != nil {
		t.Fatal(err)
	}
	input.Target = "Working tree"
	repo := f.read()
	doc, err := f.g.ReviewImportedPatch(ctx, repo, input)
	if err != nil {
		t.Fatal(err)
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	if err := f.g.ApplyImportedPatch(cancelCtx, repo, doc); err == nil {
		t.Fatal("canceled apply succeeded")
	}
	f.write(name, base+"external\r\n")
	if err := f.g.ApplyImportedPatch(ctx, repo, doc); err == nil {
		t.Fatal("stale apply succeeded")
	}
	data, _ := os.ReadFile(filepath.Join(f.root, name))
	if string(data) != base+"external\r\n" {
		t.Fatal("stale/cancel lost external data")
	}
	for _, bad := range []string{strings.Replace(base, "line 20\r\n", "line 20\n", 1), strings.Replace(base, "line 20", "bare\rcarriage", 1)} {
		f.write(name, bad)
		if _, err := f.g.ReviewImportedPatch(ctx, f.read(), input); err == nil {
			t.Fatal("accepted mixed or bare CR target")
		}
	}
	f.write(name, base)
	// Validate Index text independently from the unchanged disk target.
	f.write(name, strings.Replace(base, "line 20\r\n", "line 20\n", 1))
	f.git("add", "--", name)
	f.write(name, base)
	input.Target = "Index only"
	if _, err := f.g.ReviewImportedPatch(ctx, f.read(), input); err == nil {
		t.Fatal("accepted mixed Index blob")
	}
	f.git("reset", "--mixed", "HEAD")
	input.Target = "Working tree"
	f.write(".gitattributes", "*.txt text eol=crlf\n")
	if _, err := f.g.ReviewImportedPatch(ctx, f.read(), input); err == nil {
		t.Fatal("accepted conversion attributes")
	}
}

func TestImportedCRLFStructuralAndNoFinalNewline(t *testing.T) {
	for _, target := range []string{"Working tree", "Index only"} {
		t.Run(target, func(t *testing.T) {
			t.Parallel()
			f := newWorkflow(t)
			ctx := context.Background()
			base := "original\r\nlast without newline"
			f.write("文件 name.txt", base)
			f.write("remove.txt", base)
			f.write("old.txt", strings.Repeat("rename\r\n", 30))
			f.git("add", "--", "文件 name.txt", "remove.txt", "old.txt")
			f.git("commit", "-m", "CRLF structural base")
			f.write("文件 name.txt", "changed\r\nnew last without newline")
			f.write("added.txt", "added\r\n")
			f.git("rm", "--", "remove.txt")
			f.git("mv", "--", "old.txt", "renamed.txt")
			f.git("add", "--", "文件 name.txt", "added.txt")
			raw, err := f.g.read(ctx, f.root, "diff", "--cached", "--binary", "--no-ext-diff", "--no-textconv", "--find-renames")
			if err != nil {
				t.Fatal(err)
			}
			f.git("reset", "--mixed", "HEAD")
			f.write("文件 name.txt", base)
			f.write("remove.txt", base)
			f.write("old.txt", strings.Repeat("rename\r\n", 30))
			for _, p := range []string{"added.txt", "renamed.txt"} {
				if err := os.Remove(filepath.Join(f.root, p)); err != nil {
					t.Fatal(err)
				}
			}
			doc, err := application.ParseImportedPatch(raw)
			if err != nil {
				t.Fatal(err)
			}
			doc.Target = target
			repo := f.read()
			tree := f.git("write-tree")
			head := repo.Head
			doc, err = f.g.ReviewImportedPatch(ctx, repo, doc)
			if err != nil {
				t.Fatal(err)
			}
			if err := f.g.ApplyImportedPatch(ctx, repo, doc); err != nil {
				t.Fatal(err)
			}
			read := func(p string) string {
				t.Helper()
				if target == "Index only" {
					s, e := f.g.read(ctx, f.root, "show", ":0:"+p)
					if e != nil {
						t.Fatal(e)
					}
					return s
				}
				b, e := os.ReadFile(filepath.Join(f.root, p))
				if e != nil {
					t.Fatal(e)
				}
				return string(b)
			}
			if read("文件 name.txt") != "changed\r\nnew last without newline" || read("added.txt") != "added\r\n" || read("renamed.txt") != strings.Repeat("rename\r\n", 30) {
				t.Fatal("structural CRLF bytes changed")
			}
			if target == "Working tree" && f.git("write-tree") != tree {
				t.Fatal("worktree structural import changed Index")
			}
			if target == "Index only" {
				disk, _ := os.ReadFile(filepath.Join(f.root, "文件 name.txt"))
				if string(disk) != base {
					t.Fatal("Index structural import changed disk")
				}
			}
			repo = f.read()
			doc.Reverse = true
			doc, err = f.g.ReviewImportedPatch(ctx, repo, doc)
			if err != nil {
				t.Fatal(err)
			}
			if err := f.g.ApplyImportedPatch(ctx, repo, doc); err != nil {
				t.Fatal(err)
			}
			if read("文件 name.txt") != base || read("remove.txt") != base || f.git("write-tree") != tree || f.git("rev-parse", "HEAD") != head {
				t.Fatal("structural reverse changed baseline")
			}
		})
	}
}
