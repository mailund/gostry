package gostr

import (
	"reflect"
	"strings"
	"testing"
)

func checkAlphabet(
	t *testing.T, x string,
	letters []byte, expectedMapped []byte) {

	alpha := NewAlphabet(x)
	if alpha.Size() != len(letters)+1 { // +1 for sentinel
		t.Fatalf("Expected size %d, got %d", len(letters)+1, alpha.Size())
	}
	for i := 0; i < len(x); i++ {
		a := x[i]
		if !alpha.Contains(a) {
			t.Fatalf("Expected alphabet to contain %c.", a)
		}
	}
	for i, a := range letters {
		if alpha._map[a] != byte(i+1) {
			t.Fatalf("Expected %c to map to %d", a, i+1)
		}
	}
	for i := 0; i < 256; i++ {
		a := alpha._revmap[alpha._map[byte(i)]]
		if a != 0 && a != byte(i) {
			t.Fatalf("Mapping %c and then reversing doesn't give %c back", byte(i), byte(i))
		}
		b := alpha._map[alpha._revmap[byte(i)]]
		if b != 0 && b != byte(i) {
			t.Fatalf("Reverse-mapping %c and then forward doesn't give %c back", byte(i), byte(i))
		}
	}

	y, err := alpha.MapToBytesWithSentinel(x)
	if err != nil {
		t.Fatalf("Error mapping string %s", x)
	}
	if !reflect.DeepEqual(y, expectedMapped) {
		t.Fatalf("We expected the mapped string to be %v but it is %v", expectedMapped, y)
	}

	xx := strings.ReplaceAll(x, string(Sentinel), string(SentinelSymbol))
	z := alpha.RevmapBytesStripSentinel(y)
	if !reflect.DeepEqual(xx, z) {
		t.Fatalf(`Mapping back we expected "%s" but got "%s"`, x, z)
	}
	zz := alpha.RevmapBytes(y)
	if !reflect.DeepEqual(zz[:len(z)], z) {
		t.Fatalf(`Strings "%s" and "%s" should be equal`, z, zz)
	}
	if zz != z+string(SentinelSymbol) {
		t.Fatalf(`The last character in "%s" should be sentinel`, zz)
	}

	expectedInts := make([]int32, len(expectedMapped))
	for i, b := range expectedMapped {
		expectedInts[i] = int32(b)
	}
	yy, err2 := alpha.MapToIntsWithSentinel(x)
	if err2 != nil {
		t.Fatalf("Error mapping string %s", x)
	}
	if !reflect.DeepEqual(yy, expectedInts) {
		t.Fatalf("We expected the mapped string to be %v but it is %v", expectedMapped, y)
	}
}

func Test_Alphabet(t *testing.T) {
	checkAlphabet(t, "foo", []byte{'f', 'o'}, []byte{1, 2, 2, 0})

	checkAlphabet(t, "foobar", []byte{'a', 'b', 'f', 'o', 'r'}, []byte{3, 4, 4, 2, 1, 5, 0})

	// including the sentinel. Doesn't affect the alphabet, but
	// it will be included in the mapped string
	checkAlphabet(t, "foobar\x00", []byte{'a', 'b', 'f', 'o', 'r'}, []byte{3, 4, 4, 2, 1, 5, 0, 0})
	checkAlphabet(t, "foo\x00bar", []byte{'a', 'b', 'f', 'o', 'r'}, []byte{3, 4, 4, 0, 2, 1, 5, 0})

	// See what happens if we try to map a string with the wrong alphabet.
	alpha := NewAlphabet("foo")
	if _, err := alpha.MapToBytes("foobar"); err == nil {
		t.Fatalf("Expected an error when mapping to an alphabet that doesn't match")
	}
	if _, err := alpha.MapToBytesWithSentinel("foobar"); err == nil {
		t.Fatalf("Expected an error when mapping to an alphabet that doesn't match")
	}
	if _, err := alpha.MapToInts("foobar"); err == nil {
		t.Fatalf("Expected an error when mapping to an alphabet that doesn't match")
	}
	if _, err := alpha.MapToIntsWithSentinel("foobar"); err == nil {
		t.Fatalf("Expected an error when mapping to an alphabet that doesn't match")
	}
}
