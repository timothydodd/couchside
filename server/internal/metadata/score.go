package metadata

import (
	"strings"
	"unicode"
)

// Normalize reduces a title to comparable words: lower case, "&" as "and",
// punctuation dropped, a leading "the" removed. "The Goonies" and "Goonies",
// "Alien - Resurrection" and "Alien: Resurrection" normalize the same.
func Normalize(t string) string {
	t = strings.ToLower(strings.ReplaceAll(t, "&", " and "))
	var b strings.Builder
	for _, r := range t {
		if f, ok := fold[r]; ok {
			r = f
		}
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
		case r == '\'' || r == '’' || r == '·':
			// Apostrophes and interpuncts join words: "hell's" → "hells", "wall·e" → "walle".
		default:
			b.WriteRune(' ')
		}
	}
	f := strings.Fields(b.String())
	if len(f) > 1 && f[0] == "the" {
		f = f[1:]
	}
	return strings.Join(f, " ")
}

// fold maps accented Latin letters to plain ones ("máscara" = "mascara").
var fold = func() map[rune]rune {
	m := map[rune]rune{}
	for plain, accented := range map[rune]string{
		'a': "áàâäãåā", 'e': "éèêëē", 'i': "íìîïī", 'o': "óòôöõøō", 'u': "úùûüū",
		'n': "ñ", 'c': "ç", 'y': "ýÿ", 's': "š", 'z': "ž",
	} {
		for _, r := range accented {
			m[r] = plain
		}
	}
	return m
}()

// TitleScore rates how well a candidate title fits the title we looked up:
// 3 exact; 1 when one adds a subtitle ("Harry Potter and the Deathly
// Hallows" → "...: Part 1") or a franchise prefix ("Return of the Jedi" →
// "Star Wars: Episode VI - Return of the Jedi"); 0 unrelated.
func TitleScore(query, candidate string) int {
	q, c := Normalize(query), Normalize(candidate)
	switch {
	case q == "" || c == "":
		return 0
	case q == c:
		return 3
	case strings.HasPrefix(c, q+" ") || strings.HasPrefix(q, c+" "):
		return 1
	}
	// Franchise prefix: the query is everything after a ": " or " - " in the candidate.
	for _, sep := range []string{": ", " - "} {
		for i := strings.Index(candidate, sep); i >= 0; {
			if Normalize(candidate[i+len(sep):]) == q {
				return 1
			}
			j := strings.Index(candidate[i+len(sep):], sep)
			if j < 0 {
				break
			}
			i += len(sep) + j
		}
	}
	return 0
}

// candidate is one possible answer gathered from several queries.
type candidate struct {
	id, title string
	year      int
}

// pickBest chooses the candidate that fits title and year best, or false when
// none is acceptable. Movies must agree on the year (±1, release dates vary by
// country). Series premiere years are fuzzier: ±2, or any year for an exact
// title when nothing closer exists.
func pickBest(kind Kind, title string, year int, cands []candidate) (candidate, bool) {
	best, bestScore := candidate{}, -1
	for _, c := range cands {
		ts := TitleScore(title, c.title)
		if ts == 0 {
			continue
		}
		score := ts * 10
		if year > 0 && c.year > 0 {
			d := abs(c.year - year)
			switch {
			case d == 0:
				score += 5
			case d == 1:
				score += 3
			case kind == Series && d <= 2:
				score += 1
			case kind == Series && ts == 3:
				score -= 20 // exact show name, different start year: only if nothing closer
			default:
				continue
			}
		}
		if score > bestScore {
			best, bestScore = c, score
		}
	}
	return best, bestScore >= 0
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
