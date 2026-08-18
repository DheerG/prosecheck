package modelruntime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

type interruptedBody struct {
	sent bool
}

func (body *interruptedBody) Read(buffer []byte) (int, error) {
	if body.sent {
		return 0, errors.New("download interrupted")
	}
	body.sent = true
	return copy(buffer, "partial data"), nil
}

func (body *interruptedBody) Close() error {
	return nil
}

func TestDefaultPathsUsesProsecheckHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PROSECHECK_HOME", root)

	paths, err := DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if paths.Root != root {
		t.Fatalf("expected root %q, got %q", root, paths.Root)
	}
	if paths.Model != filepath.Join(root, "models", ModelName, ModelFileName) {
		t.Fatalf("unexpected model path %q", paths.Model)
	}
}

func TestAvailablePortUsesFallback(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("this environment does not allow local listeners: %v", err)
	}
	defer listener.Close()
	occupied := listener.Addr().(*net.TCPAddr).Port

	port, err := availablePort(occupied)
	if err != nil {
		t.Fatal(err)
	}
	if port == occupied || port == 0 {
		t.Fatalf("expected a free fallback port, got %d", port)
	}
}

func TestTailLogsReturnsLastLines(t *testing.T) {
	paths := PathsForRoot(t.TempDir())
	if err := os.WriteFile(paths.Log, []byte("one\ntwo\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := NewWithOptions(Options{Paths: paths})

	logs, err := manager.TailLogs(2)
	if err != nil {
		t.Fatal(err)
	}
	if logs != "two\nthree" {
		t.Fatalf("unexpected log tail %q", logs)
	}
}

func TestDefaultDownloadClientHasNoOverallTimeout(t *testing.T) {
	manager := NewWithOptions(Options{Paths: PathsForRoot(t.TempDir())})
	if manager.healthHTTP.Timeout != 2*time.Second {
		t.Fatalf("unexpected health timeout: %s", manager.healthHTTP.Timeout)
	}
	if manager.downloadHTTP.Timeout != 0 {
		t.Fatalf("expected no overall download timeout, got %s", manager.downloadHTTP.Timeout)
	}
}

func TestDownloadUsesItsOwnHTTPClient(t *testing.T) {
	content := []byte("complete model")
	hash := sha256.Sum256(content)
	downloadClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(content)),
			Header:     make(http.Header),
		}, nil
	})}
	healthClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("the health client must not download files")
	})}
	manager := NewWithOptions(Options{
		Paths: PathsForRoot(t.TempDir()), HTTPClient: healthClient, DownloadClient: downloadClient,
	})
	destination := filepath.Join(t.TempDir(), "model.gguf")
	if err := manager.downloadVerified(context.Background(), "https://model.test/model.gguf", destination, hex.EncodeToString(hash[:])); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(written, content) {
		t.Fatalf("unexpected downloaded data: %q", written)
	}
}

func TestDownloadRemovesPartialFileAfterInterruption(t *testing.T) {
	downloadClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       &interruptedBody{},
			Header:     make(http.Header),
		}, nil
	})}
	manager := NewWithOptions(Options{
		Paths: PathsForRoot(t.TempDir()), DownloadClient: downloadClient,
	})
	destination := filepath.Join(t.TempDir(), "model.gguf")
	err := manager.downloadVerified(context.Background(), "https://model.test/model.gguf", destination, "unused")
	if err == nil {
		t.Fatal("expected the interrupted download to fail")
	}
	if _, statErr := os.Stat(destination + ".partial"); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected the partial file to be removed, got %v", statErr)
	}
	if _, statErr := os.Stat(destination); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected no destination file, got %v", statErr)
	}
}

func TestTarExtractionRejectsParentPath(t *testing.T) {
	directory := t.TempDir()
	archive := filepath.Join(directory, "runtime.tar.gz")
	var compressed bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressed)
	tarWriter := tar.NewWriter(gzipWriter)
	data := []byte("unsafe")
	if err := tarWriter.WriteHeader(&tar.Header{Name: "../outside", Mode: 0o600, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive, compressed.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	err := extractTarGz(archive, filepath.Join(directory, "target"))
	if err == nil {
		t.Fatal("expected an unsafe archive path to fail")
	}
}

func TestAcquireLockRemovesItOnRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	release, err := acquireLock(context.Background(), path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	release()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected lock removal, got %v", err)
	}
}
