package netshare

import "testing"

func TestRoot(t *testing.T) {
	for in, want := range map[string]string{
		`\\nas\media`:             `\\nas\media`,
		`\\nas\media\Movies\2024`: `\\nas\media`,
		` //nas/media/TV `:        `\\nas\media`,
		`\\192.168.1.5\share$\x`:  `\\192.168.1.5\share$`,
	} {
		if got, err := Root(in); err != nil || got != want {
			t.Errorf("Root(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{`D:\Media`, `\\nas`, `\\nas\`, `\\\media`, ``, `/mnt/media`} {
		if _, err := Root(bad); err == nil {
			t.Errorf("Root(%q) accepted", bad)
		}
	}
}
