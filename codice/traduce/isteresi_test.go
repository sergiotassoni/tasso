package traduce

import (
	"context"
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// tramaMondo: sfondo "del mondo" con un disegno fine (sempre lo stesso per lo stesso punto del mondo):
// spostando la visuale di (ox, oy) si sposta anche lui.
func tramaMondo(x, y, seme int) uint8 {
	h := uint32(x*73856093) ^ uint32(y*19349663) ^ uint32(seme*83492791)
	h ^= h >> 13
	h *= 0x5bd1e995
	h ^= h >> 15
	return uint8(20 + h%50)
}

// immagineMondo: sfondo con la trama spostato di (ox, oy); seme diverso = sfondo diverso (la
// visuale gira dietro un riquadro trasparente).
func immagineMondo(w, h, ox, oy, seme int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := tramaMondo(x-ox, y-oy, seme)
			img.SetRGBA(x, y, color.RGBA{v, v, v + 10, 255})
		}
	}
	return img
}

// lettereIrregolari: "lettere" bianche di larghezze e altezze diverse (non si ritrovano spostate
// di una lettera, come una scritta vera). Restituisce dove finiscono.
func lettereIrregolari(img *image.RGBA, x, y, h int) int {
	larg := []int{4, 9, 3, 7, 5, 11, 3, 6, 8, 4, 10, 5, 3, 7}
	alto := []int{0, 6, 0, 3, 0, 0, 8, 0, 2, 0, 0, 5, 0, 0}
	bianco := color.RGBA{240, 240, 240, 255}
	for k, l := range larg {
		for yy := y + alto[k]; yy < y+h; yy++ {
			for xx := x; xx < x+l; xx++ {
				img.SetRGBA(xx, yy, bianco)
			}
		}
		x += l + 4
	}
	return x - 4
}

func bloccoMostrato(img *image.RGBA, testo, tradotto string, x, y, w, h int) Blocco {
	b := Blocco{Testo: testo, Tradotto: tradotto, X: x, Y: y, W: w, H: h, Righe: 1, Em: float64(h), Img: img}
	coloriBlocco(img, &b)
	return b
}

// Una lettura dubbia (inglese) della stessa scritta nello stesso posto, o nessuna lettura, non
// toglie la traduzione già mostrata (prima tornava l'inglese per un giro: sfarfallio).
func TestIsteresiNonTornaInglese(t *testing.T) {
	img1 := immagineMondo(600, 300, 0, 0, 1)
	f := lettereIrregolari(img1, 100, 100, 24)
	img2 := immagineMondo(600, 300, 0, 0, 1)
	lettereIrregolari(img2, 100, 100, 24)
	ist := &isteresi{}
	t0 := time.Unix(1000, 0)
	o := bloccoMostrato(img1, "Sprint by Holding Shift", "Corri tenendo premuto Shift", 100, 100, f-100, 24)
	vis, _ := ist.applica([]Blocco{o}, nil, nil, img1, t0)
	if len(vis) != 1 {
		t.Fatalf("prima lettura: %d scritte", len(vis))
	}
	// letta peggio: resta in inglese nella lettura, ma lo schermo tiene l'italiano
	e := Blocco{Testo: "Sprint by Holdng Shift", X: 101, Y: 100, W: f - 101, H: 24, Righe: 1}
	vis2, ingl := ist.applica(nil, vis, []Blocco{e}, img2, t0.Add(250*time.Millisecond))
	if len(vis2) != 1 || vis2[0].Tradotto != "Corri tenendo premuto Shift" || len(ingl) != 0 {
		t.Fatalf("lettura dubbia: mostrate %+v, inglesi %d", vis2, len(ingl))
	}
	// nessuna lettura: resta
	vis3, _ := ist.applica(nil, vis2, nil, img2, t0.Add(500*time.Millisecond))
	if len(vis3) != 1 {
		t.Fatal("fotogramma senza lettura: la traduzione è sparita")
	}
	// una lettura inglese PIÙ LARGA (la frase continua fuori dal riquadro): mai mezza inglese
	lunga := Blocco{Testo: "Sprint by Holding Shift and jump over the fence", X: 100, Y: 100, W: f + 200, H: 24, Righe: 1}
	if v, _ := ist.applica(nil, vis, []Blocco{lunga}, img2, t0.Add(300*time.Millisecond)); len(v) != 0 {
		t.Fatal("tenuta la traduzione accanto a un pezzo inglese della stessa frase")
	}
	// lettere cambiate sotto: la traduzione vecchia non resta
	img3 := immagineMondo(600, 300, 0, 0, 1)
	if v, _ := ist.applica(nil, vis, nil, img3, t0.Add(300*time.Millisecond)); len(v) != 0 {
		t.Fatal("tenuta una traduzione sopra una scritta che non c'è più")
	}
}

// Il tempo di grazia: dopo circa un secondo senza nessuna lettura che la confermi, la traduzione
// tenuta va via (lo sfondo si muove, non posso esserne sicuro per sempre).
func TestIsteresiTempoDiGrazia(t *testing.T) {
	img1 := immagineMondo(600, 300, 0, 0, 1)
	f := lettereIrregolari(img1, 100, 100, 24)
	ist := &isteresi{}
	t0 := time.Unix(1000, 0)
	vis, _ := ist.applica([]Blocco{bloccoMostrato(img1, "Hold", "Tieni", 100, 100, f-100, 24)}, nil, nil, img1, t0)
	for i := 1; i <= 3; i++ {
		img := immagineMondo(600, 300, 0, 0, 1+i) // lo sfondo dietro cambia
		lettereIrregolari(img, 100, 100, 24)
		vis, _ = ist.applica(nil, vis, nil, img, t0.Add(time.Duration(i)*250*time.Millisecond))
		if len(vis) != 1 {
			t.Fatalf("giro %d: la traduzione è sparita dentro il tempo di grazia", i)
		}
	}
	img := immagineMondo(600, 300, 0, 0, 9)
	lettereIrregolari(img, 100, 100, 24)
	if v, _ := ist.applica(nil, vis, nil, img, t0.Add(1200*time.Millisecond)); len(v) != 0 {
		t.Fatal("dopo il tempo di grazia la traduzione non confermata doveva andare via")
	}
}

// Un'altra traduzione nello stesso posto, con le stesse lettere sotto, si mostra solo se due
// letture di fila la danno uguale. Se le lettere cambiano davvero, subito.
func TestIsteresiCambioConfermato(t *testing.T) {
	img1 := immagineMondo(600, 300, 0, 0, 1)
	f := lettereIrregolari(img1, 100, 100, 24)
	img2 := immagineMondo(600, 300, 0, 0, 1)
	lettereIrregolari(img2, 100, 100, 24)
	ist := &isteresi{}
	t0 := time.Unix(1000, 0)
	a := bloccoMostrato(img1, "Sprint by Holding Shift", "Corri tenendo premuto Shift", 100, 100, f-100, 24)
	vis, _ := ist.applica([]Blocco{a}, nil, nil, img1, t0)
	b := bloccoMostrato(img2, "Sprint by Holdinq Shift", "Scatta tenendo Shift", 100, 100, f-100, 24)
	c := bloccoMostrato(img2, "Sprint by Holding Shift", "Corri tenendo premuto Shift", 100, 100, f-100, 24)
	// B, A, B, A...: non cambia mai
	for i, n := range []Blocco{b, c, b, c} {
		vis, _ = ist.applica([]Blocco{n}, vis, nil, img2, t0.Add(time.Duration(i+1)*250*time.Millisecond))
		if len(vis) != 1 || vis[0].Tradotto != a.Tradotto {
			t.Fatalf("giro %d: la traduzione è cambiata con una lettura sola: %q", i, vis[0].Tradotto)
		}
	}
	// B due volte di fila: cambia
	vis, _ = ist.applica([]Blocco{b}, vis, nil, img2, t0.Add(2*time.Second))
	if vis[0].Tradotto != a.Tradotto {
		t.Fatal("cambiata alla prima lettura")
	}
	vis, _ = ist.applica([]Blocco{b}, vis, nil, img2, t0.Add(2250*time.Millisecond))
	if vis[0].Tradotto != b.Tradotto {
		t.Fatalf("due letture uguali di fila non hanno cambiato la traduzione: %q", vis[0].Tradotto)
	}
	// lettere diverse sotto (scritta nuova del gioco): subito
	img3 := immagineMondo(600, 300, 0, 0, 1)
	lettereIrregolari(img3, 100, 112, 12) // lettere diverse: solo in basso
	d := bloccoMostrato(img3, "Crouch with Ctrl", "Accovacciati con Ctrl", 100, 100, f-100, 24)
	vis, _ = ist.applica([]Blocco{d}, vis, nil, img3, t0.Add(3*time.Second))
	if len(vis) != 1 || vis[0].Tradotto != d.Tradotto {
		t.Fatalf("scritta nuova non mostrata subito: %+v", vis)
	}
}

// Un fotogramma in cui l'OCR non legge niente non toglie le scritte le cui lettere sono ancora lì.
func TestAncoraLi(t *testing.T) {
	img1 := immagineMondo(600, 300, 0, 0, 1)
	f := lettereIrregolari(img1, 100, 100, 24)
	g := lettereIrregolari(img1, 100, 200, 24)
	bl := []Blocco{bloccoMostrato(img1, "Hold", "Tieni", 100, 100, f-100, 24), bloccoMostrato(img1, "Skip", "Salta", 100, 200, g-100, 24)}
	img2 := immagineMondo(600, 300, 0, 0, 1)
	lettereIrregolari(img2, 100, 100, 24) // la seconda scritta è sparita
	r := ancoraLi(img2, bl)
	if len(r) != 1 || r[0].Testo != "Hold" {
		t.Fatalf("ancoraLi: %+v", r)
	}
}

// La visuale si sposta (o la pagina scorre): la traduzione segue la scritta nel posto nuovo,
// invece di sparire e ricomparire dopo la lettura.
func TestSeguiScritte(t *testing.T) {
	img1 := immagineMondo(800, 400, 0, 0, 1)
	f := lettereIrregolari(img1, 200, 150, 24)
	b := bloccoMostrato(img1, "Sprint by Holding Shift", "Corri tenendo premuto Shift", 200, 150, f-200, 24)
	b.Pulita = pulisciPagina(img1, []Blocco{b})
	// tutto spostato di 18 a destra e 8 in su
	img2 := immagineMondo(800, 400, 18, -8, 1)
	lettereIrregolari(img2, 218, 142, 24)
	out, n := seguiScritte(img2, []Blocco{b})
	if n != 1 || out[0].X != 218 || out[0].Y != 142 || out[0].Img != img2 || out[0].Tradotto != b.Tradotto {
		t.Fatalf("scritta non seguita: n=%d %d,%d", n, out[0].X, out[0].Y)
	}
	if out[0].Pulita == nil || out[0].Pulita.Bounds() != img2.Bounds() {
		t.Fatal("la scritta seguita non ha la pagina pulita nuova")
	}
	// suggerimento fermo sullo schermo, mondo che gira dietro: non si sposta
	img3 := immagineMondo(800, 400, 30, 0, 5)
	lettereIrregolari(img3, 200, 150, 24)
	if _, n := seguiScritte(img3, []Blocco{b}); n != 0 {
		t.Fatal("scritta ferma presa per spostata")
	}
	// scritta sparita: niente da seguire
	if _, n := seguiScritte(immagineMondo(800, 400, 18, -8, 1), []Blocco{b}); n != 0 {
		t.Fatal("seguita una scritta che non c'è più")
	}
}

// motoreContato: un motore che risponde ogni volta in modo diverso (come un correttore IA che
// cambia idea) e conta le chiamate.
type motoreContato struct{ n *atomic.Int32 }

func (motoreContato) Nome() string { return "Mozilla" }
func (m motoreContato) Traduci(_ context.Context, r RichiestaTraduzione) (string, error) {
	k := m.n.Add(1)
	return "versione " + string(rune('A'+k-1)) + " di " + strings.ToLower(r.Testo), nil
}

// La stessa scritta inglese esce sempre con la stessa traduzione: la prima mostrata resta per
// tutta la sessione, anche se il motore (o la memoria) direbbe altro, e ricompare subito dopo
// aver guardato altrove (anche nel primo giro "solo veloce").
func TestTraduzioniFisse(t *testing.T) {
	var n atomic.Int32
	tr := NuovoTraduttore(filepath.Join(t.TempDir(), "mem.json"), glossarioBase(), motoreContato{&n})
	f := &traduzioniFisse{}
	da := []Blocco{{Testo: "Repair the broken coffee machine"}}
	r1 := traduciTutteFisse(context.Background(), tr, da, false, f)
	if r1[0].Tradotto == "" {
		t.Fatalf("prima traduzione vuota: %+v", r1[0])
	}
	f.fissa(coppiaDi(tr), da[0].Testo, r1[0].Tradotto)
	// memoria nuova (come dopo una correzione dell'IA): il motore direbbe "versione B"
	tr = NuovoTraduttore(filepath.Join(t.TempDir(), "mem2.json"), glossarioBase(), motoreContato{&n})
	prima := n.Load()
	for _, testo := range []string{"Repair the broken coffee machine", "Repair  the broken coffee rnachine", "REPAIR THE BROKEN COFFEE MACHINE"} {
		r := traduciTutteFisse(context.Background(), tr, []Blocco{{Testo: testo}}, true, f)
		if r[0].Tradotto != r1[0].Tradotto || r[0].Errore != "" {
			t.Fatalf("%q: %q (attesa %q, errore %q)", testo, r[0].Tradotto, r1[0].Tradotto, r[0].Errore)
		}
	}
	if n.Load() != prima {
		t.Error("una scritta con la traduzione fissa è passata di nuovo dal motore")
	}
	// numeri diversi: scritta diversa
	if f.fissata(coppiaDi(tr), "Level 5") || chiaveFissa("en>it", "Level 5") == chiaveFissa("en>it", "Level 6") {
		t.Error("Level 5 e Level 6 devono essere scritte diverse")
	}
	// la prima vince
	f.fissa("en>it", "Hold", "Tieni premuto")
	f.fissa("en>it", "Hold", "Aspetta")
	if v, _ := f.cerca("en>it", "Hold"); v != "Tieni premuto" {
		t.Errorf("la traduzione fissa è cambiata: %q", v)
	}
	// un'altra coppia di lingue non la vede
	if f.fissata("en>de", "Hold") {
		t.Error("traduzione fissa usata per un'altra lingua")
	}
}

// Sequenza di fotogrammi con lettura che "balla" (buona, dubbia, niente, buona…) e lo sfondo che
// cambia dietro la scritta, nel percorso completo del banco: la traduzione mostrata è la stessa
// in TUTTI i fotogrammi (prima spariva quando la lettura saltava e lo sfondo si muoveva).
func TestSequenzaLetturaBallerina(t *testing.T) {
	tr := NuovoTraduttore(filepath.Join(t.TempDir(), "mem.json"), glossarioBase(), motoreDizionario{})
	var f int
	cornice := func(seme int) *image.RGBA {
		img := immagineMondo(900, 400, 0, 0, seme)
		f = lettereIrregolari(img, 60, 60, 26)
		return img
	}
	cornice(1)
	parole := func(ts ...string) []RigaOCR {
		var p []ParolaOCR
		x, w := 60.0, float64(f-60)/float64(len(ts))
		for _, s := range ts {
			p = append(p, ParolaOCR{T: s, X: x, Y: 60, W: w - 6, H: 26})
			x += w
		}
		return []RigaOCR{{Parole: p}}
	}
	buona := parole("Press", "any", "key", "to", "continue")
	brutta := parole("Prxqz", "zny", "kqv", "tn", "cxntinvq")
	letture := [][]RigaOCR{buona, brutta, nil, buona, nil, brutta, buona, nil}
	var lettura []RigaOCR
	o := opzioniBanco{leggi: func(*image.RGBA) ([]RigaOCR, float64, error) { return lettura, 1, nil }, tr: tr,
		voti: &votazione{}, stato: &statoBanco{}}
	var esiti []esitoImmagine
	var attesa string
	for i, l := range letture {
		lettura = l
		img := cornice(1 + i) // lo sfondo dietro le lettere cambia a ogni fotogramma
		e, p := elaboraImmagine(img, "f"+string(rune('0'+i)), o)
		esiti = append(esiti, e)
		if len(p) != 1 {
			t.Fatalf("fotogramma %d (%d parole lette): %d scritte mostrate: %+v", i, len(l), len(p), e.Righe)
		}
		if i == 0 {
			attesa = p[0].Tradotto
		} else if p[0].Tradotto != attesa {
			t.Fatalf("fotogramma %d: %q invece di %q", i, p[0].Tradotto, attesa)
		}
	}
	for i := 1; i < len(esiti); i++ {
		if ins := confrontaFotogrammi(esiti[i-1], esiti[i]); len(ins) != 0 {
			t.Errorf("instabilità %d→%d: %+v", i-1, i, ins)
		}
	}
}
