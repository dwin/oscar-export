package cache

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEventsFileParsesHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.001")
	if err := os.WriteFile(path, buildEventFileBytes(eventsVersion), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := LoadEventsFile(path)
	if err != nil {
		t.Fatalf("LoadEventsFile() error = %v", err)
	}
	if len(got.Lists) != 0 {
		t.Fatalf("LoadEventsFile().Lists length = %d, want 0", len(got.Lists))
	}
}

func TestLoadEventsFileRejectsUnsupportedVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.001")
	if err := os.WriteFile(path, buildEventFileBytes(eventsVersion+1), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := LoadEventsFile(path)
	if err == nil {
		t.Fatal("LoadEventsFile() error = nil, want unsupported version error")
	}
}

func buildEventFileBytes(version uint16) []byte {
	var buf bytes.Buffer
	mustWriteLEEvents(&buf, uint32(magicNumber))
	mustWriteLEEvents(&buf, version)
	mustWriteLEEvents(&buf, uint16(fileTypeData))
	mustWriteLEEvents(&buf, uint32(1452605806))
	mustWriteLEEvents(&buf, uint32(1548131700))
	mustWriteLEEvents(&buf, int64(1548131708000))
	mustWriteLEEvents(&buf, int64(1548132184000))
	mustWriteLEEvents(&buf, uint16(0))
	mustWriteLEEvents(&buf, uint16(machineTypeCPAP))
	mustWriteLEEvents(&buf, uint32(2))
	mustWriteLEEvents(&buf, uint16(0))
	mustWriteLEEvents(&buf, int16(0))
	return buf.Bytes()
}

func mustWriteLEEvents(buf *bytes.Buffer, value any) {
	if err := binary.Write(buf, binary.LittleEndian, value); err != nil {
		panic(err)
	}
}
