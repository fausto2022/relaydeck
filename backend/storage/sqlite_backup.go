package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

const DefaultSQLiteBackupKeep = 7

type SQLiteBackups struct {
	db        *gorm.DB
	directory string
	keep      int
	now       func() time.Time
}

func NewSQLiteBackups(db *gorm.DB, cfg DBConfig, keep int) *SQLiteBackups {
	driver := DBDriver(strings.ToLower(string(cfg.Driver)))
	if driver != "" && driver != DBDriverSQLite {
		return nil
	}
	if keep <= 0 {
		keep = DefaultSQLiteBackupKeep
	}
	databasePath := resolveSQLitePath(cfg.SQLitePath())
	return &SQLiteBackups{
		db:        db,
		directory: filepath.Join(filepath.Dir(databasePath), "backups"),
		keep:      keep,
		now:       time.Now,
	}
}

func (b *SQLiteBackups) Backup() (string, error) {
	if b == nil || b.db == nil {
		return "", nil
	}
	if err := os.MkdirAll(b.directory, 0o755); err != nil {
		return "", fmt.Errorf("create sqlite backup directory: %w", err)
	}
	name := "relaydeck-" + b.now().Format("20060102-150405.000000000") + ".db"
	path := filepath.Join(b.directory, name)
	escapedPath := strings.ReplaceAll(path, "'", "''")
	if err := b.db.Exec("VACUUM INTO '" + escapedPath + "'").Error; err != nil {
		return "", fmt.Errorf("vacuum sqlite backup: %w", err)
	}
	if err := b.rotate(); err != nil {
		return path, err
	}
	return path, nil
}

func (b *SQLiteBackups) rotate() error {
	entries, err := os.ReadDir(b.directory)
	if err != nil {
		return fmt.Errorf("list sqlite backups: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "relaydeck-") && strings.HasSuffix(entry.Name(), ".db") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for len(names) > b.keep {
		path := filepath.Join(b.directory, names[0])
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove old sqlite backup: %w", err)
		}
		names = names[1:]
	}
	return nil
}
