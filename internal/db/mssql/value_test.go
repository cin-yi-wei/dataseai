package mssql

import "testing"

func TestNormalizeValueFormatsUniqueidentifierBytes(t *testing.T) {
	stored := []byte{0x33, 0x22, 0x11, 0x00, 0x55, 0x44, 0x77, 0x66, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}

	got := normalizeValue(stored, "uniqueidentifier")

	if got != "00112233-4455-6677-8899-aabbccddeeff" {
		t.Fatalf("normalizeValue() = %#v, want GUID string", got)
	}
}

func TestNormalizeValueLeavesOtherBytesAsString(t *testing.T) {
	got := normalizeValue([]byte("hello"), "varchar")

	if got != "hello" {
		t.Fatalf("normalizeValue() = %#v, want hello", got)
	}
}
