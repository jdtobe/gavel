package runtime

import (
	"os"
	"path/filepath"
)

// Paths holds every app data path Gavel uses.
type Paths struct {
	Home             string
	ConfigFile       string
	RuntimeDir       string
	CacheDir         string
	LogsDir          string
	HFHome           string
	UVCacheDir       string
	PythonDir        string
	UVDir            string
	LockFile         string
	SidecarStateFile string
}

// ResolvePaths returns the default app paths, honoring GAVEL_HOME.
func ResolvePaths() (Paths, error) {
	if home := os.Getenv("GAVEL_HOME"); home != "" {
		return ResolvePathsAt(home)
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, err
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return Paths{}, err
	}
	return buildPaths(filepath.Join(configDir, "gavel"), filepath.Join(cacheDir, "gavel")), nil
}

// ResolvePathsAt treats home as the app home, with cache beneath it.
func ResolvePathsAt(home string) (Paths, error) {
	abs, err := filepath.Abs(home)
	if err != nil {
		return Paths{}, err
	}
	return buildPaths(abs, filepath.Join(abs, "cache")), nil
}

func buildPaths(home, cacheDir string) Paths {
	runtimeDir := filepath.Join(home, "runtime")
	return Paths{
		Home:             home,
		ConfigFile:       filepath.Join(home, "config.json"),
		RuntimeDir:       runtimeDir,
		CacheDir:         cacheDir,
		LogsDir:          filepath.Join(home, "logs"),
		HFHome:           filepath.Join(cacheDir, "huggingface"),
		UVCacheDir:       filepath.Join(cacheDir, "uv"),
		PythonDir:        filepath.Join(runtimeDir, "python"),
		UVDir:            filepath.Join(runtimeDir, "uv"),
		LockFile:         filepath.Join(runtimeDir, "gavel.lock"),
		SidecarStateFile: filepath.Join(runtimeDir, "sidecar.json"),
	}
}
