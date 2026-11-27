package gitcli

import (
	"context"
	"errors"

	"vela/internal/application"
	"vela/internal/domain"
)

func (g *Executor) validatePartialLineText(ctx context.Context, repo domain.Repository, doc domain.PatchDocument) error {
	for _, cached := range []bool{false, true} {
		args := []string{"--literal-pathspecs", "check-attr", "-z"}
		if cached {
			args = append(args, "--cached")
		}
		args = append(args, "filter", "working-tree-encoding", "eol", "text", "--", doc.Path)
		raw, err := g.read(ctx, repo.Root, args...)
		if err != nil {
			return err
		}
		fields := nulFields(raw)
		if len(fields) != 12 {
			return errors.New("invalid target attributes")
		}
		for i := 2; i < len(fields); i += 3 {
			if fields[i] != "unspecified" && fields[i] != "unset" {
				return errors.New("partial lines do not support text/encoding/filter attributes")
			}
		}
	}
	file, err := g.ReadFile(ctx, repo.Root, doc.Source, domain.File{Path: doc.Path, Status: "M"})
	if err != nil {
		return err
	}
	ending, err := application.ImportedPatchLineEnding(doc)
	if err != nil {
		return err
	}
	for _, text := range []string{file.Old, file.New} {
		actual, err := application.UniformTextLineEnding(text)
		if err != nil {
			return err
		}
		if actual != "" && ending != "" && actual != ending {
			return errors.New("partial lines do not support newline conversion")
		}
	}
	return nil
}
