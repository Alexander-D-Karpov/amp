package platform

import (
	"os"
	"path/filepath"
	"runtime"
)

const (
	osWindows = "windows"
	osDarwin  = "darwin"
	osAndroid = "android"
)

// GetDataDir returns the platform-specific data directory for AMP
func GetDataDir() (string, error) {
	switch runtime.GOOS {
	case osWindows:
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "AMP"), nil
		}
		return filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming", "AMP"), nil
	case osDarwin:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", "AMP"), nil
	case osAndroid:
		if androidData := os.Getenv("ANDROID_DATA"); androidData != "" {
			return filepath.Join(androidData, "data", "ru.akarpov.amp", "files"), nil
		}
		return "/data/data/ru.akarpov.amp/files", nil
	default:
		if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
			return filepath.Join(xdgData, "amp"), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "share", "amp"), nil
	}
}

// GetCacheDir returns the platform-specific cache directory for AMP
func GetCacheDir() (string, error) {
	switch runtime.GOOS {
	case osWindows:
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			return filepath.Join(localAppData, "AMP", "Cache"), nil
		}
		return filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local", "AMP", "Cache"), nil
	case osDarwin:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Caches", "AMP"), nil
	case osAndroid:
		if androidData := os.Getenv("ANDROID_DATA"); androidData != "" {
			return filepath.Join(androidData, "data", "ru.akarpov.amp", "cache"), nil
		}
		return "/data/data/ru.akarpov.amp/cache", nil
	default:
		if xdgCache := os.Getenv("XDG_CACHE_HOME"); xdgCache != "" {
			return filepath.Join(xdgCache, "amp"), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".cache", "amp"), nil
	}
}

// GetConfigDir returns the platform-specific configuration directory for AMP
func GetConfigDir() (string, error) {
	switch runtime.GOOS {
	case osWindows:
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "AMP"), nil
		}
		return filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming", "AMP"), nil
	case osDarwin:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Preferences", "AMP"), nil
	case osAndroid:
		if androidData := os.Getenv("ANDROID_DATA"); androidData != "" {
			return filepath.Join(androidData, "data", "ru.akarpov.amp", "files"), nil
		}
		return "/data/data/ru.akarpov.amp/files", nil
	default:
		if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
			return filepath.Join(xdgConfig, "amp"), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".config", "amp"), nil
	}
}
