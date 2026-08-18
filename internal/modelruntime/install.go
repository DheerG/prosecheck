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

type InstallProgress func(message string)

func (m *Manager) Install(ctx context.Context, modelFile string, progress InstallProgress) error {
	if progress == nil {
		progress = func(string) {}
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
		progress("Downloading the local model runtime...")
		if err := m.installRuntime(ctx, asset); err != nil {
			return err
		}
		server, serverErr = findServer(m.paths.Runtime, asset.ServerExe)
	}
	if serverErr != nil {
		return fmt.Errorf("the local model runtime does not contain %s", asset.ServerExe)
	}
	_ = server

	if validFile(m.paths.Model, ModelSHA256) {
		progress("The Ministral model is already installed.")
		return nil
	}
	if modelFile != "" {
		progress("Importing the Ministral model...")
		if err := copyVerified(modelFile, m.paths.Model, ModelSHA256); err != nil {
			return fmt.Errorf("cannot import the Ministral model: %w", err)
		}
	} else {
		progress("Downloading the Ministral model (5.20 GB)...")
		if err := m.downloadVerified(ctx, modelDownloadURL, m.paths.Model, ModelSHA256); err != nil {
			return fmt.Errorf("cannot download the Ministral model: %w", err)
		}
	}
	progress("Installed Ministral 3 8B and the local model runtime.")
	return nil
}

func (m *Manager) installRuntime(ctx context.Context, asset runtimeAsset) error {
	archivePath := filepath.Join(m.paths.Root, "downloads", "prism-runtime."+strings.ReplaceAll(asset.Archive, ".", "-"))
	if err := m.downloadVerified(ctx, asset.URL, archivePath, asset.SHA256); err != nil {
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

func (m *Manager) downloadVerified(ctx context.Context, sourceURL, destination, checksum string) error {
	if validFile(destination, checksum) {
		return nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return err
	}
	response, err := m.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("the download server returned HTTP %d", response.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	partial := destination + ".partial"
	file, err := os.OpenFile(partial, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(file, hash), response.Body)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(partial)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(partial)
		return closeErr
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != checksum {
		_ = os.Remove(partial)
		return fmt.Errorf("checksum mismatch: expected %s, got %s", checksum, actual)
	}
	return os.Rename(partial, destination)
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
