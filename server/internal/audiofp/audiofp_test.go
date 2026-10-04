package audiofp

import (
	"math"
	"math/rand"
	"testing"
)

// tune is a made-up theme: a melody of changing notes with a little
// harmony, so its spectrum moves the way music's does.
func tune(sec float64, seed int64) []int16 {
	r := rand.New(rand.NewSource(seed))
	out := make([]int16, int(sec*Rate))
	note, next := 440.0, 0
	for i := range out {
		if i >= next {
			note = 220 * math.Pow(2, float64(r.Intn(24))/12)
			next = i + Rate/8 + r.Intn(Rate/4)
		}
		t := float64(i) / Rate
		v := math.Sin(2*math.Pi*note*t) + 0.5*math.Sin(2*math.Pi*note*1.5*t) + 0.3*math.Sin(2*math.Pi*note*2*t)
		out[i] = int16(v * 5000)
	}
	return out
}

// noise stands in for an episode's own dialogue and sound: different in
// every episode.
func noise(sec float64, seed int64) []int16 {
	r := rand.New(rand.NewSource(seed))
	out := make([]int16, int(sec*Rate))
	lp := 0.0
	for i := range out {
		lp = 0.9*lp + 0.1*(r.Float64()*2-1)
		out[i] = int16(lp * 20000)
	}
	return out
}

func mix(dst []int16, at float64, src []int16, gain float64) {
	base := int(at * Rate)
	for i, v := range src {
		if base+i < len(dst) {
			dst[base+i] = int16(float64(dst[base+i]) + float64(v)*gain)
		}
	}
}

// Two episodes with their own sound and the same 35-second theme, starting
// at 20 s in one and 65 s in the other.
func TestCommonFindsASharedTheme(t *testing.T) {
	theme := tune(35, 1)
	a, b := noise(150, 2), noise(150, 3)
	// The theme replaces the episode's sound, with a little of it bleeding through.
	put := func(ep []int16, at float64) {
		base := int(at * Rate)
		for i := range theme {
			ep[base+i] = int16(float64(ep[base+i]) * 0.1)
		}
		mix(ep, at, theme, 1)
	}
	put(a, 20)
	put(b, 65)
	s, ok := Common(Fingerprint(a), Fingerprint(b), 15)
	if !ok {
		t.Fatal("no shared stretch found")
	}
	if math.Abs(s.StartA-20) > 2 || math.Abs(s.StartB-65) > 2 || math.Abs(s.Length-35) > 4 {
		t.Fatalf("shared = %+v, want about 35s at 20s and 65s", s)
	}
}

// Episodes with nothing in common, or only a few seconds, share nothing.
func TestCommonFindsNothingInUnrelatedAudio(t *testing.T) {
	a, b := noise(150, 4), noise(150, 5)
	if s, ok := Common(Fingerprint(a), Fingerprint(b), 15); ok {
		t.Fatalf("unrelated audio shares %+v", s)
	}
	sting := tune(4, 9) // a short sound both use
	mix(a, 30, sting, 1)
	mix(b, 100, sting, 1)
	if s, ok := Common(Fingerprint(a), Fingerprint(b), 15); ok {
		t.Fatalf("a 4-second sting counted as a theme: %+v", s)
	}
	if _, ok := Common(nil, Fingerprint(b), 15); ok {
		t.Fatal("an empty fingerprint matched")
	}
}

// The same theme at different loudness, with the other episode's sound
// louder underneath: still found.
func TestCommonToleratesLevelAndNoise(t *testing.T) {
	theme := tune(40, 7)
	a, b := noise(150, 8), noise(150, 10)
	for i := range a {
		a[i] /= 4
		b[i] /= 4
	}
	mix(a, 50, theme, 1)
	mix(b, 10, theme, 0.5)
	s, ok := Common(Fingerprint(a), Fingerprint(b), 15)
	if !ok || math.Abs(s.StartA-50) > 3 || math.Abs(s.StartB-10) > 3 || s.Length < 30 {
		t.Fatalf("shared = %+v ok=%v", s, ok)
	}
}

func TestFFT(t *testing.T) {
	n := 64
	re, im := make([]float64, n), make([]float64, n)
	for i := range re {
		re[i] = math.Cos(2 * math.Pi * 5 * float64(i) / float64(n))
	}
	fft(re, im)
	for k := 0; k < n/2; k++ {
		mag := math.Hypot(re[k], im[k])
		if k == 5 && math.Abs(mag-float64(n)/2) > 1e-6 {
			t.Fatalf("bin 5 = %v, want %v", mag, n/2)
		}
		if k != 5 && mag > 1e-6 {
			t.Fatalf("bin %d = %v, want 0", k, mag)
		}
	}
}
