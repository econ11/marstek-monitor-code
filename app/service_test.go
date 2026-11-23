package app

import (
	"errors"
	"testing"

	"github.com/econ11/marstek-monitor-code/core/monitor"
)

func TestJournalPageWithoutMonitor(t *testing.T) {
	var s MonitorService
	if _, err := s.JournalPage("2026-11-20", "2026-11-21", "", "", 200, "all", ""); !errors.Is(err, monitor.ErrNoStation) {
		t.Fatalf("JournalPage err = %v, want ErrNoStation", err)
	}
	if _, err := s.JournalSince("2026-11-20", "2026-11-21", "2026-11-21T10:00:00Z", "conn.x:1", "all", ""); !errors.Is(err, monitor.ErrNoStation) {
		t.Fatalf("JournalSince err = %v, want ErrNoStation", err)
	}
}

func TestJournalCursorParse(t *testing.T) {
	c, err := parseCursor("", "")
	if err != nil || c != nil {
		t.Fatalf("empty cursor = %v, %v; want nil", c, err)
	}
	c, err = parseCursor("2026-11-21T10:00:00.5Z", "conn.x:1")
	if err != nil || c == nil || c.ID != "conn.x:1" || c.At.Nanosecond() != 5e8 {
		t.Fatalf("cursor = %+v, %v", c, err)
	}
	if _, err := parseCursor("nope", "x"); err == nil {
		t.Fatal("a bad time must fail")
	}
}
