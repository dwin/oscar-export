package cache

import (
	"fmt"
	"os"
)

func LoadSummaryFile(path string) (SummaryData, error) {
	// #nosec G304 -- path is derived from the selected OSCAR dataset layout.
	data, err := os.ReadFile(path)
	if err != nil {
		return SummaryData{}, err
	}
	reader := NewReader(data)
	wrapErr := func(section string, err error) error {
		if err == nil {
			return nil
		}
		return fmt.Errorf("%s: %w", section, err)
	}

	magic, err := reader.Uint32()
	if err != nil {
		return SummaryData{}, wrapErr("read magic", err)
	}
	version, err := reader.Uint16()
	if err != nil {
		return SummaryData{}, wrapErr("read version", err)
	}
	fileType, err := reader.Uint16()
	if err != nil {
		return SummaryData{}, wrapErr("read file type", err)
	}
	machineID, err := reader.Uint32()
	if err != nil {
		return SummaryData{}, wrapErr("read machine id", err)
	}
	sessionID, err := reader.Uint32()
	if err != nil {
		return SummaryData{}, wrapErr("read session id", err)
	}
	first, err := reader.Int64()
	if err != nil {
		return SummaryData{}, wrapErr("read first timestamp", err)
	}
	last, err := reader.Int64()
	if err != nil {
		return SummaryData{}, wrapErr("read last timestamp", err)
	}

	if magic != magicNumber {
		return SummaryData{}, fmt.Errorf("unexpected summary magic 0x%x in %s", magic, path)
	}
	if version != summaryVersion {
		return SummaryData{}, fmt.Errorf("unsupported summary version %d in %s", version, path)
	}
	if fileType != fileTypeSummary {
		return SummaryData{}, fmt.Errorf("unexpected summary file type %d in %s", fileType, path)
	}

	settings, err := readHash(reader, (*Reader).Uint32, (*Reader).QVariant)
	if err != nil {
		return SummaryData{}, wrapErr("read settings", err)
	}
	counts, err := readHash(reader, (*Reader).Uint32, (*Reader).QtFloat)
	if err != nil {
		return SummaryData{}, wrapErr("read counts", err)
	}
	sums, err := readHash(reader, (*Reader).Uint32, (*Reader).Float64)
	if err != nil {
		return SummaryData{}, wrapErr("read sums", err)
	}
	avgs, err := readHash(reader, (*Reader).Uint32, (*Reader).QtFloat)
	if err != nil {
		return SummaryData{}, wrapErr("read averages", err)
	}
	wavgs, err := readHash(reader, (*Reader).Uint32, (*Reader).QtFloat)
	if err != nil {
		return SummaryData{}, wrapErr("read weighted averages", err)
	}
	mins, err := readHash(reader, (*Reader).Uint32, (*Reader).QtFloat)
	if err != nil {
		return SummaryData{}, wrapErr("read minimums", err)
	}
	maxs, err := readHash(reader, (*Reader).Uint32, (*Reader).QtFloat)
	if err != nil {
		return SummaryData{}, wrapErr("read maximums", err)
	}
	physicalMins, err := readHash(reader, (*Reader).Uint32, (*Reader).QtFloat)
	if err != nil {
		return SummaryData{}, wrapErr("read physical minimums", err)
	}
	physicalMaxs, err := readHash(reader, (*Reader).Uint32, (*Reader).QtFloat)
	if err != nil {
		return SummaryData{}, wrapErr("read physical maximums", err)
	}
	cph, err := readHash(reader, (*Reader).Uint32, (*Reader).QtFloat)
	if err != nil {
		return SummaryData{}, wrapErr("read counts-per-hour", err)
	}
	sph, err := readHash(reader, (*Reader).Uint32, (*Reader).QtFloat)
	if err != nil {
		return SummaryData{}, wrapErr("read session-percent-per-hour", err)
	}
	firstChannel, err := readHash(reader, (*Reader).Uint32, (*Reader).Uint64)
	if err != nil {
		return SummaryData{}, wrapErr("read first-channel timestamps", err)
	}
	lastChannel, err := readHash(reader, (*Reader).Uint32, (*Reader).Uint64)
	if err != nil {
		return SummaryData{}, wrapErr("read last-channel timestamps", err)
	}
	valueSummary, err := readHash(reader, (*Reader).Uint32, func(r *Reader) (map[int16]int16, error) {
		return readHash(r, (*Reader).Int16, (*Reader).Int16)
	})
	if err != nil {
		return SummaryData{}, wrapErr("read value summary histogram", err)
	}
	timeSummary, err := readHash(reader, (*Reader).Uint32, func(r *Reader) (map[int16]uint32, error) {
		return readHash(r, (*Reader).Int16, (*Reader).Uint32)
	})
	if err != nil {
		return SummaryData{}, wrapErr("read time summary histogram", err)
	}
	gain, err := readHash(reader, (*Reader).Uint32, (*Reader).QtFloat)
	if err != nil {
		return SummaryData{}, wrapErr("read gains", err)
	}
	availableChannels, err := readList(reader, (*Reader).Uint32)
	if err != nil {
		return SummaryData{}, wrapErr("read available channels", err)
	}
	timeAboveThreshold, err := readHash(reader, (*Reader).Uint32, (*Reader).QtFloat)
	if err != nil {
		return SummaryData{}, wrapErr("read time-above-threshold", err)
	}
	upperThreshold, err := readHash(reader, (*Reader).Uint32, (*Reader).QtFloat)
	if err != nil {
		return SummaryData{}, wrapErr("read upper thresholds", err)
	}
	timeBelowThreshold, err := readHash(reader, (*Reader).Uint32, (*Reader).QtFloat)
	if err != nil {
		return SummaryData{}, wrapErr("read time-below-threshold", err)
	}
	lowerThreshold, err := readHash(reader, (*Reader).Uint32, (*Reader).QtFloat)
	if err != nil {
		return SummaryData{}, wrapErr("read lower thresholds", err)
	}
	summaryOnly, err := reader.Bool()
	if err != nil {
		return SummaryData{}, wrapErr("read summary-only flag", err)
	}
	noSettings, err := reader.Bool()
	if err != nil {
		return SummaryData{}, wrapErr("read no-settings flag", err)
	}
	slicesData, err := readList(reader, func(r *Reader) (SessionSlice, error) {
		start, err := r.Int64()
		if err != nil {
			return SessionSlice{}, err
		}
		length, err := r.Uint32()
		if err != nil {
			return SessionSlice{}, err
		}
		status, err := r.Uint16()
		if err != nil {
			return SessionSlice{}, err
		}
		return SessionSlice{
			Start:  start,
			End:    start + int64(length),
			Status: status,
		}, nil
	})
	if err != nil {
		return SummaryData{}, wrapErr("read session slices", err)
	}

	return SummaryData{
		MachineID:          machineID,
		SessionID:          sessionID,
		First:              first,
		Last:               last,
		Settings:           settings,
		Counts:             counts,
		Sums:               sums,
		Avgs:               avgs,
		WAvgs:              wavgs,
		Mins:               mins,
		Maxs:               maxs,
		PhysicalMins:       physicalMins,
		PhysicalMaxs:       physicalMaxs,
		CPH:                cph,
		SPH:                sph,
		FirstChannel:       firstChannel,
		LastChannel:        lastChannel,
		ValueSummary:       valueSummary,
		TimeSummary:        timeSummary,
		Gain:               gain,
		TimeAboveThreshold: timeAboveThreshold,
		UpperThreshold:     upperThreshold,
		TimeBelowThreshold: timeBelowThreshold,
		LowerThreshold:     lowerThreshold,
		AvailableChannels:  availableChannels,
		SummaryOnly:        summaryOnly,
		NoSettings:         noSettings,
		Slices:             slicesData,
	}, nil
}
