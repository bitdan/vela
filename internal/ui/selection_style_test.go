package ui

import (
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
)

func TestNeutralSelectionTheme(t *testing.T) {
	for _, tc := range []struct {
		dark      bool
		selection string
	}{{false, "#DCDEE2"}, {true, "#41454D"}} {
		p := design(tc.dark)
		if p.theme.Accent != native.Hex(tc.selection) || p.theme.AccentText != p.theme.Text {
			t.Fatalf("dark=%v: selection should use neutral fill and ordinary text", tc.dark)
		}
	}
}

func TestBranchDropdownFillFollowsMenu(t *testing.T) {
	layout := application.DefaultLayout()
	layout.Dark = false
	a := New(application.New(), layout)
	tt := native.NewTester(a.View, 1440, 900)
	box, ok := tt.Find("Current branch")
	if !ok {
		t.Fatal("missing branch dropdown")
	}
	pixel := func() native.Color {
		c := tt.Image().RGBAAt(int(box.X)+5, int(box.Y)+5)
		return native.RGB(c.R, c.G, c.B)
	}
	p := design(false)
	if got := pixel(); got != p.chrome {
		t.Fatalf("closed dropdown fill = %v", got)
	}
	if err := tt.Click("Current branch"); err != nil {
		t.Fatal(err)
	}
	if got := pixel(); got != p.theme.Accent {
		t.Fatalf("open dropdown fill = %v", got)
	}
	tt.Key(0, native.KeyEscape)
	if got := pixel(); got != p.chrome {
		t.Fatalf("dropdown fill after Escape = %v", got)
	}
}

func TestGitActivitySelectionState(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	before, ok := tt.Access("Toggle Git")
	if !ok || !before.Checked {
		t.Fatal("visible Git tool is not marked selected")
	}
	if err := tt.Click("Toggle Git"); err != nil {
		t.Fatal(err)
	}
	after, ok := tt.Access("Toggle Git")
	if !ok || after.Checked {
		t.Fatal("hidden Git tool still appears selected")
	}
}
