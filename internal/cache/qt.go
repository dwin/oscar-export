package cache

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"unicode/utf16"
)

const (
	qvariantBool     = 1
	qvariantInt      = 2
	qvariantUInt     = 3
	qvariantDouble   = 6
	qvariantString   = 10
	qvariantDate     = 14
	qvariantTime     = 15
	qvariantDateTime = 16
	qvariantFloat    = 38
	qvariantFloat46  = 135
)

type QVariant struct {
	Type  uint32
	Value any
}

type QDateValue struct {
	JulianDay int32
}

type QTimeValue struct {
	Msecs int32
}

type QDateTimeValue struct {
	Date QDateValue
	Time QTimeValue
	Spec uint8
}

type Reader struct {
	r *bytes.Reader
}

func NewReader(data []byte) *Reader {
	return &Reader{r: bytes.NewReader(data)}
}

func (r *Reader) Uint8() (uint8, error) {
	var v uint8
	err := binary.Read(r.r, binary.LittleEndian, &v)
	return v, err
}

func (r *Reader) Int8() (int8, error) {
	var v int8
	err := binary.Read(r.r, binary.LittleEndian, &v)
	return v, err
}

func (r *Reader) Bool() (bool, error) {
	v, err := r.Uint8()
	return v != 0, err
}

func (r *Reader) Uint16() (uint16, error) {
	var v uint16
	err := binary.Read(r.r, binary.LittleEndian, &v)
	return v, err
}

func (r *Reader) Int16() (int16, error) {
	var v int16
	err := binary.Read(r.r, binary.LittleEndian, &v)
	return v, err
}

func (r *Reader) Uint32() (uint32, error) {
	var v uint32
	err := binary.Read(r.r, binary.LittleEndian, &v)
	return v, err
}

func (r *Reader) Int32() (int32, error) {
	var v int32
	err := binary.Read(r.r, binary.LittleEndian, &v)
	return v, err
}

func (r *Reader) Uint64() (uint64, error) {
	var v uint64
	err := binary.Read(r.r, binary.LittleEndian, &v)
	return v, err
}

func (r *Reader) Int64() (int64, error) {
	var v int64
	err := binary.Read(r.r, binary.LittleEndian, &v)
	return v, err
}

func (r *Reader) Float32() (float32, error) {
	var v float32
	err := binary.Read(r.r, binary.LittleEndian, &v)
	return v, err
}

func (r *Reader) Float64() (float64, error) {
	var v float64
	err := binary.Read(r.r, binary.LittleEndian, &v)
	return v, err
}

func (r *Reader) QtFloat() (float32, error) {
	v, err := r.Float64()
	return float32(v), err
}

func (r *Reader) Raw(n int) ([]byte, error) {
	buf := make([]byte, n)
	_, err := io.ReadFull(r.r, buf)
	return buf, err
}

func (r *Reader) QString() (string, error) {
	n, err := r.Uint32()
	if err != nil {
		return "", err
	}
	if n == 0xffffffff || n == 0 {
		return "", nil
	}
	if n%2 != 0 {
		return "", fmt.Errorf("unexpected UTF-16 byte length %d", n)
	}
	buf, err := r.Raw(int(n))
	if err != nil {
		return "", err
	}
	u16 := make([]uint16, n/2)
	for i := range u16 {
		u16[i] = binary.LittleEndian.Uint16(buf[i*2 : i*2+2])
	}
	return string(utf16.Decode(u16)), nil
}

func (r *Reader) QVariant() (QVariant, error) {
	typ, err := r.Uint32()
	if err != nil {
		return QVariant{}, err
	}
	isNull, err := r.Uint8()
	if err != nil {
		return QVariant{}, err
	}
	if typ == 0 {
		if _, err := r.QString(); err != nil {
			return QVariant{}, err
		}
		return QVariant{Type: typ}, nil
	}
	if isNull != 0 {
		return QVariant{Type: typ}, nil
	}

	switch typ {
	case qvariantBool:
		v, err := r.Bool()
		return QVariant{Type: typ, Value: v}, err
	case qvariantInt:
		v, err := r.Int32()
		return QVariant{Type: typ, Value: v}, err
	case qvariantUInt:
		v, err := r.Uint32()
		return QVariant{Type: typ, Value: v}, err
	case qvariantDouble:
		v, err := r.Float64()
		return QVariant{Type: typ, Value: v}, err
	case qvariantFloat:
		v, err := r.Float32()
		return QVariant{Type: typ, Value: v}, err
	case qvariantFloat46:
		v, err := r.Float64()
		return QVariant{Type: qvariantFloat, Value: float32(v)}, err
	case qvariantString:
		v, err := r.QString()
		return QVariant{Type: typ, Value: v}, err
	case qvariantDate:
		v, err := r.Int32()
		return QVariant{Type: typ, Value: QDateValue{JulianDay: v}}, err
	case qvariantTime:
		v, err := r.Int32()
		return QVariant{Type: typ, Value: QTimeValue{Msecs: v}}, err
	case qvariantDateTime:
		date, err := r.Int32()
		if err != nil {
			return QVariant{}, err
		}
		clock, err := r.Int32()
		if err != nil {
			return QVariant{}, err
		}
		spec, err := r.Uint8()
		if err != nil {
			return QVariant{}, err
		}
		return QVariant{
			Type: typ,
			Value: QDateTimeValue{
				Date: QDateValue{JulianDay: date},
				Time: QTimeValue{Msecs: clock},
				Spec: spec,
			},
		}, nil
	default:
		return QVariant{}, fmt.Errorf("unsupported QVariant type %d", typ)
	}
}

func readHash[K comparable, V any](r *Reader, readKey func(*Reader) (K, error), readVal func(*Reader) (V, error)) (map[K]V, error) {
	count, err := r.Uint32()
	if err != nil {
		return nil, err
	}
	out := make(map[K]V, count)
	for i := uint32(0); i < count; i++ {
		key, err := readKey(r)
		if err != nil {
			return nil, err
		}
		value, err := readVal(r)
		if err != nil {
			return nil, err
		}
		out[key] = value
	}
	return out, nil
}

func readList[T any](r *Reader, readValue func(*Reader) (T, error)) ([]T, error) {
	count, err := r.Int32()
	if err != nil {
		return nil, err
	}
	if count < 0 {
		return nil, fmt.Errorf("unexpected negative list size %d", count)
	}
	out := make([]T, 0, count)
	for i := int32(0); i < count; i++ {
		value, err := readValue(r)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}
