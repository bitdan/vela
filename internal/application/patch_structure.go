package application

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"vela/internal/domain"
)

var importedIndexHeader = regexp.MustCompile(`^index [0-9a-f]+\.\.[0-9a-f]+(?: (100644|100755))?$`)

func patchPathToken(token string) (string, error) {
	token = strings.TrimSuffix(token, "\t")
	if strings.HasPrefix(token, "\"") {
		return strconv.Unquote(token)
	}
	return token, nil
}

// Match a Git header against known endpoints. Unquoted names may contain
// spaces; C-quoted names may contain octal UTF-8 escapes.
func importedDiffHeader(line, oldPath, newPath string) bool {
	rest, ok := strings.CutPrefix(line, "diff --git ")
	if !ok {
		return false
	}
	oldToken := "a/" + oldPath
	if strings.HasPrefix(rest, "\"") {
		end := 1
		for end < len(rest) {
			if rest[end] == '\\' {
				end += 2
				continue
			}
			if rest[end] == '"' {
				break
			}
			end++
		}
		if end >= len(rest) {
			return false
		}
		decoded, err := strconv.Unquote(rest[:end+1])
		if err != nil || decoded != oldToken || end+1 >= len(rest) || rest[end+1] != ' ' {
			return false
		}
		rest = rest[end+2:]
	} else {
		var yes bool
		rest, yes = strings.CutPrefix(rest, oldToken+" ")
		if !yes {
			return false
		}
	}
	newToken, err := patchPathToken(rest)
	return err == nil && newToken == "b/"+newPath
}

func importedSamePath(line string) (string, error) {
	rest := strings.TrimPrefix(line, "diff --git ")
	if strings.HasPrefix(rest, "\"") {
		for i := 1; i < len(rest); i++ {
			if rest[i] == '\\' {
				i++
				continue
			}
			if rest[i] == '"' {
				name, err := strconv.Unquote(rest[:i+1])
				if err == nil && strings.HasPrefix(name, "a/") && importedDiffHeader(line, name[2:], name[2:]) {
					return name[2:], nil
				}
				break
			}
		}
	} else {
		for i := 0; i < len(rest); i++ {
			if strings.HasPrefix(rest[i:], " b/") && strings.HasPrefix(rest, "a/") {
				name := rest[2:i]
				if importedDiffHeader(line, name, name) {
					return name, nil
				}
			}
		}
	}
	return "", errors.New("added/deleted patch endpoints must identify the same path")
}

func parseImportedFile(raw string) (domain.PatchDocument, string, error) {
	bad := func() (domain.PatchDocument, string, error) {
		return domain.PatchDocument{}, "", errors.New("unsupported or inconsistent Git text patch headers")
	}
	lines := strings.Split(strings.TrimSuffix(raw, "\n"), "\n")
	if len(lines) < 2 {
		return bad()
	}
	file := domain.PatchDocument{}
	mode := ""
	i := 1
	if strings.HasPrefix(lines[i], "old mode ") {
		if i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "new mode ") {
			return bad()
		}
		file.OldMode, file.NewMode = strings.TrimPrefix(lines[i], "old mode "), strings.TrimPrefix(lines[i+1], "new mode ")
		if !regularImportedMode(file.OldMode) || !regularImportedMode(file.NewMode) || file.OldMode == file.NewMode {
			return bad()
		}
		file.Kind, mode = "P", file.OldMode
		i += 2
	}
	if i < len(lines) && (strings.HasPrefix(lines[i], "new file mode ") || strings.HasPrefix(lines[i], "deleted file mode ")) {
		if file.Kind != "" {
			return bad()
		}
		prefix := "new file mode "
		file.Kind = "A"
		if strings.HasPrefix(lines[i], "deleted") {
			prefix = "deleted file mode "
			file.Kind = "D"
		}
		mode = strings.TrimPrefix(lines[i], prefix)
		i++
		if mode != "100644" && mode != "100755" {
			return bad()
		}
	}
	similarity := -1
	if i < len(lines) && strings.HasPrefix(lines[i], "similarity index ") {
		if file.Kind != "" && file.Kind != "P" {
			return bad()
		}
		value := strings.TrimPrefix(lines[i], "similarity index ")
		if !strings.HasSuffix(value, "%") {
			return bad()
		}
		var err error
		similarity, err = strconv.Atoi(strings.TrimSuffix(value, "%"))
		if err != nil || similarity < 0 || similarity > 100 {
			return bad()
		}
		i++
		if i+1 >= len(lines) || !strings.HasPrefix(lines[i], "rename from ") || !strings.HasPrefix(lines[i+1], "rename to ") {
			return bad()
		}
		file.Kind = "R"
		file.OldPath, err = patchPathToken(strings.TrimPrefix(lines[i], "rename from "))
		if err != nil {
			return bad()
		}
		file.Path, err = patchPathToken(strings.TrimPrefix(lines[i+1], "rename to "))
		if err != nil || file.Path == file.OldPath {
			return bad()
		}
		i += 2
	}
	hasIndex := i < len(lines) && strings.HasPrefix(lines[i], "index ")
	if hasIndex {
		match := importedIndexHeader.FindStringSubmatch(lines[i])
		if match == nil {
			return bad()
		}
		if file.Kind == "A" || file.Kind == "D" {
			if match[1] != "" && match[1] != mode {
				return bad()
			}
		} else if file.OldMode != "" {
			if match[1] != "" {
				return bad()
			}
		} else {
			mode = match[1]
			if mode == "" {
				return bad()
			}
		}
		i++
	} else if file.Kind != "R" && file.Kind != "P" {
		return bad()
	}
	if file.OldMode == "" && file.NewMode == "" {
		switch file.Kind {
		case "A":
			file.NewMode = mode
		case "D":
			file.OldMode = mode
		default:
			file.OldMode, file.NewMode = mode, mode
		}
	}
	if i < len(lines) && lines[i] == "GIT binary patch" {
		if !hasIndex {
			return bad()
		}
		ids := strings.Split(strings.Fields(lines[i-1])[1], "..")
		if len(ids) != 2 || len(ids[0]) != 40 && len(ids[0]) != 64 || len(ids[1]) != len(ids[0]) {
			return bad()
		}
		file.Binary = true
		if file.Kind == "" {
			file.Kind = "B"
		}
		if file.Kind != "R" {
			var err error
			file.Path, err = importedSamePath(lines[0])
			if err != nil {
				return file, "", err
			}
		}
		file.Payload = strings.Join(lines[i:], "\n") + "\n"
		var err error
		file.OldSize, file.NewSize, err = validateImportedBinary(file.Payload)
		if err != nil {
			return file, "", err
		}
	} else if i < len(lines) {
		if i+2 >= len(lines) || !strings.HasPrefix(lines[i], "--- ") || !strings.HasPrefix(lines[i+1], "+++ ") {
			return bad()
		}
		oldName, err := patchPathToken(strings.TrimPrefix(lines[i], "--- "))
		if err != nil {
			return bad()
		}
		newName, err := patchPathToken(strings.TrimPrefix(lines[i+1], "+++ "))
		if err != nil {
			return bad()
		}
		switch file.Kind {
		case "A":
			if oldName != "/dev/null" || !strings.HasPrefix(newName, "b/") {
				return bad()
			}
			file.Path = newName[2:]
		case "D":
			if newName != "/dev/null" || !strings.HasPrefix(oldName, "a/") {
				return bad()
			}
			file.Path = oldName[2:]
		case "R":
			if oldName != "a/"+file.OldPath || newName != "b/"+file.Path {
				return bad()
			}
		default:
			if !strings.HasPrefix(oldName, "a/") || !strings.HasPrefix(newName, "b/") || oldName[2:] != newName[2:] {
				return bad()
			}
			file.Path = newName[2:]
		}
		i += 2
		// Reuse the independent hunk counter/newline-marker validation without
		// loosening the Stage/Unstage/Discard Hunk parser's existing scope.
		body := strings.Join(lines[i:], "\n") + "\n"
		parsed, err := ParseTextPatch("diff --git a/file b/file\nindex a..b 100644\n--- a/file\n+++ b/file\n"+body, file.Path)
		if err != nil {
			return file, "", err
		}
		file.Hunks = parsed.Hunks
		for _, h := range file.Hunks {
			if file.Kind == "A" && h.OldCount != 0 || file.Kind == "D" && h.NewCount != 0 {
				return bad()
			}
		}
	} else if file.Kind == "A" || file.Kind == "D" || file.Kind == "P" {
		var err error
		file.Path, err = importedSamePath(lines[0])
		if err != nil {
			return file, "", err
		}
	} else if file.Kind != "R" || similarity != 100 {
		return bad()
	}
	oldPath := file.Path
	if file.Kind == "R" {
		oldPath = file.OldPath
	}
	if !importedDiffHeader(lines[0], oldPath, file.Path) {
		return bad()
	}
	for _, name := range []string{file.Path, oldPath} {
		if _, err := ResolveImportedPatchPath(name, 1, ""); err != nil {
			return file, "", fmt.Errorf("invalid imported path: %w", err)
		}
	}
	file.Header = strings.Join(lines[:i], "\n") + "\n"
	return file, mode, nil
}

// ImportedPatchUnits treats structural operations as one indivisible file.
func ImportedPatchUnits(file domain.PatchDocument) int {
	if file.Kind != "" {
		return 1
	}
	return len(file.Hunks)
}

func regularImportedMode(mode string) bool { return mode == "100644" || mode == "100755" }
