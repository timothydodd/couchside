package config

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// envFilePath is the settings file read before the environment:
// COUCHSIDE_CONFIG, else %ProgramData%\Couchside\couchside.env on Windows
// (where the installer writes it, since a service has no shell to set
// variables in). Elsewhere there's none unless COUCHSIDE_CONFIG names one.
func envFilePath() string {
	if p := os.Getenv("COUCHSIDE_CONFIG"); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		if pd := os.Getenv("ProgramData"); pd != "" {
			return filepath.Join(pd, "Couchside", "couchside.env")
		}
	}
	return ""
}

// fileKeys are the variables the settings file set (not the environment):
// kept across loads, since a second load finds them already set.
var (
	fileMu   sync.Mutex
	fileKeys = map[string]bool{}
)

// FromFile says the variable came from the settings file.
func FromFile(key string) bool {
	fileMu.Lock()
	defer fileMu.Unlock()
	return fileKeys[key]
}

// loadEnvFile sets each KEY=VALUE line of the settings file that isn't
// already in the environment, so a variable set by hand still wins. Blank
// lines and # comments are skipped, and a value may be wrapped in quotes.
// It returns the file it read, or "" when there was none.
func loadEnvFile() string {
	p := envFilePath()
	if p == "" {
		return ""
	}
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		k = strings.TrimSpace(strings.TrimPrefix(k, "export "))
		if !ok || k == "" {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
		if _, set := os.LookupEnv(k); !set {
			os.Setenv(k, v)
			fileMu.Lock()
			fileKeys[k] = true
			fileMu.Unlock()
		}
	}
	return p
}

// besideExe is name in the executable's own folder when it's there (the
// Windows installer puts ffmpeg next to couchside.exe), else name itself,
// for the PATH to resolve.
func besideExe(name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return strings.TrimSuffix(name, ".exe")
}
