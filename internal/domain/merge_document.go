package domain

import "strings"

type MergeDocument struct {
	Conflict                  MergeConflict
	Current, Incoming, Result string
	WorktreeVersion, Head     string
	MergeHeads                []string
	Rebase                    RebaseState
	Sequencer                 SequencerState
	MarkerSize                int
}

type ConflictBlock struct {
	Start, End        int
	Current, Incoming string
}

// ConflictBlocks locates Git conflict marker ranges in the editable result.
// Incomplete marker groups are reported separately to prevent staging them.
func ConflictBlocks(text string, size int) (blocks []ConflictBlock, incomplete bool) {
	if size < 1 {
		size = 7
	}
	start, base, separator, end := strings.Repeat("<", size), strings.Repeat("|", size), strings.Repeat("=", size), strings.Repeat(">", size)
	matches := func(line, marker string) bool {
		return line == marker || strings.HasPrefix(line, marker+" ") || strings.HasPrefix(line, marker+"\t")
	}
	var block ConflictBlock
	phase, offset, content := 0, 0, 0
	for line := range strings.SplitAfterSeq(text, "\n") {
		clean := strings.TrimRight(line, "\r\n")
		next := offset + len(line)
		switch {
		case matches(clean, start):
			incomplete = incomplete || phase != 0
			block = ConflictBlock{Start: offset}
			content, phase = next, 1
		case matches(clean, base):
			if phase == 1 {
				block.Current = text[content:offset]
				phase = 2
			} else if phase != 0 {
				incomplete = true
			}
		case clean == separator:
			if phase == 1 {
				block.Current = text[content:offset]
			}
			if phase == 1 || phase == 2 {
				content, phase = next, 3
			} else if phase != 0 {
				incomplete = true
			}
		case matches(clean, end):
			if phase == 3 {
				block.Incoming, block.End = text[content:offset], next
				blocks = append(blocks, block)
			} else {
				incomplete = true
			}
			phase = 0
		}
		offset = next
	}
	return blocks, incomplete || phase != 0
}
