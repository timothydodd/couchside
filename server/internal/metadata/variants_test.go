package metadata

import (
	"slices"
	"testing"
)

func TestTitleVariants(t *testing.T) {
	cases := map[string]string{
		"3.10 to Yuma":                          "3:10 to Yuma",
		"Armour of God II - Operation Condor":   "Armour of God II: Operation Condor",
		"Planes, Trains and Automobiles":        "Planes, Trains & Automobiles",
		"Stephen King's It":                     "It",
		"Dr. Seuss' The Grinch":                 "The Grinch",
		"Plagues & Pleasures On the Salton Sea": "Plagues and Pleasures On the Salton Sea",
	}
	for in, want := range cases {
		if got := TitleVariants(in); !slices.Contains(got, want) {
			t.Errorf("TitleVariants(%q) = %q, want it to include %q", in, got, want)
		}
	}
	if got := TitleVariants("Heat"); len(got) != 0 {
		t.Errorf("plain title should have no variants, got %q", got)
	}
}
