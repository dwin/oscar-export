package cache

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestQString(t *testing.T) {
	data := []byte{
		0x06, 0x00, 0x00, 0x00,
		0x61, 0x00, 0x62, 0x00, 0x63, 0x00,
	}

	got, err := NewReader(data).QString()
	if err != nil {
		t.Fatalf("QString() error = %v", err)
	}
	if got != "abc" {
		t.Fatalf("QString() = %q, want %q", got, "abc")
	}
}

func TestQVariantInt(t *testing.T) {
	data := []byte{
		0x02, 0x00, 0x00, 0x00,
		0x00,
		0x2a, 0x00, 0x00, 0x00,
	}

	got, err := NewReader(data).QVariant()
	if err != nil {
		t.Fatalf("QVariant() error = %v", err)
	}
	if got.Type != qvariantInt {
		t.Fatalf("QVariant().Type = %d, want %d", got.Type, qvariantInt)
	}
	value, ok := got.Value.(int32)
	if !ok {
		t.Fatalf("QVariant().Value type = %T, want int32", got.Value)
	}
	if value != 42 {
		t.Fatalf("QVariant().Value = %d, want 42", value)
	}
}

func TestQVariantQt46FloatCompat(t *testing.T) {
	var buf bytes.Buffer
	mustWriteLE(t, &buf, uint32(qvariantFloat46))
	mustWriteLE(t, &buf, uint8(0))
	mustWriteLE(t, &buf, math.Float64bits(6.5))

	got, err := NewReader(buf.Bytes()).QVariant()
	if err != nil {
		t.Fatalf("QVariant() error = %v", err)
	}
	if got.Type != qvariantFloat {
		t.Fatalf("QVariant().Type = %d, want %d", got.Type, qvariantFloat)
	}
	value, ok := got.Value.(float32)
	if !ok {
		t.Fatalf("QVariant().Value type = %T, want float32", got.Value)
	}
	if value != float32(6.5) {
		t.Fatalf("QVariant().Value = %v, want 6.5", value)
	}
}

func TestQVariantInvalid(t *testing.T) {
	data := []byte{
		0x00, 0x00, 0x00, 0x00,
		0x01,
		0xff, 0xff, 0xff, 0xff,
	}

	got, err := NewReader(data).QVariant()
	if err != nil {
		t.Fatalf("QVariant() error = %v", err)
	}
	if got.Type != 0 {
		t.Fatalf("QVariant().Type = %d, want 0", got.Type)
	}
	if got.Value != nil {
		t.Fatalf("QVariant().Value = %#v, want nil", got.Value)
	}
}

func TestReadHash(t *testing.T) {
	var buf bytes.Buffer
	mustWriteLE(t, &buf, uint32(2))
	mustWriteLE(t, &buf, uint32(10))
	mustWriteLE(t, &buf, int32(100))
	mustWriteLE(t, &buf, uint32(20))
	mustWriteLE(t, &buf, int32(200))

	got, err := readHash(NewReader(buf.Bytes()), (*Reader).Uint32, (*Reader).Int32)
	if err != nil {
		t.Fatalf("readHash() error = %v", err)
	}
	if len(got) != 2 || got[10] != 100 || got[20] != 200 {
		t.Fatalf("readHash() = %#v, want {10:100,20:200}", got)
	}
}

func TestReadList(t *testing.T) {
	var buf bytes.Buffer
	mustWriteLE(t, &buf, int32(3))
	mustWriteLE(t, &buf, uint32(7))
	mustWriteLE(t, &buf, uint32(8))
	mustWriteLE(t, &buf, uint32(9))

	got, err := readList(NewReader(buf.Bytes()), (*Reader).Uint32)
	if err != nil {
		t.Fatalf("readList() error = %v", err)
	}
	want := []uint32{7, 8, 9}
	if len(got) != len(want) {
		t.Fatalf("readList() length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("readList()[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func mustWriteLE(t *testing.T, buf *bytes.Buffer, value any) {
	t.Helper()
	if err := binary.Write(buf, binary.LittleEndian, value); err != nil {
		t.Fatalf("binary.Write(%T) error = %v", value, err)
	}
}
