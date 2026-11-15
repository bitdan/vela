package application

import (
	"crypto/rand"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"vela/internal/domain"
)

type Changelist struct {
	ID, Name string
	Paths    []string `json:",omitempty"`
}
type ChangelistState struct {
	Active string
	Lists  []Changelist
}

func CloneChangelists(s ChangelistState) ChangelistState {
	s.Lists = slices.Clone(s.Lists)
	for i := range s.Lists {
		s.Lists[i].Paths = slices.Clone(s.Lists[i].Paths)
	}
	return s
}

func validChangelistName(name string) bool {
	return name != "" && len(name) <= 200 && utf8.ValidString(name) && !strings.ContainsAny(name, "\x00\r\n")
}

// Normalize reconciles whole-file membership after a refresh. Renames carry
// the old path's list; newly changed paths join the active list exactly once.
func (s *ChangelistState) Normalize(repo domain.Repository) {
	old := CloneChangelists(*s)
	s.Lists = []Changelist{{ID: "default", Name: "Default"}}
	for _, list := range old.Lists {
		list.Name = strings.TrimSpace(list.Name)
		if list.ID == "" || list.ID == "default" || !validChangelistName(list.Name) || len(s.Lists) >= 100 || slices.ContainsFunc(s.Lists, func(c Changelist) bool { return c.ID == list.ID || strings.EqualFold(c.Name, list.Name) }) {
			continue
		}
		s.Lists = append(s.Lists, Changelist{ID: list.ID, Name: list.Name})
	}
	if !slices.ContainsFunc(s.Lists, func(c Changelist) bool { return c.ID == s.Active }) {
		s.Active = "default"
	}
	owners := map[string]string{}
	for _, list := range old.Lists {
		if !slices.ContainsFunc(s.Lists, func(c Changelist) bool { return c.ID == list.ID }) {
			continue
		}
		for _, path := range list.Paths {
			if owners[path] == "" {
				owners[path] = list.ID
			}
		}
	}
	changes := slices.Concat(repo.Working, repo.Staged)
	renames := map[string]string{}
	for _, file := range changes {
		if file.OldPath != "" {
			renames[file.Path] = file.OldPath
		}
	}
	assigned := map[string]bool{}
	for _, file := range changes {
		if assigned[file.Path] {
			continue
		}
		assigned[file.Path] = true
		id := owners[file.Path]
		if id == "" && renames[file.Path] != "" {
			id = owners[renames[file.Path]]
		}
		if id == "" {
			id = s.Active
		}
		i := slices.IndexFunc(s.Lists, func(c Changelist) bool { return c.ID == id })
		s.Lists[i].Paths = append(s.Lists[i].Paths, file.Path)
	}
	for i := range s.Lists {
		slices.Sort(s.Lists[i].Paths)
	}
}

func (s *ChangelistState) Owner(path string) string {
	for _, list := range s.Lists {
		if slices.Contains(list.Paths, path) {
			return list.ID
		}
	}
	return ""
}

func (s *ChangelistState) Create(name string) (string, error) {
	name = strings.TrimSpace(name)
	if len(s.Lists) >= 100 || !validChangelistName(name) || slices.ContainsFunc(s.Lists, func(c Changelist) bool { return strings.EqualFold(c.Name, name) }) {
		return "", errors.New("use a unique single-line changelist name up to 200 bytes (maximum 100 lists)")
	}
	id := rand.Text()
	s.Lists = append(s.Lists, Changelist{ID: id, Name: name})
	return id, nil
}

func (s *ChangelistState) Rename(id, name string) error {
	name = strings.TrimSpace(name)
	index := slices.IndexFunc(s.Lists, func(c Changelist) bool { return c.ID == id })
	if index < 0 || id == "default" || !validChangelistName(name) || slices.ContainsFunc(s.Lists, func(c Changelist) bool { return c.ID != id && strings.EqualFold(c.Name, name) }) {
		return errors.New("select a named list and use a unique single-line name up to 200 bytes")
	}
	s.Lists[index].Name = name
	return nil
}

func (s *ChangelistState) Remove(id string) error {
	index := slices.IndexFunc(s.Lists, func(c Changelist) bool { return c.ID == id })
	if index < 0 || id == "default" {
		return errors.New("Default cannot be removed")
	}
	paths := slices.Clone(s.Lists[index].Paths)
	s.Lists = slices.Delete(s.Lists, index, index+1)
	def := slices.IndexFunc(s.Lists, func(c Changelist) bool { return c.ID == "default" })
	s.Lists[def].Paths = append(s.Lists[def].Paths, paths...)
	slices.Sort(s.Lists[def].Paths)
	if s.Active == id {
		s.Active = "default"
	}
	return nil
}

func (s *ChangelistState) Move(paths []string, id string) error {
	if !slices.ContainsFunc(s.Lists, func(c Changelist) bool { return c.ID == id }) {
		return errors.New("select an existing changelist")
	}
	for _, path := range paths {
		if s.Owner(path) == "" {
			return errors.New("changed file is no longer in the changelists")
		}
	}
	for i := range s.Lists {
		s.Lists[i].Paths = slices.DeleteFunc(s.Lists[i].Paths, func(path string) bool { return slices.Contains(paths, path) })
		if s.Lists[i].ID == id {
			s.Lists[i].Paths = append(s.Lists[i].Paths, paths...)
			slices.Sort(s.Lists[i].Paths)
			s.Lists[i].Paths = slices.Compact(s.Lists[i].Paths)
		}
	}
	return nil
}
