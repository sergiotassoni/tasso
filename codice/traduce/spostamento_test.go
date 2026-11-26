package traduce

import (
	"image"
	"math/rand"
	"testing"
	"time"
)

// scenaSpostata: una scena piena di dettagli e la stessa spostata di (dx, dy).
func scenaSpostata(w, h, dx, dy int, seme int64) (*image.RGBA, *image.RGBA) {
	a := image.NewRGBA(image.Rect(0, 0, w, h))
	rnd := rand.New(rand.NewSource(seme))
	for i := 0; i < len(a.Pix); i += 4 {
		v := uint8(rnd.Intn(256))
		a.Pix[i], a.Pix[i+1], a.Pix[i+2], a.Pix[i+3] = v, v/2, 255-v, 255
	}
	b := image.NewRGBA(a.Rect)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sx, sy := x-dx, y-dy
			if sx >= 0 && sy >= 0 && sx < w && sy < h {
				b.SetRGBA(x, y, a.RGBAAt(sx, sy))
			}
		}
	}
	return a, b
}

// La scritta spostata si ritrova, e la ricerca resta veloce anche per un paragrafo di 5 righe
// (prima: circa mezzo secondo per scritta in 1080p).
func TestSpostamento2DVeloceEGiusto(t *testing.T) {
	a, b := scenaSpostata(1920, 1080, 14, -10, 1)
	par := Blocco{X: 600, Y: 400, W: 600, H: 150, Righe: 5}
	t0 := time.Now()
	dx, dy, ok := spostamento2D(a, b, par)
	durata := time.Since(t0)
	if !ok || dx != 14 || dy != -10 {
		t.Errorf("paragrafo spostato di (14,-10): trovato (%d,%d) %v", dx, dy, ok)
	}
	if durata > 60*time.Millisecond {
		t.Errorf("ricerca troppo lenta per un paragrafo: %v", durata)
	}
	riga := Blocco{X: 300, Y: 200, W: 240, H: 24, Righe: 1}
	if dx, dy, ok := spostamento2D(a, b, riga); !ok || dx != 14 || dy != -10 {
		t.Errorf("riga spostata di (14,-10): trovato (%d,%d) %v", dx, dy, ok)
	}
	// ferma: niente spostamento
	if _, _, ok := spostamento2D(a, a, riga); ok {
		t.Error("scritta ferma presa per spostata")
	}
	// oltre il tetto (100 px in giù): non la cerca così lontano
	_, c := scenaSpostata(1920, 1080, 0, 100, 1)
	if _, _, ok := spostamento2D(a, c, riga); ok {
		t.Error("spostamento oltre il tetto trovato lo stesso")
	}
}

// Finito il tempo del giro, le scritte che restano non si cercano più (le rimette a posto la
// lettura dopo): il ciclo della pagina non si blocca.
func TestSeguiScritteBudget(t *testing.T) {
	a, b := scenaSpostata(1280, 720, 6, 4, 2)
	var bl []Blocco
	for i := 0; i < 12; i++ {
		bl = append(bl, Blocco{X: 40 + (i%4)*300, Y: 60 + (i/4)*200, W: 240, H: 24, Righe: 1, Tradotto: "x", Img: a})
	}
	vecchio := budgetSegui
	defer func() { budgetSegui = vecchio }()
	budgetSegui = time.Hour
	if _, n := seguiScritte(b, bl); n != len(bl) {
		t.Fatalf("con tempo libero: spostate %d su %d", n, len(bl))
	}
	budgetSegui = 0
	if _, n := seguiScritte(b, bl); n > 1 {
		t.Errorf("tempo del giro finito: cercate lo stesso %d scritte", n)
	}
}

func BenchmarkSpostamento2DParagrafo(bm *testing.B) {
	a, b := scenaSpostata(1920, 1080, 14, -10, 1)
	par := Blocco{X: 600, Y: 400, W: 600, H: 150, Righe: 5}
	for i := 0; i < bm.N; i++ {
		spostamento2D(a, b, par)
	}
}

// Con letture lente (un giro ogni 1,2 s) una scritta non trovata da UNA lettura resta; mancata a
// due letture di fila va via (prima la grazia fissa di 900 ms la toglieva subito: sfarfallio).
func TestIsteresiGraziaSulGiroVero(t *testing.T) {
	img1 := immagineMondo(600, 300, 0, 0, 1)
	f := lettereIrregolari(img1, 100, 100, 24)
	ist := &isteresi{}
	t0 := time.Unix(1000, 0)
	giro := 1200 * time.Millisecond
	b := bloccoMostrato(img1, "Hold", "Tieni", 100, 100, f-100, 24)
	vis, _ := ist.applica([]Blocco{b}, nil, nil, img1, t0)
	vis, _ = ist.applica([]Blocco{b}, vis, nil, img1, t0.Add(giro)) // trovata di nuovo
	img2 := immagineMondo(600, 300, 0, 0, 2)
	lettereIrregolari(img2, 100, 100, 24)
	vis2, _ := ist.applica(nil, vis, nil, img2, t0.Add(2*giro)) // non trovata da una lettura
	if len(vis2) != 1 {
		t.Fatal("letture lente: la traduzione è sparita dopo una sola lettura mancata")
	}
	img3 := immagineMondo(600, 300, 0, 0, 3)
	lettereIrregolari(img3, 100, 100, 24)
	if v, _ := ist.applica(nil, vis2, nil, img3, t0.Add(3*giro)); len(v) != 0 {
		t.Fatal("mancata a due letture di fila: doveva andare via")
	}
	if g := ist.grazia(); g < graziaIsteresi || g > 3*time.Second {
		t.Errorf("grazia fuori dai limiti: %v", g)
	}
}

// Traduzioni fisse: due parole vere diverse non si scambiano la traduzione (Fail/Fall,
// Mail/Mall, Wait/Walt, Turn/Tum); la stessa scritta letta con un errore dell'OCR sì.
func TestFisseParoleDiverseNonSiConfondono(t *testing.T) {
	f := &traduzioniFisse{}
	f.fissa("en>it", "Fail", "Fallito")
	f.fissa("en>it", "Mail", "Posta")
	f.fissa("en>it", "Wait", "Aspetta")
	for _, w := range []string{"Fall", "Mall", "Tall"} {
		if v, ok := f.cerca("en>it", w); ok {
			t.Errorf("%q ha preso la traduzione fissa %q di un'altra parola", w, v)
		}
	}
	f.fissa("en>it", "Fall", "Cadi")
	if v, _ := f.cerca("en>it", "Fall"); v != "Cadi" {
		t.Errorf("Fall: %q", v)
	}
	if v, _ := f.cerca("en>it", "Fail"); v != "Fallito" {
		t.Errorf("Fail: %q", v)
	}
	// stessa scritta letta male dall'OCR: trova la traduzione fissa
	f.fissa("en>it", "Hold to sprint", "Tieni premuto per scattare")
	for _, letta := range []string{"Hold to sprint.", "HOLD TO SPRINT", "HoId to sprint", "Ho1d to sprint", "Hold  to sprint"} {
		if v, ok := f.cerca("en>it", letta); !ok || v != "Tieni premuto per scattare" {
			t.Errorf("%q: %q %v", letta, v, ok)
		}
	}
	// numeri diversi: scritte diverse
	f.fissa("en>it", "Level 5", "Livello 5")
	if _, ok := f.cerca("en>it", "Level 6"); ok {
		t.Error("Level 6 ha preso la traduzione di Level 5")
	}
}

// Si fissano solo le traduzioni delle scritte disegnate davvero.
func TestFissaSoloLeMostrate(t *testing.T) {
	f := &traduzioniFisse{}
	mostrate := []Blocco{{Testo: "Open Map", DaFissare: "Apri la mappa"}, {Testo: "Old", Tradotto: "Vecchia"}}
	fissaMostrate(f, "en>it", mostrate)
	if v, _ := f.cerca("en>it", "Open Map"); v != "Apri la mappa" {
		t.Errorf("Open Map: %q", v)
	}
	if f.fissata("en>it", "Old") {
		t.Error("fissata una scritta senza traduzione da fissare")
	}
	if f.fissata("en>it", "Scartata") {
		t.Error("fissata una scritta mai mostrata")
	}
}
