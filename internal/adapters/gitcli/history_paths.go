package gitcli

import (
	"context"
	"errors"
	"path"
	"path/filepath"
	"strings"
)

func historyPlatformPath(name string) error {
	if !validGitPath(name) {
		return errors.New("historical tree contains an unsafe platform path")
	}
	if filepath.Separator == '\\' {
		for _, part := range strings.Split(name, "/") {
			if strings.TrimRight(part, " .") != part || strings.ContainsRune(part, ':') {
				return errors.New("historical path has an unsupported Windows alias")
			}
		}
	}
	return nil
}

func rejectHistoryIndexAliases(paths []string, index string) error {
	for _, record := range nulFields(index) {
		_, current, _ := strings.Cut(record, "\t")
		for _, selected := range paths {
			if current != selected && rebasePathKey(current) == rebasePathKey(selected) {
				return errors.New("historical path aliases a different current tracked path")
			}
		}
	}
	return nil
}

func (g *Executor) validateHistoricalCheckoutPaths(ctx context.Context, root, revision string) error {
	out, err := g.runInputWithEnvLimit(ctx, root, "", nil, 16<<20, "ls-tree", "-r", "-z", "--name-only", revision)
	if err != nil {
		return err
	}
	files, parents := map[string]bool{}, map[string]bool{}
	for _, name := range nulFields(out.stdout) {
		if err := historyPlatformPath(name); err != nil {
			return err
		}
		key := rebasePathKey(name)
		if files[key] || parents[key] {
			return errors.New("historical checkout contains colliding platform paths")
		}
		for parent := path.Dir(key); parent != "."; parent = path.Dir(parent) {
			if files[parent] {
				return errors.New("historical checkout contains a file/directory path alias")
			}
			parents[parent] = true
		}
		files[key] = true
	}
	return nil
}
