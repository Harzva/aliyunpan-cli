package app

import (
	"github.com/tickstep/aliyunpan-api/aliyunpan"
	"github.com/tickstep/aliyunpan-api/aliyunpan/apierror"
	"github.com/tickstep/aliyunpan-api/aliyunpan_open"
	"github.com/tickstep/aliyunpan-api/aliyunpan_open/openapi"
)

type session struct {
	cfg     *Config
	cfgPath string
	profile *Profile
	client  *aliyunpan_open.OpenPanClient
}

func loadSession(configDir string) (*session, error) {
	cfg, path, err := loadConfig(configDir)
	if err != nil {
		return nil, wrapError("config_error", "failed to load config", exitFilesystem, err)
	}
	profile, err := activeProfile(cfg)
	if err != nil {
		return nil, err
	}
	if err := maybeRefreshProfile(profile, cfg, path); err != nil {
		return nil, err
	}
	client := newClient(profile)
	return &session{cfg: cfg, cfgPath: path, profile: profile, client: client}, nil
}

func newClient(profile *Profile) *aliyunpan_open.OpenPanClient {
	return aliyunpan_open.NewOpenPanClient(openapi.ApiConfig{
		UserId:       profile.UserID,
		ClientId:     profile.ClientID,
		ClientSecret: profile.ClientSecret,
	}, apiToken(profile), nil)
}

func enrichProfile(profile *Profile) error {
	client := newClient(profile)
	ui, err := client.GetUserInfo()
	if err != nil {
		return apiErr("failed to fetch user info", err)
	}
	profile.UserID = ui.UserId
	profile.Nickname = ui.Nickname
	profile.AccountName = ui.UserName
	profile.Drives = []DriveInfo{
		{DriveID: ui.FileDriveId, DriveTag: "File", DriveName: "备份盘"},
	}
	if ui.ResourceDriveId != "" {
		profile.Drives = append(profile.Drives, DriveInfo{DriveID: ui.ResourceDriveId, DriveTag: "Resource", DriveName: "资源库"})
	}
	if profile.ActiveDriveID == "" {
		profile.ActiveDriveID = ui.FileDriveId
	}
	return nil
}

func apiErr(message string, err *apierror.ApiError) error {
	if err == nil {
		return nil
	}
	return &CLIError{
		Code:    "api_error",
		Message: message + ": " + err.String(),
		Status:  exitAPI,
	}
}

func activeDriveID(profile *Profile, ref string) (string, error) {
	drive := profile.driveByRef(ref)
	if drive == nil {
		if ref == "" {
			return "", authError("profile has no active drive; run drive use")
		}
		return "", usageError("drive %q not found; use drive list", ref)
	}
	return drive.DriveID, nil
}

func normalizePanPath(p string) string {
	if p == "" {
		return "/"
	}
	p = stringsReplaceBackslash(p)
	if p[0] != '/' {
		p = "/" + p
	}
	return cleanPanPath(p)
}

func stringsReplaceBackslash(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] == '\\' {
			b[i] = '/'
		}
	}
	return string(b)
}

func cleanPanPath(p string) string {
	out := make([]byte, 0, len(p))
	lastSlash := false
	for i := 0; i < len(p); i++ {
		if p[i] == '/' {
			if !lastSlash {
				out = append(out, '/')
			}
			lastSlash = true
			continue
		}
		out = append(out, p[i])
		lastSlash = false
	}
	if len(out) > 1 && out[len(out)-1] == '/' {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return "/"
	}
	return string(out)
}

func fileOutput(f *aliyunpan.FileEntity) map[string]any {
	if f == nil {
		return map[string]any{}
	}
	return map[string]any{
		"driveId":         f.DriveId,
		"fileId":          f.FileId,
		"name":            f.FileName,
		"path":            f.Path,
		"type":            f.FileType,
		"size":            f.FileSize,
		"createdAt":       f.CreatedAt,
		"updatedAt":       f.UpdatedAt,
		"parentFileId":    f.ParentFileId,
		"contentHash":     f.ContentHash,
		"contentHashName": f.ContentHashName,
	}
}
