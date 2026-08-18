package modelruntime

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Paths struct {
	Root        string
	Model       string
	ModelBlob   string
	ModelRef    string
	Runtime     string
	State       string
	Log         string
	Lock        string
	InstallLock string
}

func DefaultPaths() (Paths, error) {
	root, err := dataRoot()
	if err != nil {
		return Paths{}, err
	}
	paths := PathsForRoot(root)
	if os.Getenv("PROSECHECK_HOME") == "" {
		hubRoot, hubErr := huggingFaceHubRoot()
		if hubErr != nil {
			return Paths{}, hubErr
		}
		paths.Model, paths.ModelBlob, paths.ModelRef = huggingFaceModelPaths(hubRoot)
	}
	return paths, nil
}

func PathsForRoot(root string) Paths {
	model, blob, ref := huggingFaceModelPaths(filepath.Join(root, "huggingface", "hub"))
	return Paths{
		Root:        root,
		Model:       model,
		ModelBlob:   blob,
		ModelRef:    ref,
		Runtime:     filepath.Join(root, "runtimes", RuntimeVersion),
		State:       filepath.Join(root, "state.json"),
		Log:         filepath.Join(root, "server.log"),
		Lock:        filepath.Join(root, "start.lock"),
		InstallLock: filepath.Join(root, "install.lock"),
	}
}

func huggingFaceModelPaths(hubRoot string) (string, string, string) {
	repository := "models--" + strings.ReplaceAll(ModelRepository, "/", "--")
	modelRoot := filepath.Join(hubRoot, repository)
	return filepath.Join(modelRoot, "snapshots", ModelRevision, ModelFileName),
		filepath.Join(modelRoot, "blobs", ModelSHA256),
		filepath.Join(modelRoot, "refs", "main")
}

func huggingFaceHubRoot() (string, error) {
	if root := os.Getenv("HF_HUB_CACHE"); root != "" {
		return filepath.Abs(root)
	}
	home := os.Getenv("HF_HOME")
	if home == "" {
		cacheRoot := os.Getenv("XDG_CACHE_HOME")
		if cacheRoot == "" {
			userHome, err := os.UserHomeDir()
			if err != nil || userHome == "" {
				return "", errors.New("cannot find the Hugging Face cache directory")
			}
			cacheRoot = filepath.Join(userHome, ".cache")
		}
		home = filepath.Join(cacheRoot, "huggingface")
	}
	return filepath.Abs(filepath.Join(home, "hub"))
}

func dataRoot() (string, error) {
	if root := os.Getenv("PROSECHECK_HOME"); root != "" {
		return filepath.Abs(root)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", errors.New("cannot find the user data directory")
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "prosecheck"), nil
	case "windows":
		config, configErr := os.UserConfigDir()
		if configErr != nil {
			return "", configErr
		}
		return filepath.Join(config, "prosecheck"), nil
	default:
		if data := os.Getenv("XDG_DATA_HOME"); data != "" {
			return filepath.Join(data, "prosecheck"), nil
		}
		return filepath.Join(home, ".local", "share", "prosecheck"), nil
	}
}
