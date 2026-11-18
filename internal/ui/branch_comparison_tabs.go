package ui

func comparisonTabTitle(s *branchComparisonDialog) string {
	return "Compare: " + s.label + " and " + s.rightLabel
}

// Dialog comparisons and editor comparisons have separate owners. Commands
// from either log carry the owning tab ID rather than relying on active state.
func (a *Workspace) comparisonTarget(t commandTarget) *branchComparisonDialog {
	if t.tabID != "" {
		if i := a.repo().tabIndex(t.tabID); i >= 0 {
			return a.repo().tabs[i].comparison
		}
		return nil
	}
	if a.branchComparison != nil {
		return a.branchComparison
	}
	r := a.repo()
	if len(r.tabs) > 0 {
		return r.tabs[r.active].comparison
	}
	return nil
}

func (a *Workspace) comparisonAlive(s *branchComparisonDialog) bool {
	if a.Model.Active().Repository.Root != s.root {
		return false
	}
	if s.tabID == "" {
		return a.branchComparison == s
	}
	for _, entry := range a.repo().tabs {
		if entry.comparison == s {
			return true
		}
	}
	return false
}

func stopComparisonReads(s *branchComparisonDialog) {
	if s.request != nil {
		stopRead(&s.request)
		s.message = "Query cancelled. Reload to retry."
	}
	for _, side := range s.sides {
		if side == nil {
			continue
		}
		if side.request != nil {
			stopRead(&side.request)
			side.message = "Query cancelled. Load More to retry."
		}
		if side.details != nil {
			stopRead(&side.details)
			side.detailMessage = "Query cancelled. Select the commit again to retry."
		}
	}
}

func (a *Workspace) stopComparisonTabReads() {
	for i := range a.repos {
		for _, entry := range a.repos[i].tabs {
			if entry.comparison != nil {
				stopComparisonReads(entry.comparison)
			}
		}
	}
}
