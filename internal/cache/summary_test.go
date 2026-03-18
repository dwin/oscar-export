package cache

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSummaryFileParsesHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.000")
	if err := os.WriteFile(path, buildSummaryFileBytes(summaryVersion), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := LoadSummaryFile(path)
	if err != nil {
		t.Fatalf("LoadSummaryFile() error = %v", err)
	}
	if got.MachineID != 1452605806 {
		t.Fatalf("MachineID = %d, want %d", got.MachineID, uint32(1452605806))
	}
	if got.SessionID != 1548131700 {
		t.Fatalf("SessionID = %d, want %d", got.SessionID, uint32(1548131700))
	}
	if got.First != 1548131708000 || got.Last != 1548132184000 {
		t.Fatalf("timestamps = (%d, %d), want (%d, %d)", got.First, got.Last, int64(1548131708000), int64(1548132184000))
	}
}

func TestLoadSummaryFileRejectsUnsupportedVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.000")
	if err := os.WriteFile(path, buildSummaryFileBytes(summaryVersion+1), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := LoadSummaryFile(path)
	if err == nil {
		t.Fatal("LoadSummaryFile() error = nil, want unsupported version error")
	}
}

func buildSummaryFileBytes(version uint16) []byte {
	var buf bytes.Buffer
	mustWriteLESummary(&buf, uint32(magicNumber))
	mustWriteLESummary(&buf, version)
	mustWriteLESummary(&buf, uint16(fileTypeSummary))
	mustWriteLESummary(&buf, uint32(1452605806))
	mustWriteLESummary(&buf, uint32(1548131700))
	mustWriteLESummary(&buf, int64(1548131708000))
	mustWriteLESummary(&buf, int64(1548132184000))

	for i := 0; i < 13; i++ {
		mustWriteLESummary(&buf, uint32(0))
	}
	for i := 0; i < 2; i++ {
		mustWriteLESummary(&buf, uint32(0))
	}
	mustWriteLESummary(&buf, uint32(0))
	mustWriteLESummary(&buf, int32(0))
	for i := 0; i < 4; i++ {
		mustWriteLESummary(&buf, uint32(0))
	}
	mustWriteLESummary(&buf, uint8(0))
	mustWriteLESummary(&buf, uint8(0))
	mustWriteLESummary(&buf, int32(0))
	return buf.Bytes()
}

func mustWriteLESummary(buf *bytes.Buffer, value any) {
	if err := binary.Write(buf, binary.LittleEndian, value); err != nil {
		panic(err)
	}
}
