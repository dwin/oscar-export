package cache

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSessionInfo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Sessions.info")
	if err := os.WriteFile(path, buildSessionInfoBytes(), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := loadSessionInfo(path)
	if err != nil {
		t.Fatalf("loadSessionInfo() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("loadSessionInfo() length = %d, want 2", len(got))
	}
	if !got[123] {
		t.Fatal("loadSessionInfo()[123] = false, want true")
	}
	if got[456] {
		t.Fatal("loadSessionInfo()[456] = true, want false")
	}
}

func buildSessionInfoBytes() []byte {
	var buf bytes.Buffer
	mustWriteLESessionInfo(&buf, uint32(magicNumber))
	mustWriteLESessionInfo(&buf, uint16(fileTypeSessionInfo))
	mustWriteLESessionInfo(&buf, uint16(sessionInfoVersion))
	mustWriteLESessionInfo(&buf, int32(2))
	mustWriteLESessionInfo(&buf, uint32(123))
	mustWriteLESessionInfo(&buf, uint8(1))
	mustWriteLESessionInfo(&buf, uint32(456))
	mustWriteLESessionInfo(&buf, uint8(0))
	return buf.Bytes()
}

func mustWriteLESessionInfo(buf *bytes.Buffer, value any) {
	if err := binary.Write(buf, binary.LittleEndian, value); err != nil {
		panic(err)
	}
}
