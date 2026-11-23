package i18n

import (
	"sort"
	"strings"
	"testing"
)

// "Станція розрядилась" is always a conclusion (LIKELY_DEAD, flows 2.2),
// never an observed fact: every text that says so must say «ймовірно».
func TestRanOutTextsSayProbably(t *testing.T) {
	var ranOut, bad []string
	for k, v := range UK {
		low := strings.ToLower(v)
		if !strings.Contains(low, "розрядилась") {
			continue
		}
		ranOut = append(ranOut, k)
		if !strings.Contains(low, "ймовірно") {
			bad = append(bad, k)
		}
	}
	sort.Strings(ranOut)
	sort.Strings(bad)
	want := []string{"journal.likelyOut", "notify.gridBack.stationWasOut.body", "notify.likelyOut.title"}
	if strings.Join(ranOut, " ") != strings.Join(want, " ") {
		t.Fatalf("texts saying the station ran out = %v, want %v (the scan is off)", ranOut, want)
	}
	if len(bad) != 0 {
		t.Fatalf("without «ймовірно»: %v", bad)
	}
}
