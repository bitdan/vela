package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"vela/internal/domain"
)

func TestCommitLogPagesFreezeAllTipsAndReconnectMerge(t *testing.T) {
	f := newWorkflow(t)
	f.git("checkout", "-b", "side")
	f.write("side.txt", "side\n")
	f.git("add", ".")
	f.git("commit", "-m", "side")
	f.git("checkout", "main")
	f.write("main.txt", "main\n")
	f.git("add", ".")
	f.git("commit", "-m", "main")
	f.git("merge", "--no-ff", "--no-edit", "side")
	f.git("tag", "-a", "annotated", "-m", "tag")
	f.git("-c", "advice.nestedTag=false", "tag", "-a", "nested", "annotated", "-m", "nested tag")
	r := f.read()
	head, index := r.Head, f.git("write-tree")
	first, err := f.g.ReadCommitLog(context.Background(), r.Root, r.LogSnapshot, 0, 2)
	if err != nil || len(first.Commits) != 2 || !first.HasMore {
		t.Fatalf("first page: %+v %v", first, err)
	}
	f.git("commit", "--allow-empty", "-m", "outside frozen query")
	f.git("tag", "-d", "annotated")
	f.git("branch", "-f", "side", "HEAD")
	second, err := f.g.ReadCommitLog(context.Background(), r.Root, r.LogSnapshot, 2, 2)
	if err != nil || len(second.Commits) != 2 || second.HasMore {
		t.Fatalf("second page: %+v %v", second, err)
	}
	all := append(first.Commits, second.Commits...)
	domain.ReconnectCommitLog(all)
	if all[0].Hash != head || len(all[0].Parents) != 2 || !strings.Contains(strings.Join(all[0].Refs, " "), "refs/tags/annotated") || !strings.Contains(strings.Join(all[0].Refs, " "), "refs/tags/nested") {
		t.Fatalf("snapshot or merge lost: %+v", all)
	}
	seen := map[string]bool{}
	for _, c := range all {
		if seen[c.Hash] || c.Subject == "outside frozen query" {
			t.Fatal("unstable history page")
		}
		seen[c.Hash] = true
	}
	if index != f.git("write-tree") {
		t.Fatal("read changed Index")
	}
}

func TestCommitLogRejectsChangedAncestryAndIgnoresReplaceRefs(t *testing.T) {
	f := newWorkflow(t)
	base := f.git("rev-parse", "HEAD")
	f.git("commit", "--allow-empty", "-m", "new tip")
	r := f.read()
	// Replacement refs must never silently change an existing history snapshot.
	f.git("replace", r.Head, base)
	page, err := f.g.ReadCommitLog(context.Background(), r.Root, r.LogSnapshot, 0, 500)
	if err != nil || len(page.Commits) != 2 || page.Commits[0].Subject != "new tip" {
		t.Fatalf("replacement changed frozen history: %+v %v", page, err)
	}
	f.git("replace", "-d", r.Head)
	// Deepening a shallow clone changes the reachable set even at fixed tips.
	if err := os.WriteFile(filepath.Join(r.CommonDir, "shallow"), []byte(r.Head+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.g.ReadCommitLog(context.Background(), r.Root, r.LogSnapshot, 0, 500); err == nil {
		t.Fatal("ancestry boundary change accepted")
	}
	if err := os.Remove(filepath.Join(r.CommonDir, "shallow")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.CommonDir, "info", "grafts"), []byte(r.Head+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.g.ReadCommitLog(context.Background(), r.Root, r.LogSnapshot, 0, 500); err == nil {
		t.Fatal("graft boundary change accepted")
	}
}

func TestCommitLogRejectsOversizedMetadataPage(t *testing.T) {
	f := newWorkflow(t)
	r := f.read()
	cmd := exec.Command("git", "commit", "--allow-empty", "-F", "-")
	cmd.Dir, cmd.Env = f.root, repositoryEnvironment()
	cmd.Stdin = strings.NewReader("large message\n\n" + strings.Repeat("x", 17<<20) + "\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("large fixture: %s %v", out, err)
	}
	r.LogSnapshot.Tips = []string{f.git("rev-parse", "HEAD")}
	if _, err := f.g.ReadCommitLog(context.Background(), r.Root, r.LogSnapshot, 0, 1); !errors.Is(err, errOutputLimit) {
		t.Fatalf("expected output limit, got %v", err)
	}
}

func TestCommitLogPagesBeyondTenThousand(t *testing.T) {
	f := newWorkflow(t)
	const total = 10023
	var input strings.Builder
	for i := 1; i <= total; i++ {
		fmt.Fprintf(&input, "commit refs/heads/long\nmark :%d\ncommitter Test <test@example.test> %d +0000\ndata %d\ncommit-%d\n", i, 1700000000+i, len(fmt.Sprintf("commit-%d", i)), i)
		if i == 1 {
			fmt.Fprintf(&input, "from %s\n", f.git("rev-parse", "HEAD"))
		}
		input.WriteString("\n")
	}
	cmd := exec.Command("git", "fast-import", "--quiet")
	cmd.Dir, cmd.Stdin = f.root, strings.NewReader(input.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %s %v", out, err)
	}
	r := f.read()
	if len(r.History) != 500 || !r.LogHasMore {
		t.Fatalf("initial read is not bounded: %d %t", len(r.History), r.LogHasMore)
	}
	seen := make(map[string]bool)
	for offset := 0; ; offset += 500 {
		page, err := f.g.ReadCommitLog(context.Background(), r.Root, r.LogSnapshot, offset, 500)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range page.Commits {
			if seen[c.Hash] {
				t.Fatal("duplicate commit across pages")
			}
			seen[c.Hash] = true
		}
		if !page.HasMore {
			break
		}
	}
	if len(seen) != total+1 {
		t.Fatalf("history truncated: got %d want %d", len(seen), total+1)
	}
}

func TestCommitLogPageRejectsInvalidQueryAndCancellation(t *testing.T) {
	f := newWorkflow(t)
	r := f.read()
	for _, q := range []struct{ offset, limit int }{{-1, 1}, {0, 0}, {0, 501}} {
		if _, err := f.g.ReadCommitLog(context.Background(), r.Root, r.LogSnapshot, q.offset, q.limit); err == nil {
			t.Fatalf("invalid bounds accepted: %+v", q)
		}
	}
	if _, err := f.g.ReadCommitLog(context.Background(), r.Root, domain.CommitLogSnapshot{Tips: []string{"--all"}}, 0, 1); err == nil {
		t.Fatal("non-OID tip accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.g.ReadCommitLog(ctx, r.Root, r.LogSnapshot, 0, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel lost: %v", err)
	}
}
