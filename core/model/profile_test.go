package model

import (
	"strings"
	"testing"
)

func TestProfileVenusE3(t *testing.T) {
	want := Profile{Model: "VenusE 3.0", NominalWh: 5120, MaxChargeW: 2500, MaxDischargeW: 2500, MinSOC: 10, CycleLife: 6000, RatedDoD: 0.9, TempMinC: 0, TempMaxC: 45, Tested: true}
	for _, d := range []string{"VenusE 3.0", "Venus E 3.0", "venuse 3.0", " VENUS E 3.0 "} {
		got := ProfileFor(d)
		w := want
		w.Model = strings.TrimSpace(d)
		if got != w {
			t.Errorf("%q: got %+v", d, got)
		}
	}
}

func TestProfileUnknown(t *testing.T) {
	p := ProfileFor("X")
	if p.Tested || p.NominalWh != 0 || p.Model != "X" {
		t.Errorf("got %+v", p)
	}
}

func TestProfileModelTrimmed(t *testing.T) {
	if got := ProfileFor(" VenusE 3.0 ").Model; got != "VenusE 3.0" {
		t.Errorf("Venus: Model = %q", got)
	}
	if got := ProfileFor(" X ").Model; got != "X" {
		t.Errorf("unknown: Model = %q", got)
	}
}
