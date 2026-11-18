package ui

import (
	"fmt"
	"strconv"

	"github.com/egoist/mygo/ui"
	"vela/internal/application"
)

func (s *patchImportDialog) patchStripDepth() (int, error) {
	if s.stripDepth == "" {
		return 1, nil
	}
	depth, err := strconv.Atoi(s.stripDepth)
	if err != nil || depth < 1 || depth > 16 {
		return 0, fmt.Errorf("choose a strip depth between 1 and 16")
	}
	return depth, nil
}

func (s *patchImportDialog) patchMappingMatches() bool {
	depth, err := s.patchStripDepth()
	return err == nil && s.doc.Strip == depth && s.doc.Directory == s.directory
}

func (s *patchImportDialog) mappedPatchPath(source string) (string, error) {
	depth, err := s.patchStripDepth()
	if err != nil {
		return "", err
	}
	return application.ResolveImportedPatchPath(source, depth, s.directory)
}

func (a *Workspace) patchRelocationControls(c *ui.Context, p tokens) {
	s := a.patchImport
	if s.input.Text == "" {
		return
	}
	busy := s.busy || s.picking || s.request != nil
	ui.Row(c).FillWidth().Gap(gap).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Strip").TextColor(p.theme.TextMuted)
		depths := make([]string, 16)
		for i := range depths {
			depths[i] = strconv.Itoa(i + 1)
		}
		if ui.Select(c, &s.stripDepth, depths).Width(68).Label("Patch strip depth").Tooltip("1 removes a/ or b/; 2 also removes the next directory").Disabled(busy).Changed() {
			a.rebuildImportedPatch()
		}
		ui.Text(c, "Target folder").TextColor(p.theme.TextMuted)
		ui.TextInput(c, &s.directory).Label("Patch destination directory").Placeholder("Repository-relative folder; empty = root").Grow(1).Tooltip("Existing directory inside this repository, using /; no absolute paths or parent traversal").Disabled(busy).OnChange(func() { a.rebuildImportedPatch() })
	})
}
