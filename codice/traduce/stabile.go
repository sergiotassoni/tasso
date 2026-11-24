package traduce

// Stabilità nel tempo (tutti i giochi). La schermata viene riletta spesso (sfondi animati, luci,
// effetti): l'OCR legge la STESSA scritta una volta bene, una volta male, una volta per niente.
// Prima la traduzione cambiava sotto gli occhi (tradotta, inglese, sbagliata). Ora:
//
//   - ogni posto dello schermo ricorda le letture fatte finché i pixel della scritta restano gli
//     stessi; vince la lettura vista più volte (le frasi pronte contano di più), a pari voti resta
//     quella già mostrata (isteresi);
//   - una lettura dubbia non cancella una buona già vista nello stesso posto con gli stessi pixel;
//   - se i pixel cambiano davvero (scritta nuova) il posto ricomincia da capo.

import (
	"image"
	"sync"
	"time"
)

type votoScritta struct {
	rett    image.Rectangle
	rit     []uint8 // ritaglio grigio rimpicciolito dei pixel della scritta
	letture map[string]int
	affid   map[string]int
	scelta  string
	vista   time.Time
}

type votazione struct {
	mu   sync.Mutex
	voci []*votoScritta
}

var votiPagina = &votazione{}

const latoRitaglio = 24 // il ritaglio è al massimo 24x24 campioni: basta per vedere se la scritta è cambiata

// ritaglioScritta: campioni di grigio sul rettangolo r (sempre lo stesso numero, qualunque grandezza).
func ritaglioScritta(img *image.RGBA, r image.Rectangle) []uint8 {
	r = r.Intersect(img.Bounds())
	if r.Empty() {
		return nil
	}
	out := make([]uint8, 0, latoRitaglio*latoRitaglio)
	for j := 0; j < latoRitaglio; j++ {
		y := r.Min.Y + (2*j+1)*r.Dy()/(2*latoRitaglio)
		for i := 0; i < latoRitaglio; i++ {
			x := r.Min.X + (2*i+1)*r.Dx()/(2*latoRitaglio)
			c := img.RGBAAt(x, y)
			out = append(out, uint8((299*int(c.R)+587*int(c.G)+114*int(c.B))/1000))
		}
	}
	return out
}

// stessiPixel: i due ritagli sono quasi uguali (la scritta non è cambiata).
func stessiPixel(a, b []uint8) bool {
	if len(a) == 0 || len(a) != len(b) {
		return false
	}
	tot := 0
	for i := range a {
		d := int(a[i]) - int(b[i])
		if d < 0 {
			d = -d
		}
		tot += d
	}
	return tot/len(a) <= 10
}

// sovrapposti: due rettangoli si coprono per più di metà del più piccolo.
func sovrapposti(a, b image.Rectangle) bool {
	in := a.Intersect(b)
	if in.Empty() {
		return false
	}
	am, bm := a.Dx()*a.Dy(), b.Dx()*b.Dy()
	return in.Dx()*in.Dy()*2 >= min(am, bm)
}

// pesoLettura: quanto conta una lettura nel voto.
func pesoLettura(affid int) int {
	switch affid {
	case letturaCatalog:
		return 3
	case letturaBuona:
		return 1
	}
	return 0
}

// scegli: registra la lettura di questo posto e restituisce quella da usare (testo e affidabilità).
func (v *votazione) scegli(img *image.RGBA, b Blocco, testo string, affid int, adesso time.Time) (string, int) {
	r := image.Rect(b.X, b.Y, b.X+b.W, b.Y+b.H)
	rit := ritaglioScritta(img, r)
	v.mu.Lock()
	defer v.mu.Unlock()
	var e *votoScritta
	tieni := v.voci[:0]
	for _, x := range v.voci { // via i posti non visti da mezzo minuto
		if adesso.Sub(x.vista) < 30*time.Second {
			tieni = append(tieni, x)
		}
	}
	v.voci = tieni
	for _, x := range v.voci {
		if sovrapposti(x.rett, r) {
			e = x
			break
		}
	}
	if e != nil && !stessiPixel(e.rit, rit) {
		// i pixel sono cambiati: è un'altra scritta, il posto ricomincia da capo
		e.letture, e.affid, e.scelta = map[string]int{}, map[string]int{}, ""
	}
	if e == nil {
		if len(v.voci) >= 400 {
			v.voci = v.voci[1:]
		}
		e = &votoScritta{letture: map[string]int{}, affid: map[string]int{}}
		v.voci = append(v.voci, e)
	}
	e.rett, e.rit, e.vista = r, rit, adesso
	if p := pesoLettura(affid); p > 0 {
		e.letture[testo] += p
		if affid > e.affid[testo] {
			e.affid[testo] = affid
		}
	}
	// vince la lettura con più voti; a pari voti quella già scelta (niente cambi a ogni rilettura)
	migliore, voti := e.scelta, e.letture[e.scelta]
	for t, n := range e.letture {
		if n > voti {
			migliore, voti = t, n
		}
	}
	if migliore == "" {
		return testo, affid // niente di buono visto qui: la lettura (dubbia) resta com'è
	}
	e.scelta = migliore
	return migliore, e.affid[migliore]
}

func (v *votazione) svuota() {
	v.mu.Lock()
	v.voci = nil
	v.mu.Unlock()
}

// recuperaPerse: le scritte già tradotte che la nuova lettura non ha trovato (l'OCR le ha saltate)
// restano, se sotto i pixel non sono cambiati: niente ritorno all'inglese a testo uguale.
func recuperaPerse(nuovi, vecchi []Blocco, img *image.RGBA) []Blocco {
	if img == nil {
		return nuovi
	}
	out := nuovi
	var pul *image.RGBA // la pagina pulita della lettura nuova (come in tieniFerme)
	for _, n := range nuovi {
		if n.Pulita != nil && n.Img == img && n.Pulita.Bounds() == img.Bounds() {
			pul = n.Pulita
			break
		}
	}
	for _, o := range vecchi {
		if o.Tradotto == "" || o.Img == nil || o.Img.Bounds() != img.Bounds() {
			continue
		}
		ro := image.Rect(o.X, o.Y, o.X+o.W, o.Y+o.H)
		coperta := false
		for _, n := range nuovi {
			if ro.Overlaps(image.Rect(n.X, n.Y, n.X+n.W, n.Y+n.H)) {
				coperta = true
				break
			}
		}
		if coperta || zonaCambiata(o.Img, img, o) || formaCambiata(o.Img, img, o) {
			continue
		}
		if pul != nil && o.Pulita != nil {
			copiaCancellati(pul, o) // le sue lettere cancellate passano nella pagina pulita nuova
			o.Img, o.Pulita = img, pul
		} else if o.Pulita == nil {
			o.Img = img
		} // senza pagina pulita nuova resta con la sua cattura e la sua pagina pulita (coerenti)
		out = append(out, o)
	}
	return out
}
