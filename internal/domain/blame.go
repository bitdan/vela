package domain

import "time"

// BlameDocument attributes the lines of one committed file version.
type BlameDocument struct {
	Path, Revision string
	Text           string // Exact committed bytes, including line endings and final newline.
	Lines          []BlameLine
}

type BlameLine struct {
	Commit, Author, Summary, Text string
	OriginalLine, Line            int
	AuthoredAt                    time.Time
}
