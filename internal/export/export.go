package exporter

import (
	"context"
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/dwin/oscar-export/internal/cache"
)

type Mode string

const (
	ModeSummary  Mode = "summary"
	ModeSessions Mode = "sessions"
	ModeDetails  Mode = "details"
	maskOnStatus      = 2
)

var countChannelCodes = []string{
	"ClearAirway",
	"AllApnea",
	"Obstructive",
	"Hypopnea",
	"Apnea",
	"VSnore",
	"VSnore2",
	"RERA",
	"FlowLimit",
	"SensAwake",
	"NRI",
	"ExP",
	"LeakFlag",
	"UserFlag1",
	"UserFlag2",
	"PressurePulse",
}

var statChannelCodes = []string{
	"Pressure",
	"PressureSet",
	"IPAP",
	"IPAPSet",
	"EPAP",
	"EPAPSet",
	"FLG",
}

type Config struct {
	Mode        Mode
	Root        string
	ProfileUser string
	Serial      string
	Out         string
	From        time.Time
	To          time.Time
}

type dayGroup struct {
	Date     time.Time
	Sessions []*cache.Session
}

type columnSpec struct {
	countChannels []cache.Channel
	statChannels  []cache.Channel
	ahiChannels   []cache.Channel
}

func Run(ctx context.Context, cfg Config) (err error) {
	_ = ctx

	ds, err := cache.LoadDataset(cfg.Root, cfg.ProfileUser, cfg.Serial)
	if err != nil {
		return err
	}
	columns, err := resolveColumns(ds)
	if err != nil {
		return err
	}

	outPath := cfg.Out
	if outPath == "" {
		outPath = defaultOutputPath(ds, cfg)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o750); err != nil {
		return fmt.Errorf("create output directory for %s: %w", outPath, err)
	}

	// #nosec G304 -- CLI writes to an explicit user-provided or derived output path.
	file, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", outPath, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", outPath, closeErr)
		}
	}()

	writer := csv.NewWriter(file)
	groups, groupIndex := buildDayGroups(ds)
	_ = groups

	switch cfg.Mode {
	case ModeSummary:
		err = writeSummary(writer, ds, columns, cfg, groupIndex)
	case ModeSessions:
		err = writeSessions(writer, ds, columns, cfg, groupIndex)
	case ModeDetails:
		err = writeDetails(writer, ds, columns, cfg, groupIndex)
	default:
		err = fmt.Errorf("unsupported mode %q", cfg.Mode)
	}
	if err != nil {
		return err
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("flush csv %s: %w", outPath, err)
	}
	return nil
}

func resolveColumns(ds *cache.Dataset) (columnSpec, error) {
	resolve := func(code string) (cache.Channel, error) {
		channel, ok := ds.ChannelsByCode[code]
		if !ok {
			return cache.Channel{}, fmt.Errorf("missing channel code %q in channels.xml", code)
		}
		return channel, nil
	}

	spec := columnSpec{}
	for _, code := range countChannelCodes {
		channel, err := resolve(code)
		if err != nil {
			return columnSpec{}, err
		}
		spec.countChannels = append(spec.countChannels, channel)
	}
	for _, code := range statChannelCodes {
		channel, err := resolve(code)
		if err != nil {
			return columnSpec{}, err
		}
		spec.statChannels = append(spec.statChannels, channel)
	}
	for _, code := range []string{"ClearAirway", "AllApnea", "Obstructive", "Hypopnea", "Apnea"} {
		channel, err := resolve(code)
		if err != nil {
			return columnSpec{}, err
		}
		spec.ahiChannels = append(spec.ahiChannels, channel)
	}
	return spec, nil
}

func buildDayGroups(ds *cache.Dataset) ([]*dayGroup, map[string]*dayGroup) {
	groups := make([]*dayGroup, 0)
	index := map[string]*dayGroup{}

	for _, session := range ds.Sessions {
		if shouldIgnoreSession(ds.Profile, session) {
			continue
		}
		date := sessionSleepDate(ds, session, groups)
		key := date.Format("2006-01-02")
		group, ok := index[key]
		if !ok {
			group = &dayGroup{Date: date}
			index[key] = group
			groups = append(groups, group)
		}
		group.Sessions = append(group.Sessions, session)
	}

	return groups, index
}

func shouldIgnoreSession(profile cache.ProfilePrefs, session *cache.Session) bool {
	if profile.IgnoreShortSession <= 0 {
		return false
	}
	return session.LengthMillis()/60000 < int64(profile.IgnoreShortSession)
}

func sessionSleepDate(ds *cache.Dataset, session *cache.Session, groups []*dayGroup) time.Time {
	start := time.UnixMilli(session.First).In(ds.Location)
	date := startOfDay(start)
	split := ds.Profile.DaySplitTime
	combine := ds.Profile.CombineCloseSession

	if ds.Profile.LockSummarySessions && session.SummaryOnly {
		split = 12 * time.Hour
		combine = 0
	}

	if clockOfDay(start) < split {
		return date.AddDate(0, 0, -1)
	}
	if combine > 0 && len(groups) > 0 {
		prev := groups[len(groups)-1]
		if prev.Date.Equal(date.AddDate(0, 0, -1)) {
			prevLast := prev.latestEndMillis()
			if prevLast > 0 {
				gap := time.UnixMilli(session.First).Sub(time.UnixMilli(prevLast))
				if gap < time.Duration(combine)*time.Minute {
					return prev.Date
				}
			}
		}
	}
	return date
}

func writeSummary(writer *csv.Writer, ds *cache.Dataset, columns columnSpec, cfg Config, groups map[string]*dayGroup) error {
	header := []string{"Date", "Session Count", "Start", "End", "Total Time", "AHI"}
	header = append(header, countHeaders(columns.countChannels)...)
	header = append(header, middleHeaders(ds.Profile, columns.statChannels)...)
	header = append(header, percentileHeaders(ds.Profile, columns.statChannels)...)
	header = append(header, maxHeaders(ds.Profile, columns.statChannels)...)
	if err := writer.Write(header); err != nil {
		return err
	}

	for day := cfg.From; !day.After(cfg.To); day = day.AddDate(0, 0, 1) {
		group := groups[day.Format("2006-01-02")]
		if group == nil {
			continue
		}

		row := []string{
			group.Date.Format("2006-01-02"),
			strconv.Itoa(len(group.Sessions)),
			formatDateTime(group.firstEnabledMillis(), ds.Location),
			formatDateTime(group.lastEnabledMillis(), ds.Location),
			formatDuration(group.totalTimeMillis()),
			formatAHI(group.ahi(columns.ahiChannels)),
		}

		for _, channel := range columns.countChannels {
			row = append(row, formatGeneral(group.count(channel.ID)))
		}
		for _, channel := range columns.statChannels {
			row = append(row, formatGeneral(group.calcMiddle(ds.Profile.PrefCalcMiddle, channel.ID)))
		}
		for _, channel := range columns.statChannels {
			row = append(row, formatGeneral(group.percentile(channel.ID, ds.Profile.PrefCalcPercentile/100.0)))
		}
		for _, channel := range columns.statChannels {
			row = append(row, formatGeneral(group.calcMax(ds.Profile.PrefCalcMax, channel.ID)))
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}

func writeSessions(writer *csv.Writer, ds *cache.Dataset, columns columnSpec, cfg Config, groups map[string]*dayGroup) error {
	header := []string{"Date", "Session", "Start", "End", "Total Time", "AHI"}
	header = append(header, countHeaders(columns.countChannels)...)
	header = append(header, middleHeaders(ds.Profile, columns.statChannels)...)
	header = append(header, percentileHeaders(ds.Profile, columns.statChannels)...)
	header = append(header, maxHeaders(ds.Profile, columns.statChannels)...)
	if err := writer.Write(header); err != nil {
		return err
	}

	for day := cfg.From; !day.After(cfg.To); day = day.AddDate(0, 0, 1) {
		group := groups[day.Format("2006-01-02")]
		if group == nil {
			continue
		}
		for _, session := range group.Sessions {
			row := []string{
				group.Date.Format("2006-01-02"),
				strconv.FormatUint(uint64(session.ID), 10),
				formatDateTime(session.First, ds.Location),
				formatDateTime(session.Last, ds.Location),
				formatDuration(session.LengthMillis()),
				formatAHI(sessionAHI(session, columns.ahiChannels)),
			}
			for _, channel := range columns.countChannels {
				row = append(row, formatGeneral(session.Count(channel.ID)))
			}
			for _, channel := range columns.statChannels {
				row = append(row, formatGeneral(session.CalcMiddle(ds.Profile.PrefCalcMiddle, channel.ID)))
			}
			for _, channel := range columns.statChannels {
				row = append(row, formatGeneral(session.Percentile(channel.ID, ds.Profile.PrefCalcPercentile/100.0)))
			}
			for _, channel := range columns.statChannels {
				row = append(row, formatGeneral(session.CalcMax(ds.Profile.PrefCalcMax, channel.ID)))
			}
			if err := writer.Write(row); err != nil {
				return err
			}
		}
	}

	return nil
}

func writeDetails(writer *csv.Writer, ds *cache.Dataset, columns columnSpec, cfg Config, groups map[string]*dayGroup) error {
	if err := writer.Write([]string{"DateTime", "Session", "Event", "Data/Duration"}); err != nil {
		return err
	}
	detailChannels := make([]cache.Channel, 0, len(columns.countChannels)+len(columns.statChannels))
	detailChannels = append(detailChannels, columns.countChannels...)
	detailChannels = append(detailChannels, columns.statChannels...)

	for day := cfg.From; !day.After(cfg.To); day = day.AddDate(0, 0, 1) {
		group := groups[day.Format("2006-01-02")]
		if group == nil {
			continue
		}
		for _, session := range group.Sessions {
			if !session.HasEvents {
				continue
			}
			eventsPath := filepath.Join(session.Machine.DataPath, "Events", fmt.Sprintf("%08x.001", session.ID))
			events, err := cache.LoadEventsFile(eventsPath)
			if err != nil {
				return fmt.Errorf("load events %s: %w", eventsPath, err)
			}
			for _, channel := range detailChannels {
				lists := events.Lists[channel.ID]
				for _, eventList := range lists {
					for idx := 0; idx < int(eventList.Count); idx++ {
						row := []string{
							formatDateTime(eventList.TimeAt(idx), ds.Location),
							strconv.FormatUint(uint64(session.ID), 10),
							channel.Code,
							formatDetails(eventList.DataAt(idx)),
						}
						if err := writer.Write(row); err != nil {
							return err
						}
					}
				}
			}
		}
	}

	return nil
}

func countHeaders(channels []cache.Channel) []string {
	headers := make([]string, 0, len(channels))
	for _, channel := range channels {
		headers = append(headers, channel.Label+" Count")
	}
	return headers
}

func middleHeaders(profile cache.ProfilePrefs, channels []cache.Channel) []string {
	prefix := "Median"
	switch profile.PrefCalcMiddle {
	case 1:
		prefix = "WAvg"
	case 2:
		prefix = "Avg"
	}
	headers := make([]string, 0, len(channels))
	for _, channel := range channels {
		headers = append(headers, prefix+" "+channel.Label)
	}
	return headers
}

func percentileHeaders(profile cache.ProfilePrefs, channels []cache.Channel) []string {
	prefix := strconv.FormatFloat(profile.PrefCalcPercentile, 'f', -1, 64) + "% "
	headers := make([]string, 0, len(channels))
	for _, channel := range channels {
		headers = append(headers, prefix+channel.Label)
	}
	return headers
}

func maxHeaders(profile cache.ProfilePrefs, channels []cache.Channel) []string {
	prefix := "Max "
	if profile.PrefCalcMax {
		prefix = "99.5% "
	}
	headers := make([]string, 0, len(channels))
	for _, channel := range channels {
		headers = append(headers, prefix+channel.Label)
	}
	return headers
}

func sessionAHI(session *cache.Session, ahiChannels []cache.Channel) float64 {
	hours := session.Hours()
	if hours == 0 {
		return 0
	}
	return sessionAHICount(session, ahiChannels) / hours
}

func sessionAHICount(session *cache.Session, ahiChannels []cache.Channel) float64 {
	total := 0.0
	for _, channel := range ahiChannels {
		total += session.Count(channel.ID)
	}
	return total
}

func (g *dayGroup) firstEnabledMillis() int64 {
	var first int64
	for _, session := range g.Sessions {
		if !session.Enabled || session.First == 0 {
			continue
		}
		if first == 0 || session.First < first {
			first = session.First
		}
	}
	return first
}

func (g *dayGroup) lastEnabledMillis() int64 {
	var last int64
	for _, session := range g.Sessions {
		if !session.Enabled || session.Last == 0 {
			continue
		}
		if session.Last > last {
			last = session.Last
		}
	}
	return last
}

func (g *dayGroup) latestEndMillis() int64 {
	var last int64
	for _, session := range g.Sessions {
		if session.Last > last {
			last = session.Last
		}
	}
	return last
}

func (g *dayGroup) totalTimeMillis() int64 {
	type interval struct {
		start int64
		end   int64
	}

	intervals := make([]interval, 0)
	for _, session := range g.Sessions {
		if !session.Enabled {
			continue
		}
		if len(session.Summary.Slices) == 0 {
			if session.Last > session.First {
				intervals = append(intervals, interval{start: session.First, end: session.Last})
			}
			continue
		}
		for _, slice := range session.Summary.Slices {
			if slice.Status != maskOnStatus || slice.End <= slice.Start {
				continue
			}
			intervals = append(intervals, interval{start: slice.Start, end: slice.End})
		}
	}
	if len(intervals) == 0 {
		return 0
	}

	slices.SortFunc(intervals, func(a, b interval) int {
		if a.start == b.start {
			if a.end == b.end {
				return 0
			}
			if a.end < b.end {
				return -1
			}
			return 1
		}
		if a.start < b.start {
			return -1
		}
		return 1
	})

	current := intervals[0]
	var total int64
	for _, next := range intervals[1:] {
		if next.start <= current.end {
			if next.end > current.end {
				current.end = next.end
			}
			continue
		}
		total += current.end - current.start
		current = next
	}
	total += current.end - current.start
	return total
}

func (g *dayGroup) ahi(ahiChannels []cache.Channel) float64 {
	hours := float64(g.totalTimeMillis()) / 3600000.0
	if hours == 0 {
		return 0
	}
	total := 0.0
	for _, channel := range ahiChannels {
		total += g.count(channel.ID)
	}
	return total / hours
}

func (g *dayGroup) count(channelID uint32) float64 {
	total := 0.0
	for _, session := range g.Sessions {
		if session.Enabled {
			total += session.Count(channelID)
		}
	}
	return total
}

func (g *dayGroup) avg(channelID uint32) float64 {
	total := 0.0
	count := 0.0
	for _, session := range g.Sessions {
		if session.Enabled {
			total += session.Sum(channelID)
			count += session.Count(channelID)
		}
	}
	if count == 0 {
		return 0
	}
	return total / count
}

func (g *dayGroup) wavg(channelID uint32) float64 {
	total := 0.0
	weights := 0.0
	for _, session := range g.Sessions {
		if !session.Enabled {
			continue
		}
		lengthHours := float64(session.LengthMillis()) / 3600000.0
		if lengthHours <= 0 {
			continue
		}
		total += session.WAvg(channelID) * lengthHours
		weights += lengthHours
	}
	if weights == 0 {
		return 0
	}
	return total / weights
}

func (g *dayGroup) max(channelID uint32) float64 {
	first := true
	maximum := 0.0
	for _, session := range g.Sessions {
		if !session.Enabled {
			continue
		}
		value := session.Max(channelID)
		if first || value > maximum {
			maximum = value
			first = false
		}
	}
	if first {
		return 0
	}
	return maximum
}

func (g *dayGroup) percentile(channelID uint32, percent float64) float64 {
	weights := map[int16]int64{}
	var totalWeight int64
	var gain float64

	for _, session := range g.Sessions {
		if !session.Enabled {
			continue
		}
		if sessionGain, ok := session.Summary.Gain[channelID]; ok {
			gain = float64(sessionGain)
		}
		if timeSummary := session.Summary.TimeSummary[channelID]; len(timeSummary) > 0 {
			for rawValue, weight := range timeSummary {
				weights[rawValue] += int64(weight)
				totalWeight += int64(weight)
			}
			continue
		}
		if valueSummary := session.Summary.ValueSummary[channelID]; len(valueSummary) > 0 {
			for rawValue, weight := range valueSummary {
				weights[rawValue] += int64(weight)
				totalWeight += int64(weight)
			}
		}
	}

	if totalWeight == 0 || len(weights) == 0 {
		return 0
	}

	keys := make([]int16, 0, len(weights))
	for key := range weights {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	if len(keys) == 1 {
		return float64(keys[0]) * gain
	}

	pct := 100.0 * percent
	nth := float64(totalWeight) * percent
	nthFloor := math.Floor(nth)

	var sum1 int64
	var v1 float64
	var w1 int64
	k := 0
	for ; k < len(keys); k++ {
		v1 = float64(keys[k]) * gain
		w1 = weights[keys[k]]
		sum1 += w1
		if float64(sum1) > nthFloor {
			return v1
		}
		if float64(sum1) == nthFloor {
			break
		}
	}
	if k >= len(keys)-1 {
		return v1
	}

	v2 := float64(keys[k+1]) * gain
	w2 := weights[keys[k+1]]
	sum2 := sum1 + w2
	px := 100.0 / float64(totalWeight)
	p1 := px * (float64(sum1) - float64(w1)/2.0)
	p2 := px * (float64(sum2) - float64(w2)/2.0)
	if p2 == p1 {
		return v1
	}
	return v1 + ((pct-p1)/(p2-p1))*(v2-v1)
}

func (g *dayGroup) calcMiddle(prefCalcMiddle int, channelID uint32) float64 {
	switch prefCalcMiddle {
	case 0:
		return g.percentile(channelID, 0.5)
	case 1:
		return g.wavg(channelID)
	default:
		return g.avg(channelID)
	}
}

func (g *dayGroup) calcMax(prefCalcMax bool, channelID uint32) float64 {
	if prefCalcMax {
		return g.percentile(channelID, 0.995)
	}
	return g.max(channelID)
}

func defaultOutputPath(ds *cache.Dataset, cfg Config) string {
	dir := ds.Profile.LastExportCSVPath
	if dir == "" {
		dir = "."
	}
	mode := "Summary"
	switch cfg.Mode {
	case ModeSessions:
		mode = "Sessions"
	case ModeDetails:
		mode = "Details"
	}
	name := fmt.Sprintf("OSCAR_%s_%s_%s", ds.Profile.UserName, mode, cfg.From.Format("2006-01-02"))
	if !cfg.From.Equal(cfg.To) {
		name += "_" + cfg.To.Format("2006-01-02")
	}
	name += ".csv"
	return filepath.Join(dir, name)
}

func startOfDay(ts time.Time) time.Time {
	year, month, day := ts.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, ts.Location())
}

func clockOfDay(ts time.Time) time.Duration {
	return time.Duration(ts.Hour())*time.Hour +
		time.Duration(ts.Minute())*time.Minute +
		time.Duration(ts.Second())*time.Second
}

func formatDateTime(ms int64, loc *time.Location) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).In(loc).Format("2006-01-02T15:04:05")
}

func formatDuration(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	totalSeconds := ms / 1000
	hours := totalSeconds / 3600
	minutes := (totalSeconds / 60) % 60
	seconds := totalSeconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

func formatAHI(value float64) string {
	return fmt.Sprintf("%.3f", sanitize(value))
}

func formatDetails(value float64) string {
	return fmt.Sprintf("%.2f", sanitize(value))
}

func formatGeneral(value float64) string {
	value = sanitize(value)
	return strconv.FormatFloat(float64(float32(value)), 'g', 6, 32)
}

func sanitize(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	if math.Abs(value) < 1e-9 {
		return 0
	}
	return value
}
