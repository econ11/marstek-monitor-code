// Package model parses device status into the Sample consumed by the core.
package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type Source string

const SourceLAN Source = "lan"

type GridSignal int

const (
	GridUnknown GridSignal = iota
	GridPresent
	GridAbsent
)

// ESStatus holds ES.GetStatus fields; nil = missing, null or of the wrong type
// (unknown, never 0). The key names live in esFields only.
type ESStatus struct {
	SOC, CapWh, PVW, OngridW, OffgridW, BatW *float64
	// bad: keys present with a value of the wrong type (esKeys bits), so
	// BatteryPower tells a bad value from a missing one.
	bad esKeys
}

type esKeys uint8

const (
	keyPV esKeys = 1 << iota
	keyOngrid
	keyOffgrid
	keyBat
)

type esField struct {
	key string
	dst **float64
	bit esKeys
}

// esFields maps each ES.GetStatus key to its field and its bad bit.
func (es *ESStatus) esFields() []esField {
	return []esField{
		{"bat_soc", &es.SOC, 0}, {"bat_cap", &es.CapWh, 0}, {"pv_power", &es.PVW, keyPV},
		{"ongrid_power", &es.OngridW, keyOngrid}, {"offgrid_power", &es.OffgridW, keyOffgrid},
		{"bat_power", &es.BatW, keyBat},
	}
}

// Sample is one normalized reading. BatteryW > 0 charge, < 0 discharge.
type Sample struct {
	StationID string
	At        time.Time
	Source    Source
	SOC       *float64
	BatteryW  *float64
	OngridW   *float64
	OffgridW  *float64
	PVW       *float64
	Grid      GridSignal
	Raw       json.RawMessage
}

// ParseESStatus reads the fields one by one: a field with the wrong type is
// unknown (nil) and the others are kept, so one bad value does not cost the
// whole sample. A non-object result is an error; "null" is ESStatus{}.
func ParseESStatus(raw json.RawMessage) (ESStatus, error) {
	var es ESStatus
	if len(raw) == 0 {
		return es, errors.New("model: empty ES.GetStatus result")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return ESStatus{}, fmt.Errorf("model: parse ES.GetStatus: %w", err)
	}
	for _, f := range es.esFields() {
		v, ok := fields[f.key]
		if !ok {
			continue
		}
		var p *float64
		if json.Unmarshal(v, &p) == nil {
			*f.dst = p
		} else {
			es.bad |= f.bit
		}
	}
	return es, nil
}

// GridDeadbandW is the power deadband (W) around zero for grid detection.
const GridDeadbandW = 20

// GridFromStatus derives grid presence (owner-approved rule, docs/dev-notes.md).
// Present by charging: ongrid < -deadband. Absent: offgrid > deadband and
// ongrid >= -deadband. Present by idle: offgrid <= deadband. Otherwise unknown.
func GridFromStatus(es ESStatus) GridSignal {
	on, off := es.OngridW, es.OffgridW
	if on != nil && *on < -GridDeadbandW {
		return GridPresent
	}
	if on != nil && off != nil && *off > GridDeadbandW {
		return GridAbsent
	}
	if off != nil && *off <= GridDeadbandW {
		return GridPresent
	}
	return GridUnknown
}

// BatteryPower returns battery power, + charge / - discharge, or nil if unknowable.
// BatteryW = -(ongrid + offgrid - pv); nil pv counts as 0. If ongrid or offgrid is
// missing it falls back to -bat_power (sign UNVERIFIED: absent on fw 151).
// A key that is present with a bad value (ParseESStatus) makes it nil: "nil pv
// counts as 0" is for a missing pv_power, not an unreadable one.
func BatteryPower(es ESStatus) *float64 {
	if es.bad&(keyPV|keyOngrid|keyOffgrid) != 0 {
		return nil
	}
	if (es.OngridW == nil || es.OffgridW == nil) && es.bad&keyBat != 0 {
		return nil
	}
	if es.OngridW != nil && es.OffgridW != nil {
		var pv float64
		if es.PVW != nil {
			pv = *es.PVW
		}
		v := 0 - (*es.OngridW + *es.OffgridW - pv)
		return &v
	}
	if es.BatW != nil {
		v := 0 - *es.BatW
		return &v
	}
	return nil
}

func NewSample(stationID string, at time.Time, src Source, raw json.RawMessage) (Sample, error) {
	es, err := ParseESStatus(raw)
	if err != nil {
		return Sample{}, err
	}
	return Sample{
		StationID: stationID, At: at, Source: src,
		SOC: es.SOC, BatteryW: BatteryPower(es),
		OngridW: es.OngridW, OffgridW: es.OffgridW, PVW: es.PVW,
		Grid: GridFromStatus(es), Raw: raw,
	}, nil
}
