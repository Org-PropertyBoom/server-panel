package services

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

const settingsDBEnv = "SETTINGS_DB_PATH"

type SettingsService struct {
	db *sql.DB
}

func NewSettingsService() (*SettingsService, error) {
	path := settingsDBPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		_ = db.Close()
		return nil, err
	}
	for oldKey, newKey := range map[string]string{
		"app_name": "general_app_name", "color_mode": "general_color_mode", "header_apps": "apps_header",
	} {
		if _, err := db.Exec(`INSERT OR IGNORE INTO settings (key, value)
			SELECT ?, value FROM settings WHERE key = ?`, newKey, oldKey); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	for key, value := range map[string]string{
		"general_app_name":    "MThan VPS Panel",
		"general_color_mode":  "system",
		"apps_header":         "[]",
		"users_default_shell": "/bin/bash",
		"users_home_base":     "/home",
		"users_create_home":   "true",
	} {
		if _, err := db.Exec("INSERT OR IGNORE INTO settings (key, value) VALUES (?, ?)", key, value); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	return &SettingsService{db: db}, nil
}

func (s *SettingsService) All() (map[string]string, error) {
	rows, err := s.db.Query("SELECT key, value FROM settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	settings := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		settings[key] = value
	}
	return settings, rows.Err()
}

func (s *SettingsService) Set(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`, key, value)
	return err
}

func (s *SettingsService) Get(key, fallback string) string {
	var value string
	if err := s.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value); err != nil {
		return fallback
	}
	return value
}

func settingsDBPath() string {
	if path := os.Getenv(settingsDBEnv); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), ".mthan-vps", "data", "db.sqlite")
	}
	return filepath.Join(home, ".mthan-vps", "data", "db.sqlite")
}
