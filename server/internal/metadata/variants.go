package metadata

import (
	"regexp"
	"strings"
)

var (
	reDigitDot  = regexp.MustCompile(`(\d)\.(\d)`)
	rePossessor = regexp.MustCompile(`^((?:\S+\s){0,2}\S+?)'s?\s+(.+)$`) // "Stephen King's It", "Dr. Seuss' The Grinch"
)

// TitleVariants returns alternative spellings of a title parsed from a
// filename, most likely first. Used only when the exact title didn't match.
func TitleVariants(t string) []string {
	seen := map[string]bool{strings.ToLower(t): true}
	var out []string
	add := func(v string) {
		v = strings.Join(strings.Fields(v), " ")
		if v != "" && !seen[strings.ToLower(v)] {
			seen[strings.ToLower(v)] = true
			out = append(out, v)
		}
	}
	// Windows can't store ":" in names: "3.10 to Yuma" → "3:10 to Yuma".
	add(reDigitDot.ReplaceAllString(t, "$1:$2"))
	// "Armour of God II - Operation Condor" → "Armour of God II: Operation Condor".
	if strings.Contains(t, " - ") {
		add(strings.Replace(t, " - ", ": ", 1))
	}
	add(strings.ReplaceAll(t, " and ", " & "))
	add(strings.ReplaceAll(t, " & ", " and "))
	// Marketing possessives: "Stephen King's It" → "It".
	if m := rePossessor.FindStringSubmatch(t); m != nil && len(strings.Fields(m[2])) >= 1 {
		add(m[2])
	}
	return out
}
