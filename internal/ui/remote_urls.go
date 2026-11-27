package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

func remoteURLLines(text string) []string {
	var urls []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line != "" {
			urls = append(urls, line)
		}
	}
	return urls
}

func (a *Workspace) remoteURLsForm(c *ui.Context, p tokens, edit *remoteEditor, busy bool) {
	ui.Column(c.Key("remote-urls")).FillWidth().Gap(gap).Children(func() {
		ui.Text(c, "One URL per line. Line order is saved.").TextColor(p.theme.TextMuted)
		ui.Row(c).FillWidth().Gap(gap).Children(func() {
			ui.Column(c).Grow(1).Children(func() {
				ui.Text(c, "Fetch URLs")
				ui.TextArea(c, &edit.fetch).Label("Fetch URL list").FillWidth().Height(95).Disabled(busy)
			})
			ui.Column(c).Grow(1).Children(func() {
				ui.Text(c, "Push URLs")
				ui.TextArea(c, &edit.push).Label("Push URL list").FillWidth().Height(95).Disabled(busy)
			})
		})
		fetch, push := remoteURLLines(edit.fetch), remoteURLLines(edit.push)
		valid := domain.ValidRemoteURLLists(fetch, push)
		if !valid {
			ui.Text(c, "Enter 1–32 supported fetch URLs and up to 32 push URLs.").TextColor(p.theme.Danger)
		}
		ui.Row(c).FillWidth().Gap(gap).Justify(ui.End).Children(func() {
			ui.Button(c, "Cancel").Label("Cancel remote edit").OnClick(func() { a.remotes.editor = nil })
			ui.Button(c, "Save").Label("Save remote URLs").Disabled(!valid || busy).OnClick(func() {
				a.confirmRemote(editRemoteURLs, edit.item.Name, domain.Remote{FetchURLs: fetch, PushURLs: push})
			})
		})
	})
}
