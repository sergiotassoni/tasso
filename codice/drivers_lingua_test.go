package main

import (
	"strings"
	"testing"
)

// F19-5: i siti dei produttori in italiano devono avere la loro versione nelle altre lingue.
func TestSitiProduttoriInLingua(t *testing.T) {
	italiano := func(u string) bool {
		return strings.Contains(u, "it-it") || strings.Contains(u, "/it/") || strings.Contains(u, "intel.it") ||
			strings.Contains(u, "://it.") || strings.Contains(u, ".it.asp")
	}
	urls := []string{"https://www.nvidia.com/it-it/software/nvidia-app/", "https://www.amd.com/it/support/download/drivers.html",
		"https://www.intel.it/content/www/it/it/support/detect.html"}
	for _, p := range vendorPages {
		urls = append(urls, p[1])
	}
	for _, u := range urls {
		if sitoInLinguaL(u, "it") != u {
			t.Errorf("in italiano cambia: %s", u)
		}
		for _, l := range []string{"en", "es", "fr", "de", "pt"} {
			g := sitoInLinguaL(u, l)
			if italiano(u) && (g == u || italiano(g)) {
				t.Errorf("%s in %s resta in italiano: %s", u, l, g)
			}
			if !strings.HasPrefix(g, "https://") {
				t.Errorf("%s in %s: %s", u, l, g)
			}
		}
	}
	if g := sitoInLinguaL("https://www.nvidia.com/it-it/drivers/", "de"); g != "https://www.nvidia.com/de-de/drivers/" {
		t.Error(g)
	}
}
