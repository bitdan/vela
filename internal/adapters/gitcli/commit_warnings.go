package gitcli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"vela/internal/application"
	"vela/internal/domain"
)

// inspectCommitWarnings examines index blobs, never the working file contents.
// Caller holds the write lock and has already verified the checkout snapshot.
func (g *Executor) inspectCommitWarnings(ctx context.Context, repo domain.Repository, index string, policy application.CommitPolicy) error {
	if !policy.WarnLarge && !policy.WarnNames {
		return nil
	}
	if policy.WarnLarge && (math.IsNaN(policy.LargeFileMB) || math.IsInf(policy.LargeFileMB, 0) || policy.LargeFileMB < 1 || policy.LargeFileMB > 100000) {
		return errors.New("large file threshold must be between 1 and 100000 MiB")
	}
	application.ReportTaskProgress(ctx, "Inspecting staged file sizes and portable names")
	staged, _, err := g.readStatus(ctx, repo.Root)
	if err != nil {
		return err
	}
	if len(staged) > 10000 {
		return errors.New("commit warnings support at most 10000 staged paths; split this commit or disable file warnings")
	}
	type entry struct{ mode, oid string }
	entries := map[string]entry{}
	spellings, collisions := map[string]string{}, map[string]bool{}
	for _, record := range nulFields(index) {
		header, name, ok := strings.Cut(record, "\t")
		fields := strings.Fields(header)
		if !ok || len(fields) != 3 || fields[2] != "0" || !validObjectID(fields[1]) {
			return errors.New("invalid or unmerged index for commit inspection")
		}
		entries[name] = entry{fields[0], fields[1]}
		if policy.WarnNames {
			parts := strings.Split(name, "/")
			for i := range parts {
				prefix := strings.Join(parts[:i+1], "/")
				fold := strings.ToLower(prefix)
				if old, ok := spellings[fold]; ok && old != prefix {
					collisions[fold] = true
				} else {
					spellings[fold] = prefix
				}
			}
		}
	}
	var objects []string
	for _, file := range staged {
		if file.Status == "D" {
			continue
		}
		value, ok := entries[file.Path]
		if !ok {
			return ErrStaleIndex
		}
		if policy.WarnLarge && (value.mode == "100644" || value.mode == "100755") {
			objects = append(objects, value.oid)
		}
	}
	slices.Sort(objects)
	objects = slices.Compact(objects)
	sizes := map[string]int64{}
	if len(objects) > 0 {
		out, err := g.runInputWithEnvLimit(canonicalHistory(ctx), repo.Root, strings.Join(objects, "\n")+"\n", nil, 2<<20, "cat-file", "--batch-check=%(objectname) %(objecttype) %(objectsize)")
		if err != nil {
			return err
		}
		rows := strings.Split(strings.TrimSuffix(out.stdout, "\n"), "\n")
		if len(rows) != len(objects) {
			return errors.New("incomplete index object size response")
		}
		for i, row := range rows {
			fields := strings.Fields(row)
			if len(fields) != 3 || fields[0] != objects[i] || fields[1] != "blob" {
				return errors.New("invalid index blob size response")
			}
			size, err := strconv.ParseInt(fields[2], 10, 64)
			if err != nil || size < 0 {
				return errors.New("invalid index blob size")
			}
			sizes[objects[i]] = size
		}
	}
	var warnings []string
	total := 0
	add := func(message string) {
		total++
		if len(warnings) < 100 {
			warnings = append(warnings, message)
		}
	}
	for _, file := range staged {
		if file.Status == "D" {
			continue
		}
		value := entries[file.Path]
		if policy.WarnLarge && float64(sizes[value.oid]) > policy.LargeFileMB*(1<<20) {
			add(fmt.Sprintf("%q: staged blob %.2f MiB exceeds %.2f MiB", file.Path, float64(sizes[value.oid])/(1<<20), policy.LargeFileMB))
		}
		if policy.WarnNames {
			bad := projectPath(file.Path, false) != nil || strings.ContainsFunc(file.Path, unicode.IsControl)
			parts := strings.Split(file.Path, "/")
			for i := range parts {
				bad = bad || collisions[strings.ToLower(strings.Join(parts[:i+1], "/"))]
			}
			if bad {
				add(fmt.Sprintf("%q: name may be incompatible with other systems (reserved characters/device name, trailing dot/space or case collision)", file.Path))
			}
		}
	}
	if total == 0 {
		return nil
	}
	if total > len(warnings) {
		warnings = append(warnings, fmt.Sprintf("%d further warnings omitted", total-len(warnings)))
	}
	slices.Sort(warnings)
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%q %q %q %q %t %t %g %q", repo.Root, repo.CurrentBranch, repo.Head, repo.IndexVersion, policy.WarnLarge, policy.WarnNames, policy.LargeFileMB, warnings))))
	if policy.WarningConsent == digest {
		return nil
	}
	return &application.CommitWarnings{Messages: warnings, Digest: digest}
}
