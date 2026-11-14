package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestProjectSearchUnicodeCRLFAndOverlay(t *testing.T) {
	input := ProjectSearchInput{Paths: []string{"unopened.go", "dirty.go", "unopened.go"}, Query: ProjectSearchQuery{Text: "k"}, Overlays: map[string]string{"dirty.go": "中文 K\r\nK k\r\n"}}
	reads := 0
	got, err := SearchProject(context.Background(), input, func(_ context.Context, path string) (string, error) { reads++; return "no match", nil })
	if err != nil || reads != 1 || got.Scanned != 2 || len(got.Matches) != 3 {
		t.Fatalf("search scope/counts: %+v %v reads=%d", got, err, reads)
	}
	for i, want := range []struct{ start, end, line, column int }{{7, 8, 1, 4}, {9, 12, 2, 1}, {13, 14, 2, 3}} {
		m := got.Matches[i]
		if m.Path != "dirty.go" || !m.InMemory || m.Start != want.start || m.End != want.end || m.Line != want.line || m.Column != want.column {
			t.Fatalf("match %d: %+v", i, m)
		}
	}
	if got.Matches[0].Version != ProjectSearchVersion("中文 K\nK k\n") {
		t.Fatal("line endings affect content version")
	}
	input.Query.MatchCase = true
	exact, err := SearchProject(context.Background(), input, func(context.Context, string) (string, error) { return "", nil })
	if err != nil || len(exact.Matches) != 1 || exact.Matches[0].Start != 13 {
		t.Fatalf("case-sensitive matches: %+v %v", exact, err)
	}
}

func TestProjectSearchFileAndByteBudgets(t *testing.T) {
	input := ProjectSearchInput{Query: ProjectSearchQuery{Text: "absent"}}
	for i := 0; i < MaxProjectSearchFiles+1; i++ {
		input.Paths = append(input.Paths, fmt.Sprintf("%05d", i))
	}
	reads := 0
	got, err := SearchProject(context.Background(), input, func(context.Context, string) (string, error) { reads++; return "", nil })
	if err != nil || !got.Limited || reads != MaxProjectSearchFiles {
		t.Fatalf("file budget: reads=%d limited=%v err=%v", reads, got.Limited, err)
	}
	input.Paths = input.Paths[:MaxProjectSearchBytes/MaxProjectSearchFileBytes+1]
	text := strings.Repeat("a", MaxProjectSearchFileBytes)
	got, err = SearchProject(context.Background(), input, func(context.Context, string) (string, error) { return text, nil })
	if err != nil || !got.Limited || got.Scanned != MaxProjectSearchBytes/MaxProjectSearchFileBytes {
		t.Fatalf("byte budget: scanned=%d limited=%v err=%v", got.Scanned, got.Limited, err)
	}
}

func TestProjectSearchLimitsErrorsAndCancellation(t *testing.T) {
	input := ProjectSearchInput{Paths: []string{"a", "b", "c", "d"}, Query: ProjectSearchQuery{Text: "needle"}}
	got, err := SearchProject(context.Background(), input, func(_ context.Context, p string) (string, error) {
		switch p {
		case "a":
			return "", ErrProjectSearchSkipped
		case "b":
			return "", errors.New("unreadable")
		case "c":
			return strings.Repeat("needle ", MaxProjectSearchMatches+1), nil
		}
		return "needle", nil
	})
	if err != nil || !got.Limited || len(got.Matches) != MaxProjectSearchMatches || got.Skipped != 1 || got.Unreadable != 1 || got.Scanned != 1 {
		t.Fatalf("limits/errors: %+v %v", got, err)
	}
	if !strings.Contains(got.FirstError, "b") || !strings.Contains(got.FirstError, "unreadable") {
		t.Fatal("read error lost file context")
	}
	ctx, cancel := context.WithCancel(context.Background())
	_, err = SearchProject(ctx, input, func(context.Context, string) (string, error) { cancel(); return "needle", nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation ignored: %v", err)
	}
	for _, q := range []string{"", "x\ny", string([]byte{0xff}), "\x00", strings.Repeat("x", 4097)} {
		input.Query.Text = q
		if _, err := SearchProject(context.Background(), input, nil); err == nil {
			t.Fatalf("invalid query accepted %q", q)
		}
	}
}

func TestProjectSearchRejectsUnsafeScope(t *testing.T) {
	for _, p := range []string{"../outside", "/outside", ".git/config", "nested/.GiT/config", "a\\b", "C:/outside", "a/../b"} {
		_, err := SearchProject(context.Background(), ProjectSearchInput{Paths: []string{p}, Query: ProjectSearchQuery{Text: "x"}, Overlays: map[string]string{p: "x"}}, nil)
		if err == nil {
			t.Fatalf("unsafe scope accepted: %q", p)
		}
	}
}
