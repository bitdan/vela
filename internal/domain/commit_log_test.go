package domain

import (
	"reflect"
	"testing"
)

func TestReconnectCommitLogAcrossPages(t *testing.T) {
	commits := []Commit{
		{Hash: "merge", ParentHashes: []string{"main", "side"}, Parents: []int{99}},
		{Hash: "main", ParentHashes: []string{"base"}},
		{Hash: "side", ParentHashes: []string{"base"}},
		{Hash: "base"},
	}
	ReconnectCommitLog(commits)
	if !reflect.DeepEqual(commits[0].Parents, []int{1, 2}) || !reflect.DeepEqual(commits[1].Parents, []int{3}) || !reflect.DeepEqual(commits[2].Parents, []int{3}) {
		t.Fatalf("disconnected graph: %+v", commits)
	}
	if commits[1].Lane == commits[2].Lane {
		t.Fatal("parallel parents share a lane")
	}
	ReconnectCommitLog(commits)
	if len(commits[0].Parents) != 2 {
		t.Fatal("reconnection accumulated duplicate edges")
	}
}
