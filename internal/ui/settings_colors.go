package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"vela/internal/application"
)

type statusColorEntry struct {
	name, code, light, dark string
	override                func(*application.FileColors) *string
}

var statusColors = [...]statusColorEntry{
	{"Added", "A", "067D17", "73C991", func(c *application.FileColors) *string { return &c.Added }},
	{"Added in not active changelist", "A-inactive", "067D17", "73C991", func(c *application.FileColors) *string { return &c.AddedInactive }},
	{"Changelist conflict", "U", "DE1B2E", "FF6B78", func(c *application.FileColors) *string { return &c.Conflict }},
	{"Copied", "C", "067D17", "73C991", func(c *application.FileColors) *string { return &c.Copied }},
	{"Deleted", "D", "6C707E", "A1A7B4", func(c *application.FileColors) *string { return &c.Deleted }},
	{"Deleted from file system", "D-fs", "6C707E", "A1A7B4", func(c *application.FileColors) *string { return &c.DeletedFromFileSystem }},
	{"External (svn)", "external", "72A038", "A9CA67", func(c *application.FileColors) *string { return &c.External }},
	{"Have changed descendants", "changed-descendants", "0033B3", "80A9FF", func(c *application.FileColors) *string { return &c.ChangedDescendants }},
	{"Have immediate changed children", "changed-children", "0033B3", "80A9FF", func(c *application.FileColors) *string { return &c.ChangedChildren }},
	{"Hijacked", "hijacked", "B28C00", "E0B85E", func(c *application.FileColors) *string { return &c.Hijacked }},
	{"Ignored", "I", "8C4F00", "DBAC78", func(c *application.FileColors) *string { return &c.Ignored }},
	{"Merged", "merged", "635FD8", "B4A0F7", func(c *application.FileColors) *string { return &c.Merged }},
	{"Merged with conflicts", "merge-conflict", "DE1B2E", "FF6B78", func(c *application.FileColors) *string { return &c.MergedConflicts }},
	{"Merged with property conflicts", "merge-property-conflict", "DE1B2E", "FF6B78", func(c *application.FileColors) *string { return &c.MergedPropertyConflicts }},
	{"Merged with text and property conflicts", "merge-text-property-conflict", "DE1B2E", "FF6B78", func(c *application.FileColors) *string { return &c.MergedTextPropertyConflicts }},
	{"Modified", "M", "0033B3", "80A9FF", func(c *application.FileColors) *string { return &c.Modified }},
	{"Modified in not active changelist", "M-inactive", "0033B3", "80A9FF", func(c *application.FileColors) *string { return &c.ModifiedInactive }},
	{"Obsolete", "obsolete", "7C7C00", "B8B868", func(c *application.FileColors) *string { return &c.Obsolete }},
	{"Obstructed (svn)", "obstructed", "727238", "B4B47D", func(c *application.FileColors) *string { return &c.Obstructed }},
	{"Renamed", "R", "0033B3", "80A9FF", func(c *application.FileColors) *string { return &c.Renamed }},
	{"Replaced (svn)", "replaced", "067D17", "73C991", func(c *application.FileColors) *string { return &c.Replaced }},
	{"Suppressed", "suppressed", "", "", func(c *application.FileColors) *string { return &c.Suppressed }},
	{"Switched", "switched", "08978F", "62CBC3", func(c *application.FileColors) *string { return &c.Switched }},
	{"Unknown", "?", "C93168", "F28CAF", func(c *application.FileColors) *string { return &c.Unknown }},
	{"Up to date", "", "", "", func(c *application.FileColors) *string { return &c.UpToDate }},
}

func (s settingsDraft) invalidColor() bool {
	for _, entry := range statusColors {
		field := *entry.override(&s.vcs.Colors)
		if strings.TrimSpace(field) == "" {
			continue
		}
		if _, ok := application.ColorHex(field); !ok {
			return true
		}
	}
	return false
}

func (entry statusColorEntry) color(p tokens, colors *application.FileColors) ui.Color {
	if hex, ok := application.ColorHex(*entry.override(colors)); ok {
		return ui.Hex(hex)
	}
	hex := entry.light
	if p.theme.Dark {
		hex = entry.dark
	}
	if hex == "" {
		return p.theme.Text
	}
	return ui.Hex("#" + hex)
}

func fileStatusColor(p tokens, colors *application.FileColors, status string) ui.Color {
	for _, entry := range statusColors {
		if entry.code == status {
			return entry.color(p, colors)
		}
	}
	return p.theme.Text
}

func (a *Workspace) settingsColors(c *ui.Context, p tokens) {
	s := &a.settings
	colors := &s.vcs.Colors
	ui.Row(c).FillWidth().Gap(inset * 2).AlignItems(ui.Start).Children(func() {
		ui.Column(c).Width(286).Background(p.panel).Border(1, p.theme.Border).Children(func() {
			for i, entry := range statusColors {
				row := ui.ButtonBase(c.Key(entry.name)).Height(21).FillWidth().Padding(0, gap).Gap(4).
					Label("Color " + entry.name).Row().Justify(ui.Start).AlignItems(ui.Center).Children(func() {
					marker := ""
					if strings.TrimSpace(*entry.override(colors)) != "" {
						marker = "*"
					}
					ui.Text(c, marker).Width(7).TextColor(p.theme.TextMuted)
					ui.Text(c, entry.name).TextColor(entry.color(p, colors)).SingleLine().Ellipsis("…")
				})
				if s.color == i {
					row.Background(p.theme.Selection)
				}
				row.OnClick(func() { s.color = i; s.colorPicker = false })
			}
		})
		ui.Column(c).Grow(1).Gap(gap).Children(func() {
			entry := statusColors[s.color]
			field := entry.override(colors)
			color := entry.color(p, colors)
			ui.Text(c, entry.name+" status color").Bold()
			ui.Row(c).Gap(gap).AlignItems(ui.Center).Children(func() {
				swatch := ui.ButtonBase(c).Size(50, controlHeight).Radius(4).Background(color).
					Label("Choose status color").OnClick(func() { s.colorPicker = !s.colorPicker })
				ui.Popover(c, swatch, &s.colorPicker, func() {
					ui.Column(c).Padding(inset).Background(p.panel).Children(func() {
						if ui.ColorPicker(c, &color).Changed() {
							*field = colorCode(color)
						}
					})
				})
				ui.TextInput(c, field).Width(118).Placeholder(colorCode(color)).Label("Status color hex")
			})
			if strings.TrimSpace(*field) != "" {
				if _, ok := application.ColorHex(*field); !ok {
					ui.Text(c, "Enter six hexadecimal RGB digits.").TextColor(p.theme.Danger)
				}
			}
			ui.Button(c, "Restore Default").Width(124).OnClick(func() { *field = "" })
		})
	})
}

func colorCode(color ui.Color) string {
	return fmt.Sprintf("#%02X%02X%02X", color.R, color.G, color.B)
}
