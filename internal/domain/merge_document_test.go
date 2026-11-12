package domain

import "testing"

func TestConflictBlocks(t *testing.T) {
	text := "unchanged\r\n<<<< ours\r\nfirst\r\n|||| base\r\noriginal\r\n====\r\nsecond\r\n>>>> theirs\r\nlast"
	blocks, incomplete := ConflictBlocks(text, 4)
	if incomplete || len(blocks) != 1 || blocks[0].Current != "first\r\n" || blocks[0].Incoming != "second\r\n" {
		t.Fatalf("diff3 block: %+v %t", blocks, incomplete)
	}
	block := blocks[0]
	if resolved := text[:block.Start] + block.Incoming + text[block.End:]; resolved != "unchanged\r\nsecond\r\nlast" {
		t.Fatalf("replacement changed surrounding content: %q", resolved)
	}
	if _, incomplete := ConflictBlocks("<<<<<<< ours\nleft\n=======\nright", 7); !incomplete {
		t.Fatal("truncated conflict was not reported")
	}
}
