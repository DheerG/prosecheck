package modelruntime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
)

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
