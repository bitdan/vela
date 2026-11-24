package application

import (
	"strings"
	"testing"
)

func TestValidateSessionDrafts(t *testing.T) {
	for _, kind := range []string{"valid", "empty", "text-limit", "base-limit", "total-limit", "count-limit", "traversal", "wrong-source", "no-version", "nul", "encoding", "eol"} {
		t.Run(kind, func(t *testing.T) {
			d := &EditorDraftSnapshot{Text: "draft", BaseText: "base", DiskVersion: "v1", LineEnding: "\n"}
			tab := TabSnapshot{Source: "project", Path: "文件 name.txt", Code: true, Draft: d}
			s := WorkspaceSnapshot{Repositories: []RepositorySnapshot{{Root: "C:/repo", Tabs: []TabSnapshot{tab}}}}
			switch kind {
			case "empty":
				d.Text = ""
			case "text-limit":
				d.Text = strings.Repeat("x", MaxEditorDraftBytes+1)
			case "base-limit":
				d.BaseText = strings.Repeat("x", MaxEditorDraftBytes+1)
			case "total-limit":
				d.Text = strings.Repeat("x", MaxEditorDraftBytes)
				d.BaseText = d.Text
				for i := 0; i < 9; i++ {
					r := s.Repositories[0]
					r.Root += strings.Repeat("x", i+1)
					s.Repositories = append(s.Repositories, r)
				}
			case "count-limit":
				for i := 0; i < 101; i++ {
					r := s.Repositories[0]
					r.Root += strings.Repeat("x", i+1)
					s.Repositories = append(s.Repositories, r)
				}
			case "traversal":
				s.Repositories[0].Tabs[0].Path = "../outside"
			case "wrong-source":
				s.Repositories[0].Tabs[0].Source = "working"
			case "no-version":
				d.DiskVersion = ""
			case "nul":
				d.Text = "bad\x00"
			case "encoding":
				d.Text = string([]byte{255})
			case "eol":
				d.LineEnding = "\r"
			}
			err := ValidateSessionDrafts(s)
			if (err == nil) != (kind == "valid" || kind == "empty") {
				t.Fatalf("%s: %v", kind, err)
			}
		})
	}
}
