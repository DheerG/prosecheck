package modelruntime

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type InstallProgress struct {
	Message        string
	Downloaded     int64
	Total          int64
	BytesPerSecond float64
	Transfer       bool
	Done           bool
}

type InstallReporter func(InstallProgress)

func (m *Manager) Install(ctx context.Context, modelFile string, report InstallReporter) error {
	if report == nil {
		report = func(InstallProgress) {}
	}
	if err := os.MkdirAll(m.paths.Root, 0o700); err != nil {
		return err
	}
	release, err := acquireLock(ctx, m.paths.InstallLock, 10*time.Minute)
	if err != nil {
		return fmt.Errorf("another model installation is in progress: %w", err)
	}
	defer release()

	asset, err := currentAsset()
	if err != nil {
		return err
	}
	server, serverErr := findServer(m.paths.Runtime, asset.ServerExe)
	if serverErr != nil {
		if err := m.installRuntime(ctx, asset, report); err != nil {
			return err
		}
		server, serverErr = findServer(m.paths.Runtime, asset.ServerExe)
	}
	if serverErr != nil {
		return fmt.Errorf("the local model runtime does not contain %s", asset.ServerExe)
	}
	_ = server

	if validFile(m.paths.Model, ModelSHA256) {
		if err := m.writeModelRef(); err != nil {
			return fmt.Errorf("cannot update the Hugging Face cache: %w", err)
		}
		report(InstallProgress{Message: "The Ministral model is already installed."})
		return nil
	}
	if validFile(m.paths.ModelBlob, ModelSHA256) {
		if err := m.linkModelSnapshot(); err != nil {
			return fmt.Errorf("cannot add the model to the Hugging Face cache: %w", err)
		}
		report(InstallProgress{Message: "The Ministral model is already installed."})
		return nil
	}
	if modelFile != "" {
		report(InstallProgress{Message: "Importing the Ministral model..."})
		if err := copyVerified(modelFile, m.paths.ModelBlob, ModelSHA256); err != nil {
			return fmt.Errorf("cannot import the Ministral model: %w", err)
		}
	} else {
		if err := m.downloadVerified(ctx, modelDownloadURL, m.paths.ModelBlob, ModelSHA256,
			"Downloading the Ministral model", report); err != nil {
			return fmt.Errorf("cannot download the Ministral model: %w", err)
		}
	}
	if err := m.linkModelSnapshot(); err != nil {
		return fmt.Errorf("cannot add the model to the Hugging Face cache: %w", err)
	}
	report(InstallProgress{Message: "Installed Ministral 3 8B and the local model runtime."})
	report(InstallProgress{Message: "Shared model file: " + m.paths.Model})
	return nil
}

func (m *Manager) linkModelSnapshot() error {
	if validFile(m.paths.Model, ModelSHA256) {
		return m.writeModelRef()
	}
	if err := os.MkdirAll(filepath.Dir(m.paths.Model), 0o700); err != nil {
		return err
	}
	if err := os.Remove(m.paths.Model); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	relativeBlob, err := filepath.Rel(filepath.Dir(m.paths.Model), m.paths.ModelBlob)
	if err != nil {
		return err
	}
	if err := os.Symlink(relativeBlob, m.paths.Model); err != nil {
		if linkErr := os.Link(m.paths.ModelBlob, m.paths.Model); linkErr != nil {
			return errors.Join(err, linkErr)
		}
	}
	return m.writeModelRef()
}

func (m *Manager) writeModelRef() error {
	if err := os.MkdirAll(filepath.Dir(m.paths.ModelRef), 0o700); err != nil {
		return err
	}
	return os.WriteFile(m.paths.ModelRef, []byte(ModelRevision), 0o600)
}

func (m *Manager) installRuntime(ctx context.Context, asset runtimeAsset, report InstallReporter) error {
	archivePath := filepath.Join(m.paths.Root, "downloads", "prism-runtime."+strings.ReplaceAll(asset.Archive, ".", "-"))
	if err := m.downloadVerified(ctx, asset.URL, archivePath, asset.SHA256,
		"Downloading the local model runtime", report); err != nil {
		return fmt.Errorf("cannot download the local model runtime: %w", err)
	}
	stage := m.paths.Runtime + ".partial"
	if err := os.RemoveAll(stage); err != nil {
		return err
	}
	if err := os.MkdirAll(stage, 0o700); err != nil {
		return err
	}
	var err error
	switch asset.Archive {
	case "tar.gz":
		err = extractTarGz(archivePath, stage)
	case "zip":
		err = extractZip(archivePath, stage)
	default:
		err = fmt.Errorf("unknown archive type %q", asset.Archive)
	}
	if err != nil {
		_ = os.RemoveAll(stage)
		return fmt.Errorf("cannot unpack the local model runtime: %w", err)
	}
	if err := os.RemoveAll(m.paths.Runtime); err != nil {
		return err
	}
	if err := os.Rename(stage, m.paths.Runtime); err != nil {
		return err
	}
	return nil
}

func (m *Manager) downloadVerified(
	ctx context.Context,
	sourceURL, destination, checksum, message string,
	report InstallReporter,
) error {
	if validFile(destination, checksum) {
		return nil
	}
	partial := destination + ".partial"
	offset := fileSize(partial)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return err
	}
	if offset > 0 {
		request.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	response, err := m.downloadHTTP.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	appendDownload := false
	total := response.ContentLength
	if offset > 0 && response.StatusCode == http.StatusPartialContent {
		start, completeSize, ok := parseContentRange(response.Header.Get("Content-Range"))
		if !ok || start != offset {
			return errors.New("the download server returned an invalid content range")
		}
		appendDownload = true
		total = completeSize
	} else if offset > 0 && response.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		if !validFile(partial, checksum) {
			return errors.New("the download server rejected the saved partial file")
		}
		if report != nil {
			report(InstallProgress{
				Message: message, Downloaded: offset, Total: offset, Transfer: true, Done: true,
			})
		}
		return os.Rename(partial, destination)
	} else if response.StatusCode == http.StatusOK {
		offset = 0
	} else if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("the download server returned HTTP %d", response.StatusCode)
	} else {
		return fmt.Errorf("the download server did not resume at byte %d", offset)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	flags := os.O_CREATE | os.O_TRUNC | os.O_WRONLY
	if appendDownload {
		flags = os.O_CREATE | os.O_APPEND | os.O_WRONLY
	}
	file, err := os.OpenFile(partial, flags, 0o600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	if appendDownload {
		existing, openErr := os.Open(partial)
		if openErr != nil {
			_ = file.Close()
			return openErr
		}
		_, hashErr := io.Copy(hash, existing)
		closeExistingErr := existing.Close()
		if hashErr != nil || closeExistingErr != nil {
			_ = file.Close()
			return errors.Join(hashErr, closeExistingErr)
		}
	}
	reader := newTransferReader(response.Body, offset, total, message, report)
	_, copyErr := io.Copy(io.MultiWriter(file, hash), reader)
	reader.finish()
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != checksum {
		_ = os.Remove(partial)
		return fmt.Errorf("checksum mismatch: expected %s, got %s", checksum, actual)
	}
	return os.Rename(partial, destination)
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return 0
	}
	return info.Size()
}

func parseContentRange(value string) (int64, int64, bool) {
	var start, end, total int64
	count, err := fmt.Sscanf(value, "bytes %d-%d/%d", &start, &end, &total)
	return start, total, err == nil && count == 3 && start >= 0 && end >= start && total > end
}

type transferReader struct {
	reader         io.Reader
	total          int64
	message        string
	report         InstallReporter
	downloaded     int64
	lastBytes      int64
	lastReport     time.Time
	lastReportDone bool
}

func newTransferReader(reader io.Reader, downloaded, total int64, message string, report InstallReporter) *transferReader {
	now := time.Now()
	transfer := &transferReader{
		reader: reader, total: total, message: message, report: report,
		downloaded: downloaded, lastBytes: downloaded, lastReport: now,
	}
	if report != nil {
		report(InstallProgress{Message: message, Downloaded: downloaded, Total: total, Transfer: true})
	}
	return transfer
}

func (reader *transferReader) Read(buffer []byte) (int, error) {
	count, err := reader.reader.Read(buffer)
	reader.downloaded += int64(count)
	now := time.Now()
	done := errors.Is(err, io.EOF) || (reader.total > 0 && reader.downloaded >= reader.total)
	if done || now.Sub(reader.lastReport) >= 500*time.Millisecond {
		reader.send(now, done)
	}
	return count, err
}

func (reader *transferReader) finish() {
	if !reader.lastReportDone {
		reader.send(time.Now(), true)
	}
}

func (reader *transferReader) send(now time.Time, done bool) {
	if reader.report == nil || reader.lastReportDone {
		return
	}
	elapsed := now.Sub(reader.lastReport).Seconds()
	speed := float64(0)
	if elapsed > 0 {
		speed = float64(reader.downloaded-reader.lastBytes) / elapsed
	}
	reader.report(InstallProgress{
		Message: reader.message, Downloaded: reader.downloaded, Total: reader.total,
		BytesPerSecond: speed, Transfer: true, Done: done,
	})
	reader.lastBytes = reader.downloaded
	reader.lastReport = now
	reader.lastReportDone = done
}

func copyVerified(source, destination, checksum string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	partial := destination + ".partial"
	output, err := os.OpenFile(partial, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(output, hash), input)
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(partial)
		return errors.Join(copyErr, closeErr)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != checksum {
		_ = os.Remove(partial)
		return fmt.Errorf("checksum mismatch: expected %s, got %s", checksum, actual)
	}
	return os.Rename(partial, destination)
}

func validFile(path, expected string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false
	}
	return hex.EncodeToString(hash.Sum(nil)) == expected
}

func extractTarGz(archivePath, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		target, err := safeArchivePath(destination, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)&0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeArchiveFile(target, reader, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := createSafeSymlink(destination, target, header.Linkname); err != nil {
				return err
			}
		}
	}
}

func extractZip(archivePath, destination string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, entry := range reader.File {
		target, err := safeArchivePath(destination, entry.Name)
		if err != nil {
			return err
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		input, err := entry.Open()
		if err != nil {
			return err
		}
		err = writeArchiveFile(target, input, entry.Mode())
		closeErr := input.Close()
		if err != nil || closeErr != nil {
			return errors.Join(err, closeErr)
		}
	}
	return nil
}

func safeArchivePath(root, name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." {
		return root, nil
	}
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	target := filepath.Join(root, clean)
	if target != root && !strings.HasPrefix(target, root+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	return target, nil
}

func createSafeSymlink(root, target, link string) error {
	if filepath.IsAbs(link) {
		return fmt.Errorf("unsafe symbolic link %q", link)
	}
	resolved := filepath.Clean(filepath.Join(filepath.Dir(target), filepath.FromSlash(link)))
	if resolved != root && !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
		return fmt.Errorf("unsafe symbolic link %q", link)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.Symlink(filepath.FromSlash(link), target)
}

func writeArchiveFile(target string, source io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, source)
	return errors.Join(copyErr, file.Close())
}

func findServer(root, name string) (string, error) {
	var found string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Name() == name {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", os.ErrNotExist
	}
	return found, nil
}
