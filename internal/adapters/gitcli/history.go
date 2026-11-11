package gitcli

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"vela/internal/domain"
)

func parseHistory(value string, refs map[string][]string) ([]domain.Commit, error) {
	rows := nulFields(value)
	var commits []domain.Commit
	var parents [][]string
	indices := make(map[string]int)
	for i := 0; i < len(rows); {
		if rows[i] != "" || i+8 >= len(rows) {
			return nil, errors.New("invalid Git history header")
		}
		hash := rows[i+1]
		authored, err := strconv.ParseInt(rows[i+5], 10, 64)
		if err != nil {
			return nil, err
		}
		committed, err := strconv.ParseInt(rows[i+6], 10, 64)
		if err != nil {
			return nil, err
		}
		parentHashes := strings.Fields(rows[i+2])
		commit := domain.Commit{Hash: hash, Author: rows[i+3], AuthorEmail: rows[i+4],
			AuthoredAt: time.Unix(authored, 0), Timestamp: time.Unix(committed, 0),
			Subject: rows[i+7], Body: strings.TrimRight(rows[i+8], "\n"), Merge: len(parentHashes) > 1, Refs: refs[hash]}
		i += 9
		for i < len(rows) && rows[i] != "" {
			status := strings.TrimPrefix(rows[i], "\n")
			if status == "" || i+1 == len(rows) {
				return nil, errors.New("invalid Git changed file record")
			}
			file := domain.File{Status: status[:1], Path: rows[i+1]}
			i += 2
			if file.Status == "R" || file.Status == "C" {
				if i == len(rows) {
					return nil, errors.New("missing renamed path in Git history")
				}
				file.OldPath, file.Path = file.Path, rows[i]
				i++
			}
			commit.Files = append(commit.Files, file)
		}
		indices[hash] = len(commits)
		commits = append(commits, commit)
		parents = append(parents, parentHashes)
	}
	for i, hashes := range parents {
		for _, hash := range hashes {
			if parent, ok := indices[hash]; ok && parent > i {
				commits[i].Parents = append(commits[i].Parents, parent)
			}
		}
	}
	assignLanes(commits)
	return commits, nil
}

func assignLanes(commits []domain.Commit) {
	// Keep a lane reserved until its next expected commit is reached.
	var lanes []int
	pending := make(map[int]int)
	for index := range commits {
		lane, found := pending[index]
		if !found {
			lane = reserveLane(&lanes, index)
		}
		commits[index].Lane = lane
		lanes[lane] = -1
		delete(pending, index)
		for _, parent := range commits[index].Parents {
			if _, found := pending[parent]; !found {
				pending[parent] = reserveLane(&lanes, parent)
			}
		}
	}
}

func reserveLane(lanes *[]int, target int) int {
	for i, reserved := range *lanes {
		if reserved == -1 {
			(*lanes)[i] = target
			return i
		}
	}
	*lanes = append(*lanes, target)
	return len(*lanes) - 1
}
