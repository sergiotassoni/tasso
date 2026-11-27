package traduce

import "testing"

func TestSottotitoliDiversiPocheParole(t *testing.T) {
	for _, c := range [][2]string{
		{"Did you find the key to the old gate?", "Did you find the map to the old gate?"},
		{"Meet me at the north gate before dawn.", "Meet me at the south gate before dawn."},
		{"He said he would wait for him there.", "She said he would wait for her there."},
	} {
		if stessoSottotitolo(c[0], c[1]) {
			t.Errorf("battute diverse prese per uguali: %q / %q", c[0], c[1])
		}
	}
	if !stessoSottotitolo("Meet me at the north gate before dawn.", "Meet me at the nörth gate before dawn.") {
		t.Error("una lettera sbagliata dall'OCR: è lo stesso sottotitolo")
	}
}
