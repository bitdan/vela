package application

import (
	"strings"
	"testing"
)

const binaryDeltaFixture = "diff --git a/blob.bin b/blob.bin\nindex 5a4a005f23aa03302b132af8fada87e516e53ed6..1583869155b31c687aaaec3a6351d1fdc756d79c 100644\nGIT binary patch\ndelta 17\nYcmX>gcR+4J3S-1X!(_(Djfv%a06Po@AOHXW\n\ndelta 14\nVcmX>gcR+4J$|gn!zR3lQa{wzD1yTS2\n\n"

func TestImportedBinarySelectionPreservesExactPayload(t *testing.T) {
	doc, err := ParseImportedPatch(binaryDeltaFixture)
	if err != nil || len(doc.Files) != 1 {
		t.Fatalf("binary parse: %v", err)
	}
	f := doc.Files[0]
	if !f.Binary || f.Kind != "B" || f.OldSize != 3904 || f.NewSize != 3904 || len(f.Hunks) != 0 || f.Header+f.Payload != binaryDeltaFixture {
		t.Fatalf("binary metadata or bytes changed: %+v", f)
	}
	selected, err := SelectImportedPatch(binaryDeltaFixture, map[string][]int{"blob.bin": {0}})
	if err != nil || selected.Text != binaryDeltaFixture {
		t.Fatal("whole-file selection altered binary input")
	}
	if _, err := SelectImportedPatch(binaryDeltaFixture, map[string][]int{"blob.bin": {0, 1}}); err == nil {
		t.Fatal("partial binary selection accepted")
	}
}

func TestImportedBinaryRejectsMalformedOrOversizedPayload(t *testing.T) {
	for _, text := range []string{
		strings.Replace(binaryDeltaFixture, "delta 17", "delta 2097153", 1),
		strings.Replace(binaryDeltaFixture, "delta 17", "delta 16", 1),
		strings.Replace(binaryDeltaFixture, "YcmX>g", "!cmX>g", 1),
		strings.Replace(binaryDeltaFixture, "YcmX>g", "Y!!!!!", 1),
		strings.Split(binaryDeltaFixture, "delta 14")[0],
		strings.Replace(binaryDeltaFixture, "5a4a005f23aa03302b132af8fada87e516e53ed6", "5a4", 1),
		binaryDeltaFixture + "unparsed data\n",
	} {
		if _, err := ParseImportedPatch(text); err == nil {
			t.Fatal("malformed/oversized binary input accepted")
		}
	}
}

func TestBinaryDeltaSizeLimit(t *testing.T) {
	for _, tc := range []struct {
		data []byte
		want int
	}{{[]byte{0}, 0}, {[]byte{127}, 127}, {[]byte{128, 1}, 128}, {[]byte{128, 128, 128, 1}, 2097152}} {
		i := 0
		value, err := binaryDeltaSize(tc.data, &i)
		if err != nil || value != tc.want {
			t.Fatalf("delta size %v: %d %v", tc.data, value, err)
		}
	}
	for _, data := range [][]byte{{128}, {128, 128, 128, 2}, {255, 255, 255, 255, 255, 255, 255, 255, 255, 255}} {
		i := 0
		if _, err := binaryDeltaSize(data, &i); err == nil {
			t.Fatalf("oversized/truncated delta size accepted: %v", data)
		}
	}
}
