package app

import (
	"fmt"
	"path"

	"github.com/tickstep/aliyunpan-api/aliyunpan"
)

func (a *App) runWhoami(args []string, opts OutputOptions) error {
	fs := newFlagSet("whoami", a.errOut, &opts)
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	sess, err := loadSession(opts.ConfigDir)
	if err != nil {
		return err
	}
	ui, apiE := sess.client.GetUserInfo()
	if apiE != nil {
		return apiErr("failed to fetch user info", apiE)
	}
	out := map[string]any{
		"profile":         sess.profile.Name,
		"userId":          ui.UserId,
		"nickname":        ui.Nickname,
		"fileDriveId":     ui.FileDriveId,
		"resourceDriveId": ui.ResourceDriveId,
		"totalSize":       ui.TotalSize,
		"usedSize":        ui.UsedSize,
		"thirdPartyVip":   ui.ThirdPartyVip,
	}
	return writeOutput(a.out, opts.Format, out)
}

func (a *App) runDrive(args []string, opts OutputOptions) error {
	if len(args) == 0 {
		return usageError("drive requires a subcommand: list or use")
	}
	switch args[0] {
	case "list":
		return a.runDriveList(args[1:], opts)
	case "use":
		return a.runDriveUse(args[1:], opts)
	default:
		return usageError("unknown drive subcommand %q", args[0])
	}
}

func (a *App) runDriveList(args []string, opts OutputOptions) error {
	fs := newFlagSet("drive list", a.errOut, &opts)
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	sess, err := loadSession(opts.ConfigDir)
	if err != nil {
		return err
	}
	rows := make([]map[string]any, 0, len(sess.profile.Drives))
	for _, d := range sess.profile.Drives {
		rows = append(rows, map[string]any{
			"driveId": d.DriveID,
			"name":    d.DriveName,
			"tag":     d.DriveTag,
			"active":  d.DriveID == sess.profile.ActiveDriveID,
		})
	}
	return writeOutput(a.out, opts.Format, rows)
}

func (a *App) runDriveUse(args []string, opts OutputOptions) error {
	fs := newFlagSet("drive use", a.errOut, &opts)
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageError("drive use requires exactly one drive tag, name, or id")
	}
	cfg, cfgPath, err := loadConfig(opts.ConfigDir)
	if err != nil {
		return wrapError("config_error", "failed to load config", exitFilesystem, err)
	}
	profile, err := activeProfile(cfg)
	if err != nil {
		return err
	}
	drive := profile.driveByRef(fs.Arg(0))
	if drive == nil {
		return usageError("drive %q not found; run drive list", fs.Arg(0))
	}
	profile.ActiveDriveID = drive.DriveID
	if err := saveConfig(cfgPath, cfg); err != nil {
		return wrapError("config_error", "failed to save config", exitFilesystem, err)
	}
	return writeOutput(a.out, opts.Format, map[string]any{
		"activeDriveId": drive.DriveID,
		"name":          drive.DriveName,
		"tag":           drive.DriveTag,
	})
}

func (a *App) runLS(args []string, opts OutputOptions) error {
	fs := newFlagSet("ls", a.errOut, &opts)
	driveRef := fs.String("drive", "", "drive tag, name, or id")
	limit := fs.Int("limit", 200, "page size for listing")
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	target := "/"
	if fs.NArg() > 0 {
		target = fs.Arg(0)
	}
	sess, err := loadSession(opts.ConfigDir)
	if err != nil {
		return err
	}
	driveID, err := activeDriveID(sess.profile, *driveRef)
	if err != nil {
		return err
	}
	target = normalizePanPath(target)
	info, apiE := sess.client.FileInfoByPath(driveID, target)
	if apiE != nil {
		return apiErr("failed to resolve path", apiE)
	}
	if info.IsFile() {
		return writeOutput(a.out, opts.Format, []map[string]any{fileOutput(info)})
	}
	if *limit <= 0 {
		*limit = 200
	}
	files, apiE := sess.client.FileListGetAll(&aliyunpan.FileListParam{
		DriveId:        driveID,
		ParentFileId:   info.FileId,
		Limit:          *limit,
		OrderBy:        aliyunpan.FileOrderByName,
		OrderDirection: aliyunpan.FileOrderDirectionAsc,
	}, 100)
	if apiE != nil {
		return apiErr("failed to list path", apiE)
	}
	rows := make([]map[string]any, 0, len(files))
	for _, f := range files {
		rows = append(rows, fileOutput(f))
	}
	return writeOutput(a.out, opts.Format, rows)
}

func (a *App) runStat(args []string, opts OutputOptions) error {
	fs := newFlagSet("stat", a.errOut, &opts)
	driveRef := fs.String("drive", "", "drive tag, name, or id")
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageError("stat requires exactly one path")
	}
	sess, err := loadSession(opts.ConfigDir)
	if err != nil {
		return err
	}
	driveID, err := activeDriveID(sess.profile, *driveRef)
	if err != nil {
		return err
	}
	info, apiE := sess.client.FileInfoByPath(driveID, normalizePanPath(fs.Arg(0)))
	if apiE != nil {
		return apiErr("failed to stat path", apiE)
	}
	return writeOutput(a.out, opts.Format, fileOutput(info))
}

func (a *App) runMkdir(args []string, opts OutputOptions) error {
	fs := newFlagSet("mkdir", a.errOut, &opts)
	driveRef := fs.String("drive", "", "drive tag, name, or id")
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageError("mkdir requires exactly one path")
	}
	sess, err := loadSession(opts.ConfigDir)
	if err != nil {
		return err
	}
	driveID, err := activeDriveID(sess.profile, *driveRef)
	if err != nil {
		return err
	}
	result, apiE := sess.client.MkdirByFullPath(driveID, normalizePanPath(fs.Arg(0)))
	if apiE != nil {
		return apiErr("failed to create directory", apiE)
	}
	return writeOutput(a.out, opts.Format, map[string]any{
		"driveId":      result.DriveId,
		"fileId":       result.FileId,
		"name":         result.FileName,
		"parentFileId": result.ParentFileId,
		"type":         result.Type,
	})
}

func (a *App) runRM(args []string, opts OutputOptions) error {
	fs := newFlagSet("rm", a.errOut, &opts)
	driveRef := fs.String("drive", "", "drive tag, name, or id")
	permanent := fs.Bool("permanent", false, "delete permanently instead of moving to trash")
	yes := fs.Bool("yes", false, "confirm permanent deletion")
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return usageError("rm requires at least one path")
	}
	if *permanent && !*yes {
		return usageError("rm --permanent requires --yes")
	}
	sess, err := loadSession(opts.ConfigDir)
	if err != nil {
		return err
	}
	driveID, err := activeDriveID(sess.profile, *driveRef)
	if err != nil {
		return err
	}
	results := make([]map[string]any, 0, fs.NArg())
	for _, raw := range fs.Args() {
		panPath := normalizePanPath(raw)
		info, apiE := sess.client.FileInfoByPath(driveID, panPath)
		if apiE != nil {
			return apiErr(fmt.Sprintf("failed to resolve %s", panPath), apiE)
		}
		param := &aliyunpan.FileBatchActionParam{DriveId: driveID, FileId: info.FileId}
		var actionErr error
		if *permanent {
			_, apiE = sess.client.FileDeleteCompletely(param)
		} else {
			_, apiE = sess.client.FileDelete(param)
		}
		if apiE != nil {
			actionErr = apiErr(fmt.Sprintf("failed to delete %s", panPath), apiE)
		}
		if actionErr != nil {
			return actionErr
		}
		results = append(results, map[string]any{
			"path":      panPath,
			"fileId":    info.FileId,
			"name":      path.Base(panPath),
			"permanent": *permanent,
			"deleted":   true,
		})
	}
	return writeOutput(a.out, opts.Format, results)
}
