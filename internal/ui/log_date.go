package ui

import (
	"time"

	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

const (
	logDateToday    = "Today"
	logDateDay      = "Last 24 hours"
	logDateWeek     = "Last 7 days"
	logDateTwoWeeks = "Last 14 days"
	logDateMonth    = "Last 30 days"
	logDateAll      = "All"
	logDateCustom   = "Custom"
)

type logDateRange struct {
	preset       string
	start, end   time.Time
	followLatest bool
}

type logDatePicker struct {
	open, editingEnd bool
	draft            logDateRange
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func logDatePreset(name string, latest time.Time) logDateRange {
	r := logDateRange{preset: name, start: startOfDay(latest), end: latest.Truncate(time.Minute)}
	switch name {
	case logDateDay:
		r.start = latest.Add(-24 * time.Hour).Truncate(time.Minute)
	case logDateWeek:
		r.start = r.start.AddDate(0, 0, -6)
	case logDateTwoWeeks:
		r.start = r.start.AddDate(0, 0, -13)
	case logDateMonth:
		r.start = r.start.AddDate(0, 0, -29)
	}
	return r
}

func (r logDateRange) endTime(latest time.Time) time.Time {
	if r.followLatest {
		return latest.Truncate(time.Minute)
	}
	return r.end
}

func (r logDateRange) includes(value, latest time.Time) bool {
	return r.preset == "" || (!value.IsZero() && !value.Before(r.start) && value.Before(r.endTime(latest).Add(time.Minute)))
}

func logTimestamp(commit domain.Commit, useCommitTime bool) time.Time {
	if useCommitTime || commit.AuthoredAt.IsZero() {
		return commit.Timestamp
	}
	return commit.AuthoredAt
}

func formatLogTimestamp(timestamp, latest time.Time) string {
	timestamp = timestamp.In(latest.Location())
	if !latest.IsZero() {
		day, today := startOfDay(timestamp), startOfDay(latest)
		switch {
		case day.Equal(today):
			return "Today " + timestamp.Format("15:04")
		case day.Equal(today.AddDate(0, 0, -1)):
			return "Yesterday " + timestamp.Format("15:04")
		}
	}
	return timestamp.Format("2006/1/2 15:04")
}

func (a *Workspace) logDateButton(c *ui.Context, p tokens, label string, latest time.Time) ui.Element {
	r := a.repo()
	s := &r.datePicker
	button := ui.ButtonBase(c.Key("date-range")).Label("Date").Role(ui.RoleDisclosure).Expanded(s.open).
		Height(controlHeight).Padding(0, gap).Gap(4).Radius(5).Row().AlignItems(ui.Center).
		Children(func() {
			ui.Text(c, label).SingleLine()
			ui.Text(c, "⌄").TextColor(p.theme.TextMuted)
		}).OnClick(func() {
		s.open = !s.open
		if s.open {
			r.logPathsOpen = false
			s.draft = r.logDate
			if s.draft.preset == "" {
				s.draft = logDatePreset(logDateAll, latest)
			}
			s.editingEnd = false
		}
	})
	return button
}

func (a *Workspace) logDatePopover(c *ui.Context, p tokens, button ui.Element, latest time.Time) {
	r := a.repo()
	s := &r.datePicker
	ui.PopoverBase(c, button, &s.open, func(panel ui.Element) {
		panel.Width(595).Padding(inset).Gap(gap).Background(p.panel).
			Border(1, p.theme.Border).Radius(6).Label("Date range filter").Column().Children(func() {
			ui.Row(c).FillWidth().Gap(gap).Children(func() {
				for _, name := range []string{logDateToday, logDateDay, logDateWeek, logDateTwoWeeks, logDateMonth, logDateAll} {
					chip := ui.Button(c.Key(name), name).Role(ui.RoleToggleButton).Checked(s.draft.preset == name)
					if s.draft.preset == name {
						chip.Background(p.settingsSelection)
					}
					chip.OnClick(func() { s.draft = logDatePreset(name, latest); s.editingEnd = false })
				}
			})
			divider(c, p)
			ui.Row(c).FillWidth().Gap(inset).Children(func() {
				ui.Column(c).Grow(1).Gap(gap).Children(func() {
					dateRangeEndpoint(c.Key("start"), p, s, false, latest)
					dateRangeEndpoint(c.Key("end"), p, s, true, latest)
					ui.Checkbox(c, &s.draft.followLatest, "End follows latest commit").OnChange(func() {
						s.draft.preset = logDateCustom
						if s.draft.followLatest {
							s.editingEnd = false
						}
					})
					invalid := s.draft.preset != logDateAll && s.draft.start.After(s.draft.endTime(latest))
					if invalid {
						ui.Text(c, "End must be on or after start.").FontSize(p.textSize(smallText)).TextColor(p.theme.Danger)
					}
					ui.Row(c).FillWidth().Gap(gap).Children(func() {
						ui.Button(c, "Cancel").Label("Cancel date filter").OnClick(func() { s.open = false })
						ui.PrimaryButton(c, "Apply").Label("Apply date filter").Disabled(invalid).OnClick(func() {
							r.logDate = s.draft
							if r.logDate.preset == logDateAll {
								r.logDate = logDateRange{}
							}
							r.log.ScrollTo(0, ui.Start)
							s.open = false
						})
					})
				})
				date := &s.draft.start
				if s.editingEnd {
					date = &s.draft.end
				}
				ui.Calendar(c.Key(s.editingEnd), date).Width(294).Browse().Label("Commit date calendar").
					OnChange(func() { s.draft.preset = logDateCustom })
			})
		})
	})
	button.Expanded(s.open)
	if s.open {
		button.Background(p.theme.Accent)
	}
}

func dateRangeEndpoint(c *ui.Context, p tokens, s *logDatePicker, end bool, latest time.Time) {
	label, date, disabled := "Start", &s.draft.start, false
	if end {
		label, date, disabled = "End", &s.draft.end, s.draft.followLatest
	}
	shown := date
	if disabled {
		effective := s.draft.endTime(latest)
		shown = &effective
	}
	border := p.theme.Border
	if s.editingEnd == end {
		border = p.theme.Focus
	}
	ui.Column(c).FillWidth().Padding(gap).Gap(3).Border(1, border).Radius(5).Children(func() {
		ui.Text(c, label+" time").FontSize(p.textSize(smallText)).TextColor(p.theme.TextMuted)
		ui.Row(c).FillWidth().Gap(gap).AlignItems(ui.Center).Children(func() {
			ui.Button(c, shown.Format("2006/01/02")).Label(label + " date").Grow(1).Disabled(disabled).
				OnClick(func() { s.editingEnd = end })
			ui.TimeInput(c, shown).Label(label + " time").Disabled(disabled).OnChange(func() {
				s.draft.preset = logDateCustom
				s.editingEnd = end
			})
		})
	})
}
