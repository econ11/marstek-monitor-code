// Package export writes the user's data as CSV files for a spreadsheet:
// outages, journal events and the charge history. UTF-8 with a BOM (Excel
// then reads it as UTF-8); the separator and the decimal mark follow the
// app's language (Format: uk «;» and a decimal comma, as Excel expects in a
// Ukrainian locale; en «,» and a point); times in RFC 3339 with the local
// offset, English headers, CRLF line ends. An empty cell is unknown or not
// applicable. Only the user's own figures are written: no station id, MAC,
// SSID or raw station JSON.
package export

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/econ11/marstek-monitor-code/core/history"
	"github.com/econ11/marstek-monitor-code/core/journal"
	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/store"
)

var (
	outageHeader = []string{"start", "end", "duration_min", "known_min", "at_least", "soc_start", "soc_end", "died_at", "start_approx", "end_approx"}
	eventHeader  = []string{"at", "level", "code", "text"}
	chargeHeader = []string{"at", "soc", "soc_min", "soc_max", "battery_w", "ongrid_w", "offgrid_w", "pv_w", "grid", "resolution"}
)

// Chunks the charge history is read in, so a year of minutes or 30 days of
// 5 s samples never sits in memory at once and the store's one connection is
// free between chunks (the monitor keeps writing during an export).
const (
	minuteChunk = 7 * 24 * time.Hour
	rawChunk    = 24 * time.Hour
)

// Format is the CSV dialect: the field separator and the decimal mark; and
// EventText, the events' «text» column: the entry's sentence in the app's
// language, as the Errors tab shows it (the copy texts live in the app's
// catalogue, core has none). nil writes the stored JSON args.
type Format struct {
	Sep          rune
	DecimalComma bool
	EventText    func(journal.Entry) string
}

// FormatFor is the dialect for the app's language: "uk" gives «;» and a
// decimal comma, anything else «,» and a point.
func FormatFor(lang string) Format {
	if lang == "uk" {
		return Format{Sep: ';', DecimalComma: true}
	}
	return Format{Sep: ',', DecimalComma: false}
}

// Write writes the station's data in [from, to) to three files in dir and
// returns their paths: marstek-outages-<from>-<to>.csv, marstek-events-… and
// marstek-charge-…, the dates (YYYYMMDD) being the first and the last local
// day of the range, in the dialect f. Existing files of the same name are replaced; on an
// error the files already written are removed.
//
//   - outages: those that touch the range, with the bounds core/history
//     gives them (an ongoing one: no end, its duration up to the last
//     sample, at_least);
//   - events: the journal's events (journal.Codes) only; the debug events
//     (link.*, monitor.cadence) and anything else stay out;
//   - charge: the minute aggregates first (older than 30 days: soc =
//     soc_avg, battery_w = bat_w_avg, no ongrid/offgrid/pv/grid), then the
//     raw samples (the last 30 days: no soc_min/soc_max).
func Write(dir string, st *store.Store, stationID string, from, to, now time.Time, f Format) ([]string, error) {
	if !to.After(from) {
		return nil, errors.New("export: empty range")
	}
	span := from.In(time.Local).Format("20060102") + "-" + to.Add(-time.Nanosecond).In(time.Local).Format("20060102")
	files := []struct {
		kind string
		fill func(*csv.Writer) error
	}{
		{"outages", func(w *csv.Writer) error { return writeOutages(w, f, st, stationID, from, to, now) }},
		{"events", func(w *csv.Writer) error { return writeEvents(w, f, st, stationID, from, to) }},
		{"charge", func(w *csv.Writer) error { return writeCharge(w, f, st, stationID, from, to) }},
	}
	var paths []string
	for _, fl := range files {
		p := filepath.Join(dir, "marstek-"+fl.kind+"-"+span+".csv")
		paths = append(paths, p)
		if err := writeFile(p, f.Sep, fl.fill); err != nil {
			for _, q := range paths {
				os.Remove(q)
			}
			return nil, fmt.Errorf("export: %s: %w", fl.kind, err)
		}
	}
	return paths, nil
}

func writeFile(path string, sep rune, fill func(*csv.Writer) error) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	b := bufio.NewWriter(f)
	b.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	w := csv.NewWriter(b)
	w.Comma, w.UseCRLF = sep, true
	err = fill(w)
	if err == nil {
		w.Flush()
		err = w.Error()
	}
	if err == nil {
		err = b.Flush()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

func stamp(t time.Time) string { return t.In(time.Local).Format(time.RFC3339) }

func stampPtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return stamp(*t)
}

// num: at most 2 decimals, no exponent, the format's decimal mark.
func (f Format) num(v float64) string {
	v = math.Round(v*100) / 100
	if v == 0 {
		v = 0 // no «-0»
	}
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if f.DecimalComma {
		s = strings.Replace(s, ".", ",", 1)
	}
	return s
}

func (f Format) numPtr(v *float64) string {
	if v == nil {
		return ""
	}
	return f.num(*v)
}

func (f Format) minutes(d time.Duration) string { return f.num(d.Minutes()) }

func writeOutages(w *csv.Writer, f Format, st *store.Store, id string, from, to, now time.Time) error {
	if err := w.Write(outageHeader); err != nil {
		return err
	}
	outs, err := st.Outages(id, from, to)
	if err != nil {
		return err
	}
	if len(outs) == 0 {
		return nil
	}
	gaps, err := st.Gaps(id, outs[0].Start.Add(-history.BoundarySlack), now.Add(time.Second))
	if err != nil {
		return err
	}
	// The export does not know whether the link is live: an ongoing outage
	// counts up to the last sample, as a lower bound.
	lastAt := now
	if x, ok, err := st.LastSample(id); err != nil {
		return err
	} else if ok {
		lastAt = x.At
	}
	for _, o := range outs {
		b := history.Annotate(o, gaps, now, lastAt, false)
		dur := max(b.AsOf.Sub(o.Start), 0)
		if err := w.Write([]string{stamp(o.Start), stampPtr(o.End), f.minutes(dur), f.minutes(max(b.Known, 0)),
			strconv.FormatBool(b.AtLeast), f.numPtr(o.SOCStart), f.numPtr(o.SOCEnd), stampPtr(o.DiedAt),
			strconv.FormatBool(b.StartApprox), strconv.FormatBool(b.EndApprox)}); err != nil {
			return err
		}
	}
	return nil
}

func writeEvents(w *csv.Writer, f Format, st *store.Store, id string, from, to time.Time) error {
	if err := w.Write(eventHeader); err != nil {
		return err
	}
	var es []store.Event
	for _, code := range journal.Codes {
		got, err := st.Events(id, code, from, to)
		if err != nil {
			return err
		}
		es = append(es, got...)
	}
	sort.Slice(es, func(i, j int) bool {
		if !es[i].At.Equal(es[j].At) {
			return es[i].At.Before(es[j].At)
		}
		return es[i].ID < es[j].ID
	})
	for _, e := range es {
		text, level := e.Text, e.Level
		if journal.IsLegacyCode(e.Code) {
			level = "info" // the level the tab shows
		}
		if f.EventText != nil {
			// One event of journal.Codes: one entry, its copy key and args.
			if en := journal.Build([]store.Event{e}, nil, e.At); len(en) == 1 {
				text = f.EventText(en[0])
			}
		}
		if err := w.Write([]string{stamp(e.At), level, e.Code, text}); err != nil {
			return err
		}
	}
	return nil
}

func grid(g model.GridSignal) string {
	switch g {
	case model.GridPresent:
		return "present"
	case model.GridAbsent:
		return "absent"
	}
	return ""
}

func writeCharge(w *csv.Writer, f Format, st *store.Store, id string, from, to time.Time) error {
	if err := w.Write(chargeHeader); err != nil {
		return err
	}
	for a := from; a.Before(to); a = a.Add(minuteChunk) {
		ps, err := st.MinuteSeries(id, a, earlier(a.Add(minuteChunk), to))
		if err != nil {
			return err
		}
		for _, p := range ps {
			if err := w.Write([]string{stamp(p.At), f.numPtr(p.SOCAvg), f.numPtr(p.SOCMin), f.numPtr(p.SOCMax), f.numPtr(p.BatWAvg),
				"", "", "", "", "minute"}); err != nil {
				return err
			}
		}
	}
	for a := from; a.Before(to); a = a.Add(rawChunk) {
		xs, err := st.PlainSamples(id, a, earlier(a.Add(rawChunk), to))
		if err != nil {
			return err
		}
		for _, x := range xs {
			if err := w.Write([]string{stamp(x.At), f.numPtr(x.SOC), "", "", f.numPtr(x.BatteryW),
				f.numPtr(x.OngridW), f.numPtr(x.OffgridW), f.numPtr(x.PVW), grid(x.Grid), "raw"}); err != nil {
				return err
			}
		}
	}
	return nil
}

func earlier(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
