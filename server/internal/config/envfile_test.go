package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "couchside.env")
	body := "\ufeff# Couchside settings\n\nCOUCHSIDE_MEDIA_ROOT=\"D:\\Media\"\nCOUCHSIDE_ADDR = :8096\nexport COUCHSIDE_TEST_SET=from-file\nnot a setting\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COUCHSIDE_CONFIG", p)
	t.Setenv("COUCHSIDE_TEST_SET", "from-env") // the environment wins
	t.Setenv("COUCHSIDE_MEDIA_ROOT", "")
	os.Unsetenv("COUCHSIDE_MEDIA_ROOT")
	t.Setenv("COUCHSIDE_ADDR", "")
	os.Unsetenv("COUCHSIDE_ADDR")

	if got := loadEnvFile(); got != p {
		t.Fatalf("loadEnvFile = %q", got)
	}
	for k, want := range map[string]string{"COUCHSIDE_MEDIA_ROOT": `D:\Media`, "COUCHSIDE_ADDR": ":8096", "COUCHSIDE_TEST_SET": "from-env"} {
		if got := os.Getenv(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}
