package modelruntime

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

type Paths struct {
	Root        string
	Model       string
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
	return PathsForRoot(root), nil
}

func PathsForRoot(root string) Paths {
	return Paths{
		Root:        root,
		Model:       filepath.Join(root, "models", ModelName, ModelFileName),
		Runtime:     filepath.Join(root, "runtimes", RuntimeVersion),
		State:       filepath.Join(root, "state.json"),
		Log:         filepath.Join(root, "server.log"),
		Lock:        filepath.Join(root, "start.lock"),
		InstallLock: filepath.Join(root, "install.lock"),
	}
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
