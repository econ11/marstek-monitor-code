package model

import "strings"

type Profile struct {
	Model                                        string
	NominalWh, MaxChargeW, MaxDischargeW, MinSOC float64
	// CycleLife is the rated cycle count at RatedDoD (a fraction: 0.9 = 90 %
	// depth of discharge); one counted cycle is RatedDoD × NominalWh given out.
	CycleLife          int
	RatedDoD           float64
	TempMinC, TempMaxC float64
	Tested             bool
}

// ProfileFor matches the device string case-insensitively, ignoring spaces
// ("VenusE 3.0" as discovered, "Venus E 3.0" tolerated). Model is the device
// string trimmed. Unknown models get zero values so the UI shows "max unknown".
func ProfileFor(device string) Profile {
	device = strings.TrimSpace(device)
	if modelKey(device) == "venuse3.0" {
		// CycleLife: the maker rates it for over 6000 cycles at 90 % DoD (the
		// 10–100 % window), so one cycle is 0.9 × 5120 = 4608 Wh.
		// MaxDischargeW is the off-grid (EPS) output: 2.5 kVA nominal, about
		// 3.5 kVA peak for 10 s; the owner measured about 2380 W with a kettle.
		// The 800 W figure is only the plug-in on-grid feed limit and is not
		// what the power ring is scaled to.
		return Profile{Model: device, NominalWh: 5120, MaxChargeW: 2500, MaxDischargeW: 2500,
			MinSOC: 10, CycleLife: 6000, RatedDoD: 0.9, TempMinC: 0, TempMaxC: 45, Tested: true}
	}
	return Profile{Model: device}
}

// modelKey is the device string lower-cased without spaces.
func modelKey(device string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(device), " ", ""))
}
