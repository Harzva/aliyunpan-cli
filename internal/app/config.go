package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const (
	configEnvDir = "ALIYUNPAN_CLI_CONFIG_DIR"
	configName   = "config.json"
)

type Config struct {
	Version       int                 `json:"version"`
	ActiveProfile string              `json:"activeProfile"`
	Profiles      map[string]*Profile `json:"profiles"`
}

type Profile struct {
	Name          string      `json:"name"`
	Source        string      `json:"source"`
	UserID        string      `json:"userId"`
	Nickname      string      `json:"nickname"`
	AccountName   string      `json:"accountName"`
	AccessToken   string      `json:"accessToken"`
	RefreshToken  string      `json:"refreshToken,omitempty"`
	ExpiresAt     int64       `json:"expiresAt"`
	ClientID      string      `json:"clientId,omitempty"`
	ClientSecret  string      `json:"clientSecret,omitempty"`
	TokenEndpoint string      `json:"tokenEndpoint,omitempty"`
	ActiveDriveID string      `json:"activeDriveId"`
	Drives        []DriveInfo `json:"drives"`
	ImportedAt    string      `json:"importedAt"`
}

type DriveInfo struct {
	DriveID   string `json:"driveId"`
	DriveName string `json:"driveName"`
	DriveTag  string `json:"driveTag"`
}

func defaultConfigDir() (string, error) {
	if dir := os.Getenv(configEnvDir); dir != "" {
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "aliyunpan-cli"), nil
}

func configPath(dir string) (string, error) {
	if dir == "" {
		var err error
		dir, err = defaultConfigDir()
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(dir, configName), nil
}

func loadConfig(dir string) (*Config, string, error) {
	path, err := configPath(dir)
	if err != nil {
		return nil, "", err
	}
	cfg := &Config{Version: 1, Profiles: map[string]*Profile{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, path, nil
		}
		return nil, path, err
	}
	if len(data) == 0 {
		return cfg, path, nil
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, path, err
	}
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]*Profile{}
	}
	return cfg, path, nil
}

func saveConfig(path string, cfg *Config) error {
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]*Profile{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func activeProfile(cfg *Config) (*Profile, error) {
	if cfg.ActiveProfile == "" {
		return nil, authError("no active profile; run auth login or auth import")
	}
	p := cfg.Profiles[cfg.ActiveProfile]
	if p == nil {
		return nil, authError("active profile %q not found", cfg.ActiveProfile)
	}
	return p, nil
}

func (p *Profile) public() profileOutput {
	return profileOutput{
		Name:          p.Name,
		Source:        p.Source,
		UserID:        p.UserID,
		Nickname:      p.Nickname,
		AccountName:   p.AccountName,
		ExpiresAt:     p.ExpiresAt,
		ExpiresAtText: time.Unix(p.ExpiresAt, 0).Format(time.RFC3339),
		HasRefresh:    p.RefreshToken != "",
		ActiveDriveID: p.ActiveDriveID,
		Drives:        p.Drives,
	}
}

func (p *Profile) driveByRef(ref string) *DriveInfo {
	if ref == "" {
		ref = p.ActiveDriveID
	}
	switch ref {
	case "file", "backup", "bak", "备份盘":
		ref = "File"
	case "resource", "resources", "资源库":
		ref = "Resource"
	}
	for i := range p.Drives {
		d := &p.Drives[i]
		if d.DriveID == ref || d.DriveTag == ref || d.DriveName == ref {
			return d
		}
	}
	return nil
}

func nowRFC3339() string {
	return time.Now().Format(time.RFC3339)
}
