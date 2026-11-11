package ui

import native "github.com/egoist/mygo/ui"

const (
	windowWidth, windowHeight                                = 1440, 900
	toolbarHeight, statusHeight                              = 40, 25
	activityWidth, headerHeight, rowHeight                   = 46, 32, 24
	inset, gap, controlHeight                                = 10, 6, 28
	projectIconSize                                          = 20
	graphWidth, graphStart, graphStep, graphStroke, graphDot = 46, 8, 13, 1.3, 3.2
	smallText, normalText                                    = 12, 13
	emptyEditorText                                          = 16
)

type tokens struct {
	treeRepository, treeModule    native.Color
	theme                         *native.Theme
	chrome, panel, editor, stripe native.Color
	graph                         [3]native.Color
	selectionLine, favorite       native.Color
	settingsSelection             native.Color
	settingsControlTrack          native.Color
	settingsControlSelected       native.Color
}

func (p tokens) textSize(size float32) float32 {
	return size + p.theme.FontSize - normalText
}

func design(dark bool) tokens {
	t := native.LightTheme()
	p := tokens{theme: t, chrome: native.Hex("#eef0f3"), panel: native.Hex("#f7f8fa"),
		editor: native.Hex("#ffffff"), stripe: native.Hex("#edf0f5"),
		graph: [3]native.Color{native.Hex("#277d8b"), native.Hex("#7653bc"), native.Hex("#33825d")}}
	if dark {
		t = native.DarkTheme()
		p = tokens{theme: t, chrome: native.Hex("#2b2d31"), panel: native.Hex("#25262a"),
			editor: native.Hex("#1e1f22"), stripe: native.Hex("#292b31"),
			graph: [3]native.Color{native.Hex("#59b9c5"), native.Hex("#af90e8"), native.Hex("#73bc98")}}
		t.Text, t.TextMuted, t.Border = native.Hex("#e1e4eb"), native.Hex("#a5adba"), native.Hex("#3c4049")
		t.Surface, t.Accent = native.Hex("#32353d"), native.Hex("#41454D")
		t.SurfaceHover, t.SurfacePressed = native.Hex("#35383E"), native.Hex("#454951")
		t.InactiveAccent, t.Selection, t.Focus = native.Hex("#383B42"), native.Hex("#404956"), native.Hex("#849AB7")
		p.settingsSelection = native.Hex("#3C4A63")
		p.settingsControlTrack, p.settingsControlSelected = native.Hex("#35383E"), native.Hex("#4A505A")
		p.treeRepository, p.treeModule = native.Hex("#69AC83"), native.Hex("#87A8FF")
		p.selectionLine, p.favorite = native.Hex("#888E9A"), native.Hex("#F2B84B")
	} else {
		t.Text, t.TextMuted, t.Border = native.Hex("#242831"), native.Hex("#535d6d"), native.Hex("#d6dae1")
		t.Surface, t.Accent = native.Hex("#ffffff"), native.Hex("#DCDEE2")
		t.SurfaceHover, t.SurfacePressed = native.Hex("#EBEDF0"), native.Hex("#DCDEE2")
		t.InactiveAccent, t.Selection, t.Focus = native.Hex("#E6E8EB"), native.Hex("#E3EAF6"), native.Hex("#7E97BA")
		p.settingsSelection = native.Hex("#D3DFF8")
		p.settingsControlTrack, p.settingsControlSelected = native.Hex("#E2E5E9"), t.Surface
		p.treeRepository, p.treeModule = native.Hex("#82BA94"), native.Hex("#3568D4")
		p.selectionLine, p.favorite = native.Hex("#A8AFBE"), native.Hex("#C48412")
	}
	t.AccentText = t.Text
	t.AccentHover, t.AccentPressed = t.SurfaceHover, t.SurfacePressed
	t.Background, t.Radius, t.Spacing, t.FontSize = p.editor, 4, 3, normalText
	return p
}

func divider(c *native.Context, p tokens) {
	native.Box(c).FillWidth().Height(1).Background(p.theme.Border)
}

func heading(c *native.Context, p tokens, title, detail string) {
	native.Row(c).FillWidth().Height(headerHeight).Padding(0, inset).Gap(gap).AlignItems(native.Center).Background(p.chrome).Children(func() {
		native.Text(c, title).Bold().Grow(1)
		if detail != "" {
			native.Text(c, detail).FontSize(p.textSize(smallText)).TextColor(p.theme.TextMuted)
		}
	})
	divider(c, p)
}
