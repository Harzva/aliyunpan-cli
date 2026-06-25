package app

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tickstep/aliyunpan-api/aliyunpan"
	"github.com/tickstep/aliyunpan-api/aliyunpan/apierror"
	"github.com/tickstep/library-go/requester/rio"
)

func (a *App) runUpload(args []string, opts OutputOptions) error {
	fs := newFlagSet("upload", a.errOut, &opts)
	driveRef := fs.String("drive", "", "drive tag, name, or id")
	blockMB := fs.Int64("block-mb", 10, "upload part size in MiB")
	overwrite := fs.Bool("overwrite", false, "overwrite same-name remote file")
	noRapid := fs.Bool("no-rapid", false, "skip rapid-upload hash/proof fields")
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return usageError("upload requires <local-file> <remote-dir>")
	}
	sess, err := loadSession(opts.ConfigDir)
	if err != nil {
		return err
	}
	driveID, err := activeDriveID(sess.profile, *driveRef)
	if err != nil {
		return err
	}
	result, err := a.uploadFile(sess, driveID, fs.Arg(0), normalizePanPath(fs.Arg(1)), *blockMB*1024*1024, *overwrite, *noRapid, shouldShowProgress(opts))
	if err != nil {
		return err
	}
	return writeOutput(a.out, opts.Format, result)
}

func (a *App) runDownload(args []string, opts OutputOptions) error {
	fs := newFlagSet("download", a.errOut, &opts)
	driveRef := fs.String("drive", "", "drive tag, name, or id")
	output := fs.String("output", "", "local output file or directory")
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageError("download requires <remote-file>")
	}
	sess, err := loadSession(opts.ConfigDir)
	if err != nil {
		return err
	}
	driveID, err := activeDriveID(sess.profile, *driveRef)
	if err != nil {
		return err
	}
	result, err := a.downloadFile(sess, driveID, normalizePanPath(fs.Arg(0)), *output, shouldShowProgress(opts))
	if err != nil {
		return err
	}
	return writeOutput(a.out, opts.Format, result)
}

func (a *App) uploadFile(sess *session, driveID, localPath, remoteDir string, blockSize int64, overwrite, noRapid, showProgress bool) (map[string]any, error) {
	info, err := os.Stat(localPath)
	if err != nil {
		return nil, wrapError("filesystem_error", "failed to stat local file", exitFilesystem, err)
	}
	if info.IsDir() {
		return nil, usageError("upload currently supports a single regular file, not directories")
	}
	if blockSize <= 0 {
		blockSize = 10 * 1024 * 1024
	}
	parent, apiE := sess.client.FileInfoByPath(driveID, remoteDir)
	if apiE != nil {
		if apiE.Code != apierror.ApiCodeFileNotFoundCode {
			return nil, apiErr("failed to resolve remote directory", apiE)
		}
		if parentResult, mkErr := sess.client.MkdirByFullPath(driveID, remoteDir); mkErr != nil {
			return nil, apiErr("failed to create remote directory", mkErr)
		} else {
			parent = &aliyunpan.FileEntity{
				DriveId:  driveID,
				FileId:   parentResult.FileId,
				FileName: parentResult.FileName,
				FileType: "folder",
				Path:     remoteDir,
			}
		}
	}
	if parent.IsFile() {
		return nil, usageError("remote target %s is a file; upload target must be a directory", remoteDir)
	}
	file, err := os.Open(localPath)
	if err != nil {
		return nil, wrapError("filesystem_error", "failed to open local file", exitFilesystem, err)
	}
	defer file.Close()

	hash := ""
	proof := ""
	if !noRapid {
		hash, err = sha1File(file)
		if err != nil {
			return nil, wrapError("filesystem_error", "failed to hash local file", exitFilesystem, err)
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return nil, wrapError("filesystem_error", "failed to rewind local file", exitFilesystem, err)
		}
		proof = aliyunpan.CalcProofCode(sess.profile.AccessToken, rio.NewFileReaderAtLen64(file), info.Size())
	}
	checkNameMode := "auto_rename"
	if overwrite {
		checkNameMode = "overwrite"
	}
	createResult, apiE := sess.client.CreateUploadFile(&aliyunpan.CreateFileUploadParam{
		Name:            filepath.Base(localPath),
		DriveId:         driveID,
		ParentFileId:    parent.FileId,
		Size:            info.Size(),
		ContentHash:     strings.ToUpper(hash),
		ContentHashName: "sha1",
		ProofCode:       proof,
		ProofVersion:    "v1",
		CheckNameMode:   checkNameMode,
		BlockSize:       blockSize,
	})
	if apiE != nil {
		return nil, apiErr("failed to create upload", apiE)
	}
	if !createResult.RapidUpload {
		for _, part := range createResult.PartInfoList {
			offset := int64(part.PartNumber-1) * blockSize
			length := min64(blockSize, info.Size()-offset)
			if length < 0 {
				length = 0
			}
			if err := putUploadPart(file, part.UploadURL, offset, length, info.Size(), a.errOut, showProgress); err != nil {
				return nil, err
			}
		}
		completeResult, apiE := sess.client.CompleteUploadFile(&aliyunpan.CompleteUploadFileParam{
			DriveId:  createResult.DriveId,
			FileId:   createResult.FileId,
			UploadId: createResult.UploadId,
		})
		if apiE != nil {
			return nil, apiErr("failed to complete upload", apiE)
		}
		return map[string]any{
			"uploaded":    true,
			"rapidUpload": false,
			"driveId":     completeResult.DriveId,
			"fileId":      completeResult.FileId,
			"name":        completeResult.Name,
			"size":        completeResult.Size,
			"remoteDir":   remoteDir,
		}, nil
	}
	if showProgress {
		fmt.Fprintf(a.errOut, "\ruploaded %s/%s\n", formatBytes(info.Size()), formatBytes(info.Size()))
	}
	return map[string]any{
		"uploaded":    true,
		"rapidUpload": true,
		"driveId":     createResult.DriveId,
		"fileId":      createResult.FileId,
		"name":        createResult.FileName,
		"size":        info.Size(),
		"remoteDir":   remoteDir,
	}, nil
}

func putUploadPart(file *os.File, uploadURL string, offset, length, total int64, progressOut io.Writer, showProgress bool) error {
	var body io.Reader
	if length == 0 {
		body = bytes.NewReader(nil)
	} else {
		body = io.NewSectionReader(file, offset, length)
	}
	if showProgress {
		body = &progressReader{r: body, out: progressOut, label: "upload", total: total, done: offset}
	}
	req, err := http.NewRequest("PUT", uploadURL, body)
	if err != nil {
		return wrapError("api_error", "failed to build upload request", exitAPI, err)
	}
	req.Header.Set("referer", "https://www.aliyundrive.com/")
	req.ContentLength = length
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return wrapError("api_error", "upload request failed", exitAPI, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return apiError("upload part failed: %s", resp.Status)
	}
	return nil
}

func (a *App) downloadFile(sess *session, driveID, panPath, output string, showProgress bool) (map[string]any, error) {
	info, apiE := sess.client.FileInfoByPath(driveID, panPath)
	if apiE != nil {
		return nil, apiErr("failed to resolve remote file", apiE)
	}
	if info.IsFolder() {
		return nil, usageError("download currently supports a single file, not directories")
	}
	urlResult, apiE := sess.client.GetFileDownloadUrl(&aliyunpan.GetFileDownloadUrlParam{
		DriveId:   driveID,
		FileId:    info.FileId,
		ExpireSec: 3600,
	})
	if apiE != nil {
		return nil, apiErr("failed to create download URL", apiE)
	}
	localPath, err := resolveOutputPath(output, info.FileName)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return nil, wrapError("filesystem_error", "failed to create output directory", exitFilesystem, err)
	}
	out, err := os.Create(localPath)
	if err != nil {
		return nil, wrapError("filesystem_error", "failed to create output file", exitFilesystem, err)
	}
	defer out.Close()

	req, err := http.NewRequest("GET", urlResult.Url, nil)
	if err != nil {
		return nil, wrapError("api_error", "failed to build download request", exitAPI, err)
	}
	req.Header.Set("user-agent", "Mozilla/5.0 aliyunpan-cli")
	req.Header.Set("referer", "https://www.aliyundrive.com/")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, wrapError("api_error", "download request failed", exitAPI, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, apiError("download failed: %s", resp.Status)
	}
	reader := io.Reader(resp.Body)
	if showProgress {
		reader = &progressReader{r: reader, out: a.errOut, label: "download", total: info.FileSize}
	}
	written, err := io.Copy(out, reader)
	if err != nil {
		return nil, wrapError("filesystem_error", "failed to write output file", exitFilesystem, err)
	}
	if showProgress {
		fmt.Fprintln(a.errOut)
	}
	return map[string]any{
		"downloaded": true,
		"driveId":    driveID,
		"fileId":     info.FileId,
		"name":       info.FileName,
		"remotePath": panPath,
		"localPath":  localPath,
		"size":       written,
	}, nil
}

func resolveOutputPath(output, fileName string) (string, error) {
	if output == "" {
		return filepath.Abs(fileName)
	}
	if st, err := os.Stat(output); err == nil && st.IsDir() {
		return filepath.Abs(filepath.Join(output, fileName))
	}
	return filepath.Abs(output)
}

func sha1File(file *os.File) (string, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	h := sha1.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type progressReader struct {
	r     io.Reader
	out   io.Writer
	label string
	total int64
	done  int64
	last  time.Time
}

func (p *progressReader) Read(buf []byte) (int, error) {
	n, err := p.r.Read(buf)
	if n > 0 {
		p.done += int64(n)
		if time.Since(p.last) > 200*time.Millisecond || p.done >= p.total {
			p.last = time.Now()
			fmt.Fprintf(p.out, "\r%s %s/%s", p.label, formatBytes(p.done), formatBytes(p.total))
		}
	}
	return n, err
}

func formatBytes(n int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(n)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d%s", n, units[unit])
	}
	return fmt.Sprintf("%.2f%s", value, units[unit])
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
