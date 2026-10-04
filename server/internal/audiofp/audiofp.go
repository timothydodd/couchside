// Package audiofp finds the stretch of audio two recordings share: the
// opening theme that every episode of a season plays. Each recording is
// reduced to a fingerprint (a few bits per eighth of a second, describing
// how the sound's energy is spread across frequency bands and how that is
// changing), the two fingerprints are slid against each other to find the
// offset where most of them agree, and the longest run of agreement at that
// offset is the shared stretch.
//
// It's the idea behind Chromaprint, cut down to what this needs: no pitch
// folding, since both sides are the same recording of the same music.
package audiofp

import (
	"math"
	"math/bits"
)

const (
	// Rate is the sample rate fingerprints are made from (mono, 16-bit).
	Rate = 8000

	window = 4096 // samples per frame (512 ms)
	hop    = 512  // samples between frames (64 ms): two recordings are never more than half a hop out of step
	lag    = 4    // frames back that each frame is compared with (256 ms)
	bands  = 17   // energy bands per frame, giving bands-1 bits

	// FrameSec is the time between fingerprint frames.
	FrameSec = float64(hop) / Rate
)

// band edges in Hz, roughly evenly spaced on a log scale over what speech
// and music both fill.
var edges = func() []float64 {
	e := make([]float64, bands+1)
	lo, hi := 200.0, 3600.0
	for i := range e {
		e[i] = lo * math.Pow(hi/lo, float64(i)/float64(bands))
	}
	return e
}()

// Fingerprint reduces 8 kHz mono samples to one 16-bit value per frame.
// Bit b is set when band b's energy relative to band b+1 has risen since the
// frame lag frames earlier.
func Fingerprint(pcm []int16) []uint16 {
	n := (len(pcm) - window) / hop
	if n < 2 {
		return nil
	}
	hann := make([]float64, window)
	for i := range hann {
		hann[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(window-1))
	}
	// FFT bin range of each band.
	from := make([]int, bands+1)
	for i, hz := range edges {
		from[i] = int(hz * window / Rate)
	}
	re := make([]float64, window)
	im := make([]float64, window)
	hist := make([][]float64, lag+1) // the last lag+1 frames' band energies
	for i := range hist {
		hist[i] = make([]float64, bands)
	}
	out := make([]uint16, 0, n)
	for f := 0; f <= n; f++ {
		cur := hist[f%(lag+1)]
		prev := hist[(f+1)%(lag+1)] // the frame lag back, once there is one
		base := f * hop
		for i := 0; i < window; i++ {
			re[i] = float64(pcm[base+i]) * hann[i]
			im[i] = 0
		}
		fft(re, im)
		for b := 0; b < bands; b++ {
			var e float64
			for k := from[b]; k < from[b+1]; k++ {
				e += re[k]*re[k] + im[k]*im[k]
			}
			cur[b] = math.Log1p(e)
		}
		if f >= lag {
			var v uint16
			for b := 0; b < bands-1; b++ {
				if (cur[b]-cur[b+1])-(prev[b]-prev[b+1]) > 0 {
					v |= 1 << b
				}
			}
			out = append(out, v)
		}
	}
	return out
}

// fft is an in-place radix-2 transform; len(re) must be a power of two.
func fft(re, im []float64) {
	n := len(re)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			re[i], re[j] = re[j], re[i]
			im[i], im[j] = im[j], im[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		ang := -2 * math.Pi / float64(size)
		wr, wi := math.Cos(ang), math.Sin(ang)
		for start := 0; start < n; start += size {
			cr, ci := 1.0, 0.0
			for k := 0; k < size/2; k++ {
				a, b := start+k, start+k+size/2
				tr := re[b]*cr - im[b]*ci
				ti := re[b]*ci + im[b]*cr
				re[b], im[b] = re[a]-tr, im[a]-ti
				re[a], im[a] = re[a]+tr, im[a]+ti
				cr, ci = cr*wr-ci*wi, cr*wi+ci*wr
			}
		}
	}
}

// Shared is a stretch two fingerprints have in common, in seconds from the
// start of each.
type Shared struct {
	StartA, StartB float64
	Length         float64
}

const (
	// Two frames "agree" when at most this many of their 16 bits differ.
	// Unrelated audio differs in about 8.
	maxDiff = 4
	// Within a shared stretch, at least this share of frames agree, counted
	// over smooth frames at a time (a sound effect over the theme, a cut).
	minAgree = 0.55
	smooth   = 80 // frames (about 5 s)
)

// Common finds the longest stretch a and b share, or ok=false when nothing
// of at least minSec seconds is shared.
func Common(a, b []uint16, minSec float64) (s Shared, ok bool) {
	if len(a) == 0 || len(b) == 0 {
		return s, false
	}
	minFrames := int(minSec / FrameSec)
	// The offset (b's index minus a's) at which most frames agree.
	bestOff, bestCount := 0, 0
	for off := -(len(a) - minFrames); off <= len(b)-minFrames; off++ {
		lo, hi := max(0, -off), min(len(a), len(b)-off)
		if hi-lo < minFrames {
			continue
		}
		count := 0
		for i := lo; i < hi; i++ {
			if bits.OnesCount16(a[i]^b[i+off]) <= maxDiff {
				count++
			}
		}
		if count > bestCount {
			bestOff, bestCount = off, count
		}
	}
	if bestCount < minFrames/2 {
		return s, false
	}
	// At that offset, the longest run where agreement stays high.
	lo, hi := max(0, -bestOff), min(len(a), len(b)-bestOff)
	agree := make([]bool, hi-lo)
	for i := lo; i < hi; i++ {
		agree[i-lo] = bits.OnesCount16(a[i]^b[i+bestOff]) <= maxDiff
	}
	start, length := longestRun(agree)
	if length < minFrames {
		return s, false
	}
	return Shared{
		StartA: float64(lo+start) * FrameSec,
		StartB: float64(lo+start+bestOff) * FrameSec,
		Length: float64(length) * FrameSec,
	}, true
}

// longestRun finds the longest stretch of agree where, in every window of
// smooth frames, at least minAgree of them are true. It returns the stretch
// trimmed to start and end on an agreeing frame.
func longestRun(agree []bool) (start, length int) {
	n := len(agree)
	if n < smooth {
		return 0, 0
	}
	// good[i]: the window starting at i agrees enough.
	need := int(math.Ceil(minAgree * smooth))
	count := 0
	for i := 0; i < smooth; i++ {
		if agree[i] {
			count++
		}
	}
	bestS, bestL := 0, 0
	runS := -1
	for i := 0; ; i++ {
		good := count >= need
		if good && runS < 0 {
			runS = i
		}
		if (!good || i+smooth == n) && runS >= 0 {
			end := i + smooth - 1 // last frame of the last good window
			if !good {
				end = i - 1 + smooth - 1
			}
			s, e := tighten(agree, runS, min(end, n-1))
			if e-s+1 > bestL {
				bestS, bestL = s, e-s+1
			}
			runS = -1
		}
		if i+smooth >= n {
			break
		}
		if agree[i] {
			count--
		}
		if agree[i+smooth] {
			count++
		}
	}
	return bestS, bestL
}

// tighten pulls a run's ends in to where the agreement really starts and
// stops. The smoothing that lets a run survive a few bad frames also lets it
// spill a couple of seconds past each end, where less than half agree; the
// ends are where a second's worth of frames nearly all do.
func tighten(agree []bool, s, e int) (int, int) {
	const edge = 16 // frames (about 1 s)
	const need = 12
	dense := func(at int) bool { // do most of the edge frames from at agree?
		if at < 0 || at+edge > len(agree) {
			return false
		}
		c := 0
		for i := at; i < at+edge; i++ {
			if agree[i] {
				c++
			}
		}
		return c >= need
	}
	for s < e && !(agree[s] && dense(s)) {
		s++
	}
	for e > s && !(agree[e] && dense(e-edge+1)) {
		e--
	}
	return s, e
}
