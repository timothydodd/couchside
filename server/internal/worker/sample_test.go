package worker

import "testing"

func TestIsSample(t *testing.T) {
	for path, want := range map[string]bool{
		"Movie (2010)/sample.mkv":                true,
		"Movie (2010)/movie-sample.mkv":          true,
		"Movie (2010)/movie.sample.mkv":          true,
		"Movie (2010)/Sample/movie.mkv":          true,
		"Show/Season 2/S02E05 - Free Sample.mkv": false,
		"Show/Season 2/Sample Size S02E01.mkv":   false,
		"Movie (2010)/movie.mkv":                 false,
	} {
		if got := isSample(path); got != want {
			t.Errorf("isSample(%s) = %v, want %v", path, got, want)
		}
	}
}
