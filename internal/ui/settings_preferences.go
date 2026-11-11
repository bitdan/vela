package ui

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
	"vela/internal/application"
)

const (
	appearancePage          = "Appearance"
	editorPage              = "Editor"
	aboutPage               = "About"
	preferenceControlHeight = 26
)

var settingsFonts = sync.OnceValue(ui.FontFamilies)

func isVersionControlPage(page string) bool { return slices.Contains(settingsPages, page) }

func matchesSettingsPage(page, query string) bool {
	return strings.Contains(strings.ToLower(page), query) ||
		(page == shortcutHelpPage && strings.Contains("keymap", query))
}

func (a *Workspace) darkTheme() bool {
	if a.Layout.Appearance.FollowSystem {
		return a.systemDark
	}
	return a.Layout.Dark
}

func preferredFont(font, fallback string) string {
	if font == "" || font == application.SystemFont {
		return fallback
	}
	return font
}

func (a *Workspace) editorPreferences() (ui.Font, int) {
	prefs := a.Layout.Editor
	return ui.Font{Family: preferredFont(prefs.Font, "monospace"), Size: float32(min(max(prefs.FontSize, 9), 28))},
		min(max(prefs.TabSize, 1), 8)
}

func (a *Workspace) appearanceSettings(c *ui.Context, p tokens) {
	s := &a.settings
	preferencesCard(c, p, func() {
		preferenceRow(c, "Theme", func() {
			ui.Row(c).Padding(2).Gap(1).Radius(7).Background(p.settingsControlTrack).Children(func() {
				for _, mode := range []string{"System", "Light", "Dark"} {
					selected := mode == "System" && s.appearance.FollowSystem ||
						!s.appearance.FollowSystem && (mode == "Light" && !s.dark || mode == "Dark" && s.dark)
					button := ui.ButtonBase(c.Key(mode)).Height(preferenceControlHeight).MinWidth(52).Padding(0, inset).
						Center().Radius(5).Label("Theme " + mode).Role(ui.RoleToggleButton).Checked(selected)
					color := p.theme.TextMuted
					if selected {
						button.Background(p.settingsControlSelected)
						color = p.theme.Text
					} else {
						preferenceHover(button, p)
					}
					button.Children(func() {
						ui.Text(c, mode).TextColor(color).SingleLine()
					}).OnClick(func() {
						s.appearance.FollowSystem = mode == "System"
						if mode != "System" {
							s.dark = mode == "Dark"
						}
					})
				}
			})
		})
		divider(c, p)
		preferenceRow(c, "Language", func() { ui.Text(c, "English").TextColor(p.theme.TextMuted) })
		divider(c, p)
		preferenceStepper(c, p, "UI scale", &s.appearance.Scale, 80, 125, 5, "%")
		divider(c, p)
		preferenceRow(c, "UI font", func() { preferenceFont(c, p, "UI font", &s.appearance.Font, false) })
		divider(c, p)
		preferenceStepper(c, p, "UI font size", &s.appearance.FontSize, 11, 18, 1, "")
	})
}

func (a *Workspace) editorSettings(c *ui.Context, p tokens) {
	s := &a.settings.editor
	preferencesCard(c, p, func() {
		preferenceRow(c, "Font", func() { preferenceFont(c, p, "Editor font", &s.Font, true) })
		divider(c, p)
		preferenceStepper(c, p, "Editor font size", &s.FontSize, 9, 28, 1, "")
		divider(c, p)
		preferenceStepper(c, p, "Tab width", &s.TabSize, 1, 8, 1, "")
	})
}

func aboutSettings(c *ui.Context, p tokens) {
	preferencesCard(c, p, func() {
		ui.Column(c).FillWidth().Center().Padding(2 * inset).Gap(gap).Children(func() {
			ui.Image(c, projectIcon).Size(58, 58)
			ui.Text(c, "vela").FontSize(22)
			version := "Development build"
			if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
				version = info.Main.Version
			}
			ui.Text(c, version+" · "+runtime.GOARCH).TextColor(p.theme.TextMuted)
		})
	})
	preferencesCard(c, p, func() {
		preferenceRow(c, "Software updates", func() {
			ui.Text(c, "In-app updates are unavailable in this build").TextColor(p.theme.TextMuted)
		})
		divider(c, p)
		preferenceRow(c, "Open source license", func() {
			ui.Link(c, "Apache-2.0", "https://www.apache.org/licenses/LICENSE-2.0").TextColor(p.treeModule)
		})
	})
}

func preferencesCard(c *ui.Context, p tokens, content func()) {
	ui.Column(c).FillWidth().Background(p.chrome).Radius(8).Padding(gap, inset).Children(content)
}

func preferenceRow(c *ui.Context, label string, control func()) {
	ui.Row(c).FillWidth().MinHeight(headerHeight + 4).Gap(gap).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Grow(1)
		control()
	})
}

func preferenceStepper(c *ui.Context, p tokens, label string, value *int, lo, hi, step int, unit string) {
	preferenceRow(c, label, func() {
		ui.Row(c).Gap(3).AlignItems(ui.Center).Children(func() {
			stepButton := func(key, glyph, name string, disabled bool, click func()) {
				button := ui.ButtonBase(c.Key(key)).Size(preferenceControlHeight, preferenceControlHeight).
					Center().Radius(5).Label(name + " " + label).Disabled(disabled)
				if !disabled {
					preferenceHover(button, p)
				}
				button.Children(func() {
					text := ui.Text(c, glyph).FontSize(p.textSize(16)).TextColor(p.theme.TextMuted)
					if disabled {
						text.Opacity(.42)
					}
				}).OnClick(click)
			}
			stepButton("decrease", "−", "Decrease", *value <= lo, func() { *value = max(lo, *value-step) })
			ui.Text(c, fmt.Sprintf("%d%s", *value, unit)).Label(label + " value").Width(47).
				TextAlign(ui.Center).FontFeatures("tnum")
			stepButton("increase", "+", "Increase", *value >= hi, func() { *value = min(hi, *value+step) })
		})
	})
}

func preferenceHover(button ui.Element, p tokens) {
	if button.PressedInside() || button.Hovered() {
		button.Background(p.settingsControlTrack)
	}
}

func preferenceFont(c *ui.Context, p tokens, label string, font *string, monospace bool) {
	names := []string{application.SystemFont}
	for _, family := range settingsFonts() {
		if !monospace || family.Monospaced {
			names = append(names, family.Name)
		}
	}
	if *font != "" && !slices.Contains(names, *font) {
		names = append(names, *font)
	}
	selectFont := ui.SelectBase(c, font)
	trigger := selectFont.Trigger.Label("Select "+label).Height(preferenceControlHeight).MaxWidth(230).
		Radius(5).Padding(0, gap).Gap(gap).AlignItems(ui.Center)
	if selectFont.Open() {
		trigger.Background(p.settingsControlTrack)
	} else {
		preferenceHover(trigger, p)
	}
	trigger.Children(func() {
		ui.Text(c, *font).TextColor(p.theme.TextMuted).SingleLine().Ellipsis("…")
		ui.Icon(c, chevronDownIcon).Size(12, 12).TextColor(p.theme.TextMuted).Shrink(0)
	})
	selectFont.Popup(func(panel ui.Element) {
		panel.Background(p.panel).Border(1, p.theme.Border).Radius(6).Padding(4).Children(func() {
			ui.Scroll(c).MinWidth(168).MaxHeight(252).Children(func() {
				ui.Column(c).FillWidth().Children(func() {
					for _, name := range names {
						item := selectFont.Item(name).Height(rowHeight).FillWidth().Padding(0, gap).Radius(4).AlignItems(ui.Center)
						if item.Highlighted() {
							item.Background(p.settingsSelection)
						}
						item.Children(func() { ui.Text(c, name).SingleLine().Ellipsis("…") })
					}
				})
			})
		})
	})
}
