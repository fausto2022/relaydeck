package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteBackupsCreatesConsistentCopyAndRotates(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "relaydeck.db")
	db, err := Open(DBConfig{Driver: DBDriverSQLite, Path: databasePath})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get database handle: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&Channel{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	if err := db.Create(&Channel{Name: "source", Type: ChannelTypeSub2API, SiteURL: "https://example.com"}).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}

	backups := NewSQLiteBackups(db, DBConfig{Driver: DBDriverSQLite, Path: databasePath}, 2)
	if err := os.MkdirAll(backups.directory, 0o755); err != nil {
		t.Fatalf("create backup directory: %v", err)
	}
	manualBackup := filepath.Join(backups.directory, "relaydeck-before-release.db")
	if err := os.WriteFile(manualBackup, []byte("manual"), 0o600); err != nil {
		t.Fatalf("create manual backup: %v", err)
	}
	current := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	backups.now = func() time.Time { return current }
	first, err := backups.Backup()
	if err != nil {
		t.Fatalf("first backup: %v", err)
	}
	copyDB, err := Open(DBConfig{Driver: DBDriverSQLite, Path: first})
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	var count int64
	if err := copyDB.Model(&Channel{}).Count(&count).Error; err != nil {
		t.Fatalf("count backup channels: %v", err)
	}
	if count != 1 {
		t.Fatalf("backup channel count = %d, want 1", count)
	}
	copySQL, _ := copyDB.DB()
	_ = copySQL.Close()

	for i := 0; i < 2; i++ {
		current = current.Add(time.Second)
		if _, err := backups.Backup(); err != nil {
			t.Fatalf("backup %d: %v", i+2, err)
		}
	}
	entries, err := os.ReadDir(backups.directory)
	if err != nil {
		t.Fatalf("read backup directory: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("backup count = %d, want 2 automatic and 1 manual", len(entries))
	}
	if _, err := os.Stat(manualBackup); err != nil {
		t.Fatalf("manual backup was removed: %v", err)
	}
}
