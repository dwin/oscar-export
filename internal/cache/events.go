package cache

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

const eventsVersion = 10

func LoadEventsFile(path string) (*SessionEvents, error) {
	// #nosec G304 -- path is derived from the selected OSCAR dataset layout.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 42 {
		return nil, fmt.Errorf("event file %s was too short", path)
	}
	header := NewReader(data[:42])
	magic, err := header.Uint32()
	if err != nil {
		return nil, err
	}
	version, err := header.Uint16()
	if err != nil {
		return nil, err
	}
	fileType, err := header.Uint16()
	if err != nil {
		return nil, err
	}
	if _, err := header.Uint32(); err != nil { // machine id
		return nil, err
	}
	if _, err := header.Uint32(); err != nil { // session id
		return nil, err
	}
	if _, err := header.Int64(); err != nil { // first
		return nil, err
	}
	if _, err := header.Int64(); err != nil { // last
		return nil, err
	}
	compression, err := header.Uint16()
	if err != nil {
		return nil, err
	}
	if _, err := header.Uint16(); err != nil { // machine type
		return nil, err
	}
	expectedSize, err := header.Uint32()
	if err != nil {
		return nil, err
	}
	if _, err := header.Uint16(); err != nil { // CRC16
		return nil, err
	}

	if magic != magicNumber {
		return nil, fmt.Errorf("unexpected event magic 0x%x in %s", magic, path)
	}
	if version != eventsVersion {
		return nil, fmt.Errorf("unsupported event version %d in %s", version, path)
	}
	if fileType != fileTypeData {
		return nil, fmt.Errorf("unexpected event file type %d in %s", fileType, path)
	}

	payload := data[42:]
	if compression > 0 {
		if len(payload) < 4 {
			return nil, fmt.Errorf("compressed event payload in %s was too short", path)
		}
		zr, err := zlib.NewReader(bytes.NewReader(payload[4:]))
		if err != nil {
			return nil, fmt.Errorf("open compressed payload %s: %w", path, err)
		}
		payload, err = io.ReadAll(zr)
		if err != nil {
			if closeErr := zr.Close(); closeErr != nil {
				return nil, fmt.Errorf("close compressed payload %s after read error: %w", path, closeErr)
			}
			return nil, fmt.Errorf("read compressed payload %s: %w", path, err)
		}
		if err := zr.Close(); err != nil {
			return nil, fmt.Errorf("close compressed payload %s: %w", path, err)
		}
		if uint64(len(payload)) != uint64(expectedSize) {
			return nil, fmt.Errorf("event payload size %d did not match expected size %d for %s", len(payload), expectedSize, path)
		}
	}

	reader := NewReader(payload)
	channelCount, err := reader.Int16()
	if err != nil {
		return nil, err
	}
	if channelCount < 0 {
		return nil, fmt.Errorf("negative event channel count %d in %s", channelCount, path)
	}

	order := make([]uint32, 0, channelCount)
	listCounts := make([]int16, 0, channelCount)
	lists := make(map[uint32][]*EventList, channelCount)

	for i := int16(0); i < channelCount; i++ {
		channelID, err := reader.Uint32()
		if err != nil {
			return nil, err
		}
		eventListCount, err := reader.Int16()
		if err != nil {
			return nil, err
		}
		order = append(order, channelID)
		listCounts = append(listCounts, eventListCount)

		channelLists := make([]*EventList, 0, eventListCount)
		for j := int16(0); j < eventListCount; j++ {
			first, err := reader.Int64()
			if err != nil {
				return nil, err
			}
			last, err := reader.Int64()
			if err != nil {
				return nil, err
			}
			count, err := reader.Int32()
			if err != nil {
				return nil, err
			}
			listType, err := reader.Int8()
			if err != nil {
				return nil, err
			}
			rate, err := reader.QtFloat()
			if err != nil {
				return nil, err
			}
			gain, err := reader.QtFloat()
			if err != nil {
				return nil, err
			}
			offset, err := reader.QtFloat()
			if err != nil {
				return nil, err
			}
			minimum, err := reader.QtFloat()
			if err != nil {
				return nil, err
			}
			maximum, err := reader.QtFloat()
			if err != nil {
				return nil, err
			}
			dimension, err := reader.QString()
			if err != nil {
				return nil, err
			}
			hasSecondField, err := reader.Bool()
			if err != nil {
				return nil, err
			}

			eventList := &EventList{
				ChannelID:      channelID,
				First:          first,
				Last:           last,
				Count:          count,
				Type:           listType,
				Rate:           rate,
				Gain:           gain,
				Offset:         offset,
				Min:            minimum,
				Max:            maximum,
				Dimension:      dimension,
				HasSecondField: hasSecondField,
			}
			if hasSecondField {
				min2, err := reader.QtFloat()
				if err != nil {
					return nil, err
				}
				max2, err := reader.QtFloat()
				if err != nil {
					return nil, err
				}
				eventList.Min2 = min2
				eventList.Max2 = max2
			}
			channelLists = append(channelLists, eventList)
		}
		lists[channelID] = channelLists
	}

	for idx, channelID := range order {
		channelLists := lists[channelID]
		for listIndex := int16(0); listIndex < listCounts[idx]; listIndex++ {
			eventList := channelLists[listIndex]
			if eventList.Count < 0 {
				return nil, fmt.Errorf("negative event count %d in %s", eventList.Count, path)
			}
			eventList.Data = make([]int16, eventList.Count)
			if err := binary.Read(reader.r, binary.LittleEndian, &eventList.Data); err != nil {
				return nil, err
			}
			if eventList.HasSecondField {
				eventList.Data2 = make([]int16, eventList.Count)
				if err := binary.Read(reader.r, binary.LittleEndian, &eventList.Data2); err != nil {
					return nil, err
				}
			}
			if eventList.Type != 0 {
				eventList.Times = make([]uint32, eventList.Count)
				if err := binary.Read(reader.r, binary.LittleEndian, &eventList.Times); err != nil {
					return nil, err
				}
			}
		}
	}

	return &SessionEvents{Lists: lists}, nil
}
