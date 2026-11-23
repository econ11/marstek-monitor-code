package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/model"
)

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func sampleAt(i int) model.Sample {
	return model.Sample{StationID: "st1", At: time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Second),
		Source: model.SourceLAN, SOC: f(50), Grid: model.GridPresent}
}

// Review Focus 3: a backup while the monitor writes is a consistent copy
// (VACUUM INTO on the live connection), and the live DB loses nothing.
func TestBackupWhileWriting(t *testing.T) {
	s, _ := openTemp(t)
	const before = 200
	for i := 0; i < before; i++ {
		if err := s.InsertSample(sampleAt(i)); err != nil {
			t.Fatal(err)
		}
	}
	const during = 300
	var wg sync.WaitGroup
	wg.Add(1)
	errs := make(chan error, 1)
	go func() {
		defer wg.Done()
		for i := before; i < before+during; i++ {
			if err := s.InsertSample(sampleAt(i)); err != nil {
				errs <- err
				return
			}
		}
	}()
	dst := filepath.Join(t.TempDir(), "backups", "monitor-1.0.0.db")
	var backupErr error
	for i := 0; i < 3; i++ { // several backups while the writer runs; each replaces the last
		if backupErr = s.BackupTo(dst); backupErr != nil {
			break
		}
	}
	wg.Wait()
	close(errs)
	if err := <-errs; err != nil {
		t.Fatalf("insert during backup: %v", err)
	}
	if backupErr != nil {
		t.Fatalf("BackupTo: %v", backupErr)
	}
	if _, err := os.Stat(dst + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temporary file left behind: %v", err)
	}

	// The live DB has every row.
	if n := countRows(t, s.db, "samples"); n != before+during {
		t.Errorf("live samples = %d, want %d", n, before+during)
	}
	// The backup opens as a store (schema version kept), passes the
	// integrity check, and holds a prefix of the writes: every row it has
	// is contiguous from the start (a snapshot, not a torn copy).
	b, err := Open(dst)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer b.Close()
	var ok string
	if err := b.db.QueryRow(`PRAGMA integrity_check`).Scan(&ok); err != nil || ok != "ok" {
		t.Fatalf("integrity_check = %q, %v", ok, err)
	}
	n := countRows(t, b.db, "samples")
	if n < before || n > before+during {
		t.Fatalf("backup samples = %d, want %d..%d", n, before, before+during)
	}
	var maxAt int64
	if err := b.db.QueryRow(`SELECT MAX(at) FROM samples`).Scan(&maxAt); err != nil {
		t.Fatal(err)
	}
	if want := ms(sampleAt(n - 1).At); maxAt != want {
		t.Errorf("backup is not a prefix: max(at)=%d, want %d for %d rows", maxAt, want, n)
	}
}

func TestBackupReplacesAndKeepsSettings(t *testing.T) {
	s, _ := openTemp(t)
	dst := filepath.Join(t.TempDir(), "monitor-0.9.0.db")
	if err := os.WriteFile(dst, []byte("old junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetting("k", "v"); err != nil {
		t.Fatal(err)
	}
	if err := s.BackupTo(dst); err != nil {
		t.Fatal(err)
	}
	b, err := Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if v, ok, err := b.GetSetting("k"); err != nil || !ok || v != "v" {
		t.Errorf("setting in backup = %q %v %v", v, ok, err)
	}
}

func TestPruneBackupsKeeps3(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	names := []string{"monitor-0.1.0.db", "monitor-0.2.0.db", "monitor-0.3.0.db", "monitor-0.4.0.db", "monitor-0.5.0.db"}
	for i, n := range names {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		mt := base.Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
	// Not backups: never touched.
	for _, n := range []string{"notes.txt", "monitor.db", "monitor-0.6.0.db.tmp"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := PruneBackups(dir, 3); err != nil {
		t.Fatal(err)
	}
	ents, _ := os.ReadDir(dir)
	var got []string
	for _, e := range ents {
		got = append(got, e.Name())
	}
	sort.Strings(got)
	want := []string{"monitor-0.3.0.db", "monitor-0.4.0.db", "monitor-0.5.0.db", "monitor-0.6.0.db.tmp", "monitor.db", "notes.txt"}
	if len(got) != len(want) {
		t.Fatalf("left %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("left %v, want %v", got, want)
		}
	}
	// A missing folder is not an error (no backup yet).
	if err := PruneBackups(filepath.Join(dir, "none"), 3); err != nil {
		t.Errorf("missing dir: %v", err)
	}
}
