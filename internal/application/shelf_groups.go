package application

import (
	"crypto/rand"
	"errors"
	"maps"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

type ShelfGroup struct{ ID, Name string }
type ShelfGrouping struct {
	Groups      []ShelfGroup      `json:",omitempty"`
	Assignments map[string]string `json:",omitempty"`
	Filter      string            `json:",omitempty"` // empty=all, ungrouped=default, otherwise stable group ID
}

func CloneShelfGrouping(s ShelfGrouping) ShelfGrouping {
	s.Groups = slices.Clone(s.Groups)
	s.Assignments = maps.Clone(s.Assignments)
	return s
}
func shelfMetadataText(s string, limit int) bool {
	return s != "" && utf8.ValidString(s) && utf8.RuneCountInString(s) <= limit && !strings.ContainsFunc(s, unicode.IsControl)
}
func (s ShelfGrouping) Has(id string) bool {
	return slices.ContainsFunc(s.Groups, func(g ShelfGroup) bool { return g.ID == id })
}
func (s ShelfGrouping) Name(id string) string {
	if id == "" {
		return "All Shelves"
	}
	if id == "ungrouped" {
		return "Ungrouped"
	}
	for _, g := range s.Groups {
		if g.ID == id {
			return g.Name
		}
	}
	return "Unknown Group"
}
func (s ShelfGrouping) Validate() error {
	if len(s.Groups) > 100 || len(s.Assignments) > 10000 {
		return errors.New("Shelf grouping limit: 100 groups / 10,000 assignments")
	}
	seenIDs, seenNames := map[string]bool{}, []string{"All Shelves", "Ungrouped"}
	for _, g := range s.Groups {
		if !shelfMetadataText(g.ID, 128) || g.ID == "ungrouped" || seenIDs[g.ID] || !shelfMetadataText(g.Name, 80) || g.Name != strings.TrimSpace(g.Name) || slices.ContainsFunc(seenNames, func(n string) bool { return strings.EqualFold(n, g.Name) }) {
			return errors.New("invalid or duplicate Shelf group name/identity")
		}
		seenIDs[g.ID] = true
		seenNames = append(seenNames, g.Name)
	}
	if s.Filter != "" && s.Filter != "ungrouped" && !s.Has(s.Filter) {
		return errors.New("unknown Shelf group filter")
	}
	for shelf, group := range s.Assignments {
		if !shelfMetadataText(shelf, 256) || !s.Has(group) {
			return errors.New("invalid Shelf group assignment")
		}
	}
	return nil
}
func (s *ShelfGrouping) Create(name string) (string, error) {
	next := CloneShelfGrouping(*s)
	id := "group-" + rand.Text()
	next.Groups = append(next.Groups, ShelfGroup{ID: id, Name: strings.TrimSpace(name)})
	if err := next.Validate(); err != nil {
		return "", err
	}
	*s = next
	return id, nil
}
func (s *ShelfGrouping) Rename(id, name string) error {
	next := CloneShelfGrouping(*s)
	i := slices.IndexFunc(next.Groups, func(g ShelfGroup) bool { return g.ID == id })
	if i < 0 {
		return errors.New("select a named Shelf group")
	}
	next.Groups[i].Name = strings.TrimSpace(name)
	if err := next.Validate(); err != nil {
		return err
	}
	*s = next
	return nil
}
func (s *ShelfGrouping) Delete(id string) error {
	if !s.Has(id) {
		return errors.New("select a named Shelf group")
	}
	next := CloneShelfGrouping(*s)
	next.Groups = slices.DeleteFunc(next.Groups, func(g ShelfGroup) bool { return g.ID == id })
	for shelf, group := range next.Assignments {
		if group == id {
			delete(next.Assignments, shelf)
		}
	}
	if next.Filter == id {
		next.Filter = "ungrouped"
	}
	*s = next
	return nil
}
func (s *ShelfGrouping) Move(shelf, group string) error {
	next := CloneShelfGrouping(*s)
	if next.Assignments == nil {
		next.Assignments = map[string]string{}
	}
	if group == "" {
		delete(next.Assignments, shelf)
	} else {
		next.Assignments[shelf] = group
	}
	if err := next.Validate(); err != nil {
		return err
	}
	*s = next
	return nil
}
func ValidateSessionShelfGroups(session WorkspaceSnapshot) error {
	for _, r := range session.Repositories {
		if r.ShelfGroups != nil {
			if err := r.ShelfGroups.Validate(); err != nil {
				return err
			}
		}
	}
	return nil
}
