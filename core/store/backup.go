package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// BackupTo writes a consistent copy of the database to path (the one-click
// update's backup, plan 3 Task 5). It runs VACUUM INTO on the live
// connection: the copy is one read snapshot, never a raw copy of a file the
// monitor is writing, and writes made meanwhile wait for the one connection
// and land in the live DB only. The copy goes to path+".tmp" first and is
// renamed over path, so a failure leaves an earlier backup at path as it was.
func (s *Store) BackupTo(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("store: backup folder: %w", err)
	}
	tmp := path + ".tmp"
	// VACUUM INTO refuses an existing file (a leftover of a failed run).
	if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("store: backup: %w", err)
	}
	if _, err := s.db.Exec(`VACUUM INTO ?`, tmp); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("store: backup: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("store: backup: %w", err)
	}
	return nil
}

// PruneBackups keeps the keep newest backups (monitor-*.db, by modification
// time) in dir and deletes the older ones. Other files are left alone; a
// missing dir is no error.
func PruneBackups(dir string, keep int) error {
	ents, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("store: backups: %w", err)
	}
	type backup struct {
		name string
		mod  int64
	}
	var bs []backup
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, "monitor-") || !strings.HasSuffix(n, ".db") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		bs = append(bs, backup{n, info.ModTime().UnixNano()})
	}
	sort.Slice(bs, func(i, j int) bool {
		if bs[i].mod != bs[j].mod {
			return bs[i].mod > bs[j].mod
		}
		return bs[i].name > bs[j].name
	})
	var first error
	for i := keep; i < len(bs); i++ {
		if err := os.Remove(filepath.Join(dir, bs[i].name)); err != nil && first == nil {
			first = fmt.Errorf("store: prune backups: %w", err)
		}
	}
	return first
}
