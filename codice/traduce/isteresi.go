package traduce

// Niente sfarfallio (26/11, prova in PC Cafe Simulator, schermata del tutorial con i
// suggerimenti dei tasti sopra il mondo che si muove). Le cause trovate:
//
//  1. una rilettura legge la stessa scritta un po' peggio (affidabilità più bassa, una lettera
//     sbagliata): la scritta restava in inglese per un giro e poi tornava italiana. recuperaPerse
//     non la teneva perché lì "c'era una lettura inglese";
//  2. una rilettura non trova la scritta, e sotto c'è uno sfondo che si muove (la visuale gira
//     dietro un riquadro trasparente): la zona era "cambiata" e la traduzione spariva per un giro;
//  3. una lettura un po' diversa ("Holding" / "Holdng") dava un'altra traduzione, poi di nuovo la
//     prima: le parole cambiavano a ogni giro;
//  4. una lettura "niente da tradurre" (un fotogramma in cui l'OCR non legge niente) toglieva
//     tutte le traduzioni;
//  5. quando la scritta scorre o la visuale si sposta, la traduzione veniva tolta e rifatta da
//     capo dopo la lettura, invece di seguire la scritta.
//
// Le regole (isteresi):
//   - una traduzione mostrata resta per un tempo di grazia (graziaIsteresi) se le letture dopo
//     non la trovano o la leggono in modo dubbio, purché le lettere sotto siano ancora le stesse
//     (stessa forma: formaCambiata) e la lettura inglese stia dentro il suo riquadro (così non
//     resta mai mezza riga inglese accanto all'italiano);
//   - una traduzione diversa nello stesso posto, con le lettere sotto uguali, si mostra solo se
//     due letture di fila la danno uguale;
//   - le scritte che si sono spostate si ritrovano nell'immagine nuova (stesso disegno, poco più
//     in là) e la traduzione le segue.

import (
	"image"
	"strings"
	"sync"
	"time"
)

// graziaIsteresi: per quanto una traduzione mostrata resta se le letture dopo non la confermano.
const graziaIsteresi = 900 * time.Millisecond

type postoIsteresi struct {
	r        image.Rectangle
	tradotto string    // la traduzione mostrata qui
	letta    time.Time // l'ultima lettura che l'ha trovata (tradotta) in questo posto
	cand     string    // un'altra traduzione letta qui, in attesa di conferma
	candN    int
}

type isteresi struct {
	mu     sync.Mutex
	posti  []*postoIsteresi
	ultimo time.Time     // l'ultima lettura passata di qui
	giro   time.Duration // durata media di un giro di lettura (media mobile)
}

// grazia: il tempo di grazia rispetto al giro vero. Con letture lente (motore non veloce, IA
// che si carica, schermo che si muove) un giro supera i 900 ms: la grazia fissa scadeva prima
// della lettura dopo e la traduzione spariva per un giro (collaudo 26/11). Così una scritta può
// mancare a UNA lettura senza sparire, qualunque sia la durata del giro (tetto 3 s).
func (s *isteresi) grazia() time.Duration {
	return min(max(graziaIsteresi, s.giro*3/2), 3*time.Second)
}

// misuraGiro: aggiorna la durata media del giro con la lettura di adesso.
func (s *isteresi) misuraGiro(adesso time.Time) {
	if !s.ultimo.IsZero() {
		if d := adesso.Sub(s.ultimo); d > 0 && d < 10*time.Second {
			if s.giro == 0 {
				s.giro = d
			} else {
				s.giro = (s.giro*3 + d) / 4
			}
		}
	}
	s.ultimo = adesso
}

// isteresiPagina: quella della traduzione dello schermo.
var isteresiPagina = &isteresi{}

func (s *isteresi) svuota() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.posti, s.ultimo, s.giro = nil, time.Time{}, 0
	s.mu.Unlock()
}

func rettBlocco(b Blocco) image.Rectangle { return image.Rect(b.X, b.Y, b.X+b.W, b.Y+b.H) }

// posto: il posto di r (nil se non c'è e crea è falso).
func (s *isteresi) posto(r image.Rectangle, crea bool, adesso time.Time) *postoIsteresi {
	// il posto che si sovrappone di più (non il primo: con due scritte vicine di misura simile la
	// scritta vecchia ereditava la lettura della vicina)
	var meglio *postoIsteresi
	area := 0
	for _, p := range s.posti {
		if sovrapposti(p.r, r) && stessaMisura(p.r, r) {
			in := p.r.Intersect(r)
			if a := in.Dx() * in.Dy(); meglio == nil || a > area {
				meglio, area = p, a
			}
		}
	}
	if meglio != nil {
		return meglio
	}
	if !crea {
		return nil
	}
	p := &postoIsteresi{r: r, letta: adesso}
	s.posti = append(s.posti, p)
	if len(s.posti) > 400 {
		s.posti = s.posti[1:]
	}
	return p
}

// stessaFormaSotto: le lettere della scritta vecchia o sono ancora lì, uguali, nell'immagine nuova.
func stessaFormaSotto(o Blocco, img *image.RGBA) bool {
	return o.Img != nil && img != nil && o.Img.Bounds() == img.Bounds() && !formaCambiata(o.Img, img, o)
}

// dentroRiquadro: il riquadro di e sta dentro quello di o (con un piccolo margine): una lettura
// inglese lì è la stessa scritta, non un pezzo di frase che esce fuori dalla traduzione.
func dentroRiquadro(e, o Blocco) bool {
	m := max(4, o.H/2)
	return rettBlocco(e).In(rettBlocco(o).Inset(-m))
}

// applica: le scritte pronte di questa lettura (nuovi) e quelle rimaste in inglese (inglesi),
// confrontate con quelle sullo schermo (vecchi). Restituisce le scritte da mostrare e le
// inglesi che restano (quelle coperte da una traduzione tenuta non contano più).
func (s *isteresi) applica(nuovi, vecchi, inglesi []Blocco, img *image.RGBA, adesso time.Time) ([]Blocco, []Blocco) {
	if s == nil || img == nil {
		return nuovi, inglesi
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.misuraGiro(adesso)
	tieni := s.posti[:0]
	for _, p := range s.posti { // via i posti non visti da mezzo minuto
		if adesso.Sub(p.letta) < 30*time.Second {
			tieni = append(tieni, p)
		}
	}
	s.posti = tieni
	out := append([]Blocco(nil), nuovi...)
	usato := make([]bool, len(vecchi))
	gestito := make([]bool, len(out))
	// 1. stesso posto, traduzione diversa, lettere sotto uguali: serve una conferma
	for i := range out {
		n := out[i]
		rn := rettBlocco(n)
		for j, o := range vecchi {
			if usato[j] || o.Tradotto == "" {
				continue
			}
			ro := rettBlocco(o)
			if !sovrapposti(ro, rn) || !stessaMisura(ro, rn) {
				continue
			}
			usato[j], gestito[i] = true, true
			p := s.posto(ro, true, adesso)
			p.letta = adesso
			// la scritta è cambiata davvero (altri numeri, altre parole: "20 HEALTH" dopo "10
			// HEALTH", la voce dopo in un elenco): la traduzione nuova subito, mai quella vecchia
			// sopra un'altra scritta
			if o.Tradotto == n.Tradotto || !stessaFormaSotto(o, img) || !testoSimile(o.Testo, n.Testo) {
				p.r, p.tradotto, p.cand, p.candN = rn, n.Tradotto, "", 0
				break
			}
			if p.cand == n.Tradotto && p.candN >= 1 { // seconda lettura di fila uguale: si cambia
				p.r, p.tradotto, p.cand, p.candN = rn, n.Tradotto, "", 0
				break
			}
			p.cand, p.candN = n.Tradotto, 1
			out[i] = o // per ora resta quella già sullo schermo
			break
		}
	}
	// le scritte nuove senza una vecchia nello stesso posto: posto nuovo (o confermato)
	for i, n := range out {
		if gestito[i] {
			continue
		}
		p := s.posto(rettBlocco(n), true, adesso)
		p.r, p.tradotto, p.letta, p.cand, p.candN = rettBlocco(n), n.Tradotto, adesso, "", 0
	}
	// 2. vecchie non ritrovate tradotte: restano per il tempo di grazia, se le lettere sono le stesse
	restano := append([]Blocco(nil), inglesi...)
	for j, o := range vecchi {
		if usato[j] || o.Tradotto == "" || contieneBlocco(out, o) {
			continue
		}
		ro := rettBlocco(o)
		coperta := false
		for _, n := range out {
			if ro.Overlaps(rettBlocco(n)) {
				coperta = true
				break
			}
		}
		if coperta {
			continue // al suo posto c'è già un'altra traduzione
		}
		p := s.posto(ro, false, adesso)
		if p == nil || adesso.Sub(p.letta) > s.grazia() || !stessaFormaSotto(o, img) {
			continue
		}
		// le letture inglesi che toccano la scritta vecchia: tutte dentro il suo riquadro e con
		// un testo simile (la stessa scritta letta peggio), se no la traduzione vecchia lascerebbe
		// fuori un pezzo inglese
		var dentro []int
		ok := true
		for k, e := range restano {
			if !ro.Overlaps(rettBlocco(e)) {
				continue
			}
			if !dentroRiquadro(e, o) || !testoSimile(e.Testo, o.Testo) {
				ok = false
				break
			}
			dentro = append(dentro, k)
		}
		if !ok {
			continue
		}
		for k := len(dentro) - 1; k >= 0; k-- {
			restano = append(restano[:dentro[k]], restano[dentro[k]+1:]...)
		}
		out = append(out, o)
	}
	return out, restano
}

// testoSimile: due letture della stessa scritta, anche con qualche lettera sbagliata o una
// parola persa ("Sprint by Holdng Shift", "Sprint by Shift"). Più largo di stessaScritta: qui
// serve solo a tenere la traduzione già vista per meno di un secondo.
func testoSimile(a, b string) bool {
	if stessaScritta(a, b) {
		return true
	}
	if numeriESegni(a) != numeriESegni(b) {
		return false
	}
	la, lb := lettereConfuse(a), lettereConfuse(b)
	if la == "" || lb == "" {
		return la == lb
	}
	if strings.Contains(la, lb) || strings.Contains(lb, la) {
		return min(len(la), len(lb))*2 >= max(len(la), len(lb))
	}
	return distanzaModifica(la, lb)*4 <= max(len(la), len(lb))
}

// ancoraLi: le scritte mostrate le cui lettere inglesi sono ancora lì, uguali, nell'immagine
// nuova. Una lettura che non trova niente da tradurre non toglie queste (un fotogramma letto
// male non fa sparire tutto); toglie solo quelle che sotto sono cambiate davvero.
func ancoraLi(img *image.RGBA, bl []Blocco) []Blocco {
	var out []Blocco
	for _, b := range bl {
		if b.Inchiostro == coloreTestoFascia && b.Sfondo == coloreFascia {
			continue
		}
		if stessaFormaSotto(b, img) && !zonaVuota(img, b) {
			out = append(out, b)
		}
	}
	return out
}

// zonaVuota: nella zona della scritta non c'è più disegno (fondo piatto): la scritta è sparita.
func zonaVuota(img *image.RGBA, b Blocco) bool {
	r := rettBlocco(b).Intersect(img.Bounds())
	if r.Dx() < 2 || r.Dy() < 2 {
		return true
	}
	c0 := img.RGBAAt(r.Min.X, r.Min.Y)
	for y := r.Min.Y; y < r.Max.Y; y += 2 {
		for x := r.Min.X; x < r.Max.X; x += 2 {
			if distanza(img.RGBAAt(x, y), c0) > 40 {
				return false
			}
		}
	}
	return true
}

// Limiti della ricerca di una scritta spostata (banco 26/11: con il raggio calcolato su un
// paragrafo di 5 righe la ricerca durava mezzo secondo per scritta, nel ciclo della pagina).
const (
	limiteSpostY = 48 // pixel in su o in giù
	limiteSpostX = 64 // pixel di lato
)

// budgetSegui: tempo massimo della ricerca per tutte le scritte di un giro (variabile per i test).
var budgetSegui = 25 * time.Millisecond

// spostamento2D: dove si ritrova la scritta b (presa da vecchia) nell'immagine nuova, entro
// qualche riga in su o in giù e qualche em di lato. ok falso se non si ritrova bene o se non
// si è mossa.
func spostamento2D(vecchia, nuova *image.RGBA, b Blocco) (dx, dy int, ok bool) {
	r := rettBlocco(b).Intersect(vecchia.Bounds())
	if r.Dx() < 6 || r.Dy() < 4 {
		return 0, 0, false
	}
	// punti di confronto: al massimo circa 12 x 24, anche per un paragrafo grande
	passoY := max(1, r.Dy()/12)
	passoX := max(2, r.Dx()/24)
	type punto struct {
		x, y int
		c    [3]int
	}
	var pt []punto
	for y := r.Min.Y; y < r.Max.Y; y += passoY {
		for x := r.Min.X; x < r.Max.X; x += passoX {
			c := vecchia.RGBAAt(x, y)
			pt = append(pt, punto{x, y, [3]int{int(c.R), int(c.G), int(c.B)}})
		}
	}
	// la scritta deve avere disegno, se no si "ritrova" ovunque su un fondo piatto
	piatta := true
	for _, p := range pt[1:] {
		if abs(p.c[0]-pt[0].c[0])+abs(p.c[1]-pt[0].c[1])+abs(p.c[2]-pt[0].c[2]) > 40 {
			piatta = false
			break
		}
	}
	if piatta {
		return 0, 0, false
	}
	nb := nuova.Bounds()
	// media: differenza media dei punti spostati di (dx, dy). Smette appena supera tetto (non
	// può più essere la migliore): la ricerca resta veloce anche su un paragrafo grande.
	media := func(dx, dy, tetto int) int {
		tot, lim := 0, tetto*len(pt)
		for _, p := range pt {
			x, y := p.x+dx, p.y+dy
			if x < nb.Min.X || x >= nb.Max.X || y < nb.Min.Y || y >= nb.Max.Y {
				return 1 << 20
			}
			c := nuova.RGBAAt(x, y)
			tot += abs(int(c.R)-p.c[0]) + abs(int(c.G)-p.c[1]) + abs(int(c.B)-p.c[2])
			if tot > lim {
				return 1 << 20
			}
		}
		return tot / len(pt)
	}
	// raggio dall'altezza di UNA riga (non del paragrafo intero), con un tetto: oltre, è
	// un'altra lettura a rimettere la traduzione al posto giusto
	h := max(1, b.H/max(1, b.Righe))
	limY := min(max(4*h, 24), limiteSpostY)
	limX := min(max(3*h, 32), limiteSpostX)
	d0 := media(0, 0, 1<<12)
	meglio, mx, my := 1<<20, 0, 0
	for y := -limY; y <= limY; y += 2 { // prima a passi di 2, poi di fino attorno al migliore
		for x := -limX; x <= limX; x += 2 {
			if m := media(x, y, min(meglio, 37)); m < meglio {
				meglio, mx, my = m, x, y
			}
		}
	}
	cx, cy := mx, my
	for y := cy - 1; y <= cy+1; y++ {
		for x := cx - 1; x <= cx+1; x++ {
			if m := media(x, y, meglio); m < meglio {
				meglio, mx, my = m, x, y
			}
		}
	}
	if (mx == 0 && my == 0) || meglio > 36 || meglio*3 >= d0 {
		return 0, 0, false
	}
	return mx, my, true
}

// seguiScritte: le scritte mostrate che si sono spostate (la pagina scorre, la visuale si
// sposta) si ritrovano nell'immagine nuova e la traduzione le segue: stesso testo, stessa
// traduzione, nel posto nuovo. Le altre restano come sono (decidono togliCambiate e la
// rilettura). Restituisce le scritte e quante ne ha spostate.
func seguiScritte(img *image.RGBA, bl []Blocco) ([]Blocco, int) {
	if img == nil || len(bl) == 0 {
		return bl, 0
	}
	out := append([]Blocco(nil), bl...)
	var mosse []int
	inizio := time.Now()
	for i, b := range out {
		if time.Since(inizio) > budgetSegui {
			break // tempo del giro finito: le altre le rimette a posto la lettura dopo
		}
		if b.Img == nil || b.Img.Bounds() != img.Bounds() || b.Tradotto == "" ||
			(b.Inchiostro == coloreTestoFascia && b.Sfondo == coloreFascia) {
			continue
		}
		if !zonaCambiata(b.Img, img, b) {
			continue // niente è cambiato lì: non si è mossa
		}
		dx, dy, ok := spostamento2D(b.Img, img, b)
		if !ok {
			continue
		}
		n := b
		n.X, n.Y = b.X+dx, b.Y+dy
		if !rettBlocco(n).In(img.Bounds()) {
			continue
		}
		if p := b.parole(); p != nil {
			sp := make([]ParolaOCR, len(p))
			for k, w := range p {
				w.X += float64(dx)
				w.Y += float64(dy)
				sp[k] = w
			}
			n.Pos = &sp
		}
		if n.IconaDx > 0 {
			n.IconaDx += dx
		}
		if n.IconaSx > 0 {
			n.IconaSx += dx
		}
		out[i] = n
		mosse = append(mosse, i)
	}
	if len(mosse) == 0 {
		return bl, 0
	}
	// le scritte spostate prendono l'immagine nuova, con le loro lettere inglesi cancellate lì
	var sp []Blocco
	for _, i := range mosse {
		out[i].Img = img
		sp = append(sp, out[i])
	}
	pul := pulisciPagina(img, sp)
	for _, i := range mosse {
		out[i].FondoX, _ = margineFondo(out[i])
		out[i].Fondo = fondoRicostruito(img, out[i])
		out[i].Pulita = pul
		out[i].Tinta = tintaBlocco(img, out[i])
	}
	return out, len(mosse)
}
