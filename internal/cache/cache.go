package cache

import (
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	magicNumber         = 0xC73216AB
	fileTypeSummary     = 0
	fileTypeData        = 1
	fileTypeSessionInfo = 5
	summaryVersion      = 18
	sessionIndexVersion = 1
	sessionInfoVersion  = 2
	maskOnStatus        = 2
	machineTypeCPAP     = 1
)

type ProfilePrefs struct {
	UserName            string
	LastExportCSVPath   string
	TimeZone            string
	DaySplitTime        time.Duration
	PrefCalcMiddle      int
	PrefCalcPercentile  float64
	PrefCalcMax         bool
	CombineCloseSession int
	IgnoreShortSession  int
	LockSummarySessions bool
}

type Channel struct {
	ID    uint32
	Code  string
	Label string
	Name  string
}

type Machine struct {
	ID          uint32
	Type        int
	LoaderName  string
	Brand       string
	Model       string
	ModelNumber string
	Serial      string
	Series      string
	DataPath    string
}

type SessionSlice struct {
	Start  int64
	End    int64
	Status uint16
}

type SummaryData struct {
	MachineID          uint32
	SessionID          uint32
	First              int64
	Last               int64
	Settings           map[uint32]QVariant
	Counts             map[uint32]float32
	Sums               map[uint32]float64
	Avgs               map[uint32]float32
	WAvgs              map[uint32]float32
	Mins               map[uint32]float32
	Maxs               map[uint32]float32
	PhysicalMins       map[uint32]float32
	PhysicalMaxs       map[uint32]float32
	CPH                map[uint32]float32
	SPH                map[uint32]float32
	FirstChannel       map[uint32]uint64
	LastChannel        map[uint32]uint64
	ValueSummary       map[uint32]map[int16]int16
	TimeSummary        map[uint32]map[int16]uint32
	Gain               map[uint32]float32
	TimeAboveThreshold map[uint32]float32
	UpperThreshold     map[uint32]float32
	TimeBelowThreshold map[uint32]float32
	LowerThreshold     map[uint32]float32
	AvailableChannels  []uint32
	SummaryOnly        bool
	NoSettings         bool
	Slices             []SessionSlice
}

type Session struct {
	Machine           *Machine
	ID                uint32
	First             int64
	Last              int64
	Enabled           bool
	HasEvents         bool
	SummaryOnly       bool
	AvailableChannels []uint32
	AvailableSettings []uint32
	Summary           SummaryData
}

type EventList struct {
	ChannelID      uint32
	First          int64
	Last           int64
	Count          int32
	Type           int8
	Rate           float32
	Gain           float32
	Offset         float32
	Min            float32
	Max            float32
	Dimension      string
	HasSecondField bool
	Min2           float32
	Max2           float32
	Data           []int16
	Data2          []int16
	Times          []uint32
}

type SessionEvents struct {
	Lists map[uint32][]*EventList
}

type Dataset struct {
	Root           string
	ProfilePath    string
	Profile        ProfilePrefs
	Location       *time.Location
	ChannelsByID   map[uint32]Channel
	ChannelsByCode map[string]Channel
	Machines       []*Machine
	Sessions       []*Session
}

type sessionIndexEntry struct {
	ID                uint32
	First             int64
	Last              int64
	Enabled           bool
	HasEvents         bool
	AvailableChannels []uint32
	AvailableSettings []uint32
}

func LoadDataset(root, profileUser, serial string) (*Dataset, error) {
	profilePath := filepath.Join(root, "Profiles", profileUser)
	profile, err := loadProfilePrefs(filepath.Join(profilePath, "Profile.xml"))
	if err != nil {
		return nil, err
	}
	if profile.UserName == "" {
		profile.UserName = profileUser
	}
	location := time.Local
	if profile.TimeZone != "" {
		if loc, err := time.LoadLocation(profile.TimeZone); err == nil {
			location = loc
		}
	}

	channelsByID, channelsByCode, err := loadChannels(filepath.Join(profilePath, "channels.xml"))
	if err != nil {
		return nil, err
	}
	machines, err := loadMachines(filepath.Join(profilePath, "machines.xml"), profilePath)
	if err != nil {
		return nil, err
	}

	filtered := make([]*Machine, 0, len(machines))
	for _, machine := range machines {
		if machine.Type != machineTypeCPAP {
			continue
		}
		if serial != "" && machine.Serial != serial {
			continue
		}
		filtered = append(filtered, machine)
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("no CPAP machines found for profile %q", profileUser)
	}

	sessions := make([]*Session, 0)
	for _, machine := range filtered {
		entries, err := loadSessionIndex(filepath.Join(machine.DataPath, "Summaries.xml.gz"))
		if err != nil {
			return nil, fmt.Errorf("load session index for %s: %w", machine.Serial, err)
		}
		enabledMap, err := loadSessionInfo(filepath.Join(machine.DataPath, "Sessions.info"))
		if err != nil {
			return nil, fmt.Errorf("load session info for %s: %w", machine.Serial, err)
		}

		slices.SortFunc(entries, func(a, b sessionIndexEntry) int {
			if a.First == b.First {
				if a.ID == b.ID {
					return 0
				}
				if a.ID < b.ID {
					return -1
				}
				return 1
			}
			if a.First < b.First {
				return -1
			}
			return 1
		})

		for _, entry := range entries {
			summaryPath := filepath.Join(machine.DataPath, "Summaries", toHexID(entry.ID)+".000")
			summary, err := LoadSummaryFile(summaryPath)
			if err != nil {
				return nil, fmt.Errorf("load summary %s: %w", summaryPath, err)
			}
			if summary.MachineID != machine.ID {
				return nil, fmt.Errorf("summary %s machine id %d did not match machines.xml id %d", summaryPath, summary.MachineID, machine.ID)
			}

			enabled := entry.Enabled
			if value, ok := enabledMap[entry.ID]; ok {
				enabled = value
			}

			session := &Session{
				Machine:           machine,
				ID:                entry.ID,
				First:             summary.First,
				Last:              summary.Last,
				Enabled:           enabled,
				HasEvents:         entry.HasEvents,
				SummaryOnly:       !entry.HasEvents || summary.SummaryOnly,
				AvailableChannels: entry.AvailableChannels,
				AvailableSettings: entry.AvailableSettings,
				Summary:           summary,
			}
			sessions = append(sessions, session)
		}
	}

	slices.SortFunc(sessions, func(a, b *Session) int {
		if a.First == b.First {
			if a.ID == b.ID {
				return strings.Compare(a.Machine.Serial, b.Machine.Serial)
			}
			if a.ID < b.ID {
				return -1
			}
			return 1
		}
		if a.First < b.First {
			return -1
		}
		return 1
	})

	return &Dataset{
		Root:           root,
		ProfilePath:    profilePath,
		Profile:        profile,
		Location:       location,
		ChannelsByID:   channelsByID,
		ChannelsByCode: channelsByCode,
		Machines:       filtered,
		Sessions:       sessions,
	}, nil
}

func (s *Session) LengthMillis() int64 {
	if s.Last <= s.First {
		return 0
	}
	return s.Last - s.First
}

func (s *Session) Hours() float64 {
	if len(s.Summary.Slices) == 0 {
		return float64(s.LengthMillis()) / 3600000.0
	}
	var total int64
	for _, slice := range s.Summary.Slices {
		if slice.Status != maskOnStatus || slice.End <= slice.Start {
			continue
		}
		total += slice.End - slice.Start
	}
	return float64(total) / 3600000.0
}

func (s *Session) Count(channelID uint32) float64 {
	return float64(s.Summary.Counts[channelID])
}

func (s *Session) Sum(channelID uint32) float64 {
	return s.Summary.Sums[channelID]
}

func (s *Session) Avg(channelID uint32) float64 {
	return float64(s.Summary.Avgs[channelID])
}

func (s *Session) WAvg(channelID uint32) float64 {
	return float64(s.Summary.WAvgs[channelID])
}

func (s *Session) Max(channelID uint32) float64 {
	return float64(s.Summary.Maxs[channelID])
}

func (s *Session) Gain(channelID uint32) float64 {
	return float64(s.Summary.Gain[channelID])
}

func (s *Session) Percentile(channelID uint32, percent float64) float64 {
	hist := s.Summary.ValueSummary[channelID]
	if len(hist) == 0 {
		return 0
	}
	keys := make([]int16, 0, len(hist))
	total := 0
	for key, count := range hist {
		keys = append(keys, key)
		total += int(count)
	}
	if total <= 0 {
		return 0
	}
	slices.Sort(keys)
	n := int(math.Floor(float64(total) * percent))
	if n > total-1 {
		n--
	}
	if n < 0 {
		n = 0
	}
	accumulated := 0
	for _, key := range keys {
		accumulated += int(hist[key])
		if accumulated > n {
			return float64(key) * s.Gain(channelID)
		}
	}
	last := keys[len(keys)-1]
	return float64(last) * s.Gain(channelID)
}

func (s *Session) CalcMiddle(prefCalcMiddle int, channelID uint32) float64 {
	switch prefCalcMiddle {
	case 0:
		return s.Percentile(channelID, 0.5)
	case 1:
		return s.WAvg(channelID)
	default:
		return s.Avg(channelID)
	}
}

func (s *Session) CalcMax(prefCalcMax bool, channelID uint32) float64 {
	if prefCalcMax {
		return s.Percentile(channelID, 0.995)
	}
	return s.Max(channelID)
}

func (e *EventList) TimeAt(idx int) int64 {
	if e.Type == 0 {
		return e.First + int64(float32(idx)*e.Rate)
	}
	return e.First + int64(e.Times[idx])
}

func (e *EventList) DataAt(idx int) float64 {
	return float64(e.Data[idx]) * float64(e.Gain)
}

func loadProfilePrefs(path string) (ProfilePrefs, error) {
	// #nosec G304 -- path is derived from the selected OSCAR dataset layout.
	data, err := os.ReadFile(path)
	if err != nil {
		return ProfilePrefs{}, fmt.Errorf("read %s: %w", path, err)
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	decoder := xml.NewDecoder(bytes.NewReader(data))

	values := map[string]string{}
	var inProfile bool
	var current string
	var text strings.Builder

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ProfilePrefs{}, fmt.Errorf("decode %s: %w", path, err)
		}
		switch typed := token.(type) {
		case xml.StartElement:
			if typed.Name.Local == "Profile" {
				inProfile = true
				current = ""
				text.Reset()
				continue
			}
			if inProfile && current == "" {
				current = typed.Name.Local
				text.Reset()
			}
		case xml.CharData:
			if current != "" {
				text.Write([]byte(typed))
			}
		case xml.EndElement:
			if typed.Name.Local == "Profile" {
				inProfile = false
				current = ""
				text.Reset()
				continue
			}
			if current != "" && typed.Name.Local == current {
				values[current] = strings.TrimSpace(text.String())
				current = ""
				text.Reset()
			}
		}
	}

	return ProfilePrefs{
		UserName:            values["UserName"],
		LastExportCSVPath:   values["LastExportCsvPath"],
		TimeZone:            values["TimeZone"],
		DaySplitTime:        parseClock(values["DaySplitTime"], 12*time.Hour),
		PrefCalcMiddle:      parseInt(values["PrefCalcMiddle"], 0),
		PrefCalcPercentile:  parseFloat(values["PrefCalcPercentile"], 95),
		PrefCalcMax:         parseInt(values["PrefCalcMax"], 0) != 0,
		CombineCloseSession: parseInt(values["CombineCloserSessions"], 0),
		IgnoreShortSession:  parseInt(values["IgnoreShorterSessions"], 0),
		LockSummarySessions: parseBool(values["LockSummarySessions"], true),
	}, nil
}

type channelsEnvelope struct {
	Channels struct {
		Groups []struct {
			Channels []struct {
				ID    string `xml:"id,attr"`
				Code  string `xml:"code,attr"`
				Label string `xml:"label,attr"`
				Name  string `xml:"name,attr"`
			} `xml:"channel"`
		} `xml:"group"`
	} `xml:"channels"`
}

func loadChannels(path string) (map[uint32]Channel, map[string]Channel, error) {
	// #nosec G304 -- path is derived from the selected OSCAR dataset layout.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	var envelope channelsEnvelope
	if err := xml.Unmarshal(data, &envelope); err != nil {
		return nil, nil, fmt.Errorf("decode %s: %w", path, err)
	}

	byID := map[uint32]Channel{}
	byCode := map[string]Channel{}
	for _, group := range envelope.Channels.Groups {
		for _, raw := range group.Channels {
			id64, err := strconv.ParseUint(raw.ID, 10, 32)
			if err != nil {
				return nil, nil, fmt.Errorf("parse channel id %q: %w", raw.ID, err)
			}
			channel := Channel{
				ID:    uint32(id64),
				Code:  raw.Code,
				Label: raw.Label,
				Name:  raw.Name,
			}
			byID[channel.ID] = channel
			byCode[channel.Code] = channel
		}
	}
	return byID, byCode, nil
}

type machinesEnvelope struct {
	Machines []struct {
		ID          string `xml:"id,attr"`
		Type        string `xml:"type,attr"`
		Class       string `xml:"class,attr"`
		Brand       string `xml:"brand"`
		Model       string `xml:"model"`
		ModelNumber string `xml:"modelnumber"`
		Serial      string `xml:"serial"`
		Series      string `xml:"series"`
	} `xml:"machine"`
}

func loadMachines(path, profilePath string) ([]*Machine, error) {
	// #nosec G304 -- path is derived from the selected OSCAR dataset layout.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var envelope machinesEnvelope
	if err := xml.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	machines := make([]*Machine, 0, len(envelope.Machines))
	for _, raw := range envelope.Machines {
		id64, err := strconv.ParseUint(raw.ID, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("parse machine id %q: %w", raw.ID, err)
		}
		typ, err := strconv.Atoi(raw.Type)
		if err != nil {
			return nil, fmt.Errorf("parse machine type %q: %w", raw.Type, err)
		}
		serial := raw.Serial
		if serial == "" {
			serial = toHexID(uint32(id64))
		}
		machines = append(machines, &Machine{
			ID:          uint32(id64),
			Type:        typ,
			LoaderName:  raw.Class,
			Brand:       raw.Brand,
			Model:       raw.Model,
			ModelNumber: raw.ModelNumber,
			Serial:      serial,
			Series:      raw.Series,
			DataPath:    filepath.Join(profilePath, fmt.Sprintf("%s_%s", raw.Class, serial)),
		})
	}
	return machines, nil
}

type sessionIndexEnvelope struct {
	Version  string `xml:"version,attr"`
	Sessions []struct {
		ID       string `xml:"id,attr"`
		First    string `xml:"first,attr"`
		Last     string `xml:"last,attr"`
		Enabled  string `xml:"enabled,attr"`
		Events   string `xml:"events,attr"`
		Channels string `xml:"channels"`
		Settings string `xml:"settings"`
	} `xml:"session"`
}

func loadSessionIndex(path string) ([]sessionIndexEntry, error) {
	// #nosec G304 -- path is derived from the selected OSCAR dataset layout.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("open gzip %s: %w", path, err)
	}
	xmlData, err := io.ReadAll(gz)
	if err != nil {
		if closeErr := gz.Close(); closeErr != nil {
			return nil, fmt.Errorf("close gzip %s after read error: %w", path, closeErr)
		}
		return nil, fmt.Errorf("read gzip %s: %w", path, err)
	}
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("close gzip %s: %w", path, err)
	}
	var envelope sessionIndexEnvelope
	if err := xml.Unmarshal(xmlData, &envelope); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	if parseInt(envelope.Version, 0) != sessionIndexVersion {
		return nil, fmt.Errorf("unsupported session index version %q in %s", envelope.Version, path)
	}

	entries := make([]sessionIndexEntry, 0, len(envelope.Sessions))
	for _, raw := range envelope.Sessions {
		id64, err := strconv.ParseUint(raw.ID, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("parse session id %q: %w", raw.ID, err)
		}
		first, err := strconv.ParseInt(raw.First, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse session first %q: %w", raw.First, err)
		}
		last, err := strconv.ParseInt(raw.Last, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse session last %q: %w", raw.Last, err)
		}
		entries = append(entries, sessionIndexEntry{
			ID:                uint32(id64),
			First:             first,
			Last:              last,
			Enabled:           parseInt(raw.Enabled, 1) == 1,
			HasEvents:         parseInt(raw.Events, 1) == 1,
			AvailableChannels: parseHexList(raw.Channels),
			AvailableSettings: parseHexList(raw.Settings),
		})
	}
	return entries, nil
}

func loadSessionInfo(path string) (map[uint32]bool, error) {
	// #nosec G304 -- path is derived from the selected OSCAR dataset layout.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	reader := NewReader(data)
	magic, err := reader.Uint32()
	if err != nil {
		return nil, err
	}
	fileType, err := reader.Uint16()
	if err != nil {
		return nil, err
	}
	version, err := reader.Uint16()
	if err != nil {
		return nil, err
	}
	if magic != magicNumber {
		return nil, fmt.Errorf("unexpected session info magic 0x%x in %s", magic, path)
	}
	if fileType != fileTypeSessionInfo {
		return nil, fmt.Errorf("unexpected session info file type %d in %s", fileType, path)
	}
	if version != sessionInfoVersion {
		return nil, fmt.Errorf("unsupported session info version %d in %s", version, path)
	}

	count, err := reader.Int32()
	if err != nil {
		return nil, err
	}
	if count < 0 {
		return nil, fmt.Errorf("negative session info size %d in %s", count, path)
	}
	out := make(map[uint32]bool, count)
	for i := int32(0); i < count; i++ {
		sid, err := reader.Uint32()
		if err != nil {
			return nil, err
		}
		enabled, err := reader.Uint8()
		if err != nil {
			return nil, err
		}
		out[sid] = enabled&0x1 == 1
	}
	return out, nil
}

func toHexID(id uint32) string {
	return fmt.Sprintf("%08x", id)
}

func parseHexList(raw string) []uint32 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]uint32, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		value, err := strconv.ParseUint(part, 16, 32)
		if err != nil {
			continue
		}
		values = append(values, uint32(value))
	}
	return values
}

func parseBool(raw string, def bool) bool {
	raw = strings.TrimSpace(strings.ToLower(raw))
	switch raw {
	case "true", "1":
		return true
	case "false", "0":
		return false
	default:
		return def
	}
}

func parseInt(raw string, def int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return def
	}
	return int(value)
}

func parseFloat(raw string, def float64) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return def
	}
	return value
}

func parseClock(raw string, def time.Duration) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def
	}
	parts := strings.Split(raw, ":")
	if len(parts) != 3 {
		return def
	}
	hour, err1 := strconv.Atoi(parts[0])
	minute, err2 := strconv.Atoi(parts[1])
	second, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return def
	}
	return time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute + time.Duration(second)*time.Second
}
