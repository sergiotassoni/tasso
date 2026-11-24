package traduce

import "testing"

func TestComponiBadge(t *testing.T) {
	tw, th := 100, 14
	mask := make([]uint8, tw*th)
	for i := range mask {
		mask[i] = 255
	}
	img := componiBadge(mask, tw, th, 1.33, 1, false, 1)
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if w != larghezzaBadge(tw, 1.33) || h != 45 {
		t.Fatalf("misure %dx%d", w, h)
	}
	if img.RGBAAt(0, 0).A != 0 || img.RGBAAt(w-1, h-1).A != 0 {
		t.Fatal("gli angoli devono essere trasparenti (arrotondati)")
	}
	if a := img.RGBAAt(w/2, 3).A; a < 150 || a == 255 {
		t.Fatalf("fondo semitrasparente atteso, alfa %d", a)
	}
	// il puntino menta
	_, pad, st, sp, pu := misureBadge(1.33)
	c := img.RGBAAt(pad+st+sp+pu/2, h/2)
	if c.G < 200 || c.R > 120 {
		t.Fatalf("puntino non menta: %v", c)
	}
	// spento: grigio
	c = componiBadge(mask, tw, th, 1.33, 1, true, 1).RGBAAt(pad+st+sp+pu/2, h/2)
	if c.G > 180 || c.R < 120 {
		t.Fatalf("puntino spento non grigio: %v", c)
	}
	// dissolvenza
	if a := componiBadge(mask, tw, th, 1.33, 1, false, 0.5).RGBAAt(w/2, 3).A; a > 110 {
		t.Fatalf("dissolvenza: alfa %d", a)
	}
	if p := premoltiplicaBGRA(img); p[(h/2*w+w/2)*4+3] == 0 {
		t.Fatal("premoltiplicato vuoto")
	}
}
