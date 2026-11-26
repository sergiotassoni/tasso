package traduce

// Scritte dell'interfaccia o scritte del mondo (tutti i giochi). Regola del proprietario: si
// traduce quello che aiuta il giocatore a capire cosa fare (missioni, sottotitoli, menu,
// impostazioni, descrizioni, suggerimenti, diario). Le scritte che fanno parte della scena 3D
// (insegne, cartelli su barche e muri, poster, scritte sui vestiti e sui container, nomi dei
// luoghi sulla mappa) restano in inglese: tradotte e disegnate sopra la scena coprirebbero il
// gioco e si muovono con la telecamera.
//
// Nessun segno da solo basta: sommo più segni in un punteggio e, nel dubbio, NON disegno sopra
// la scena (ma sottotitoli e menu, che hanno segni forti dell'interfaccia, restano tradotti).
//
// Qui ci sono anche le altre regole universali nate dal banco sui video (25/11/2026):
//   - icone dei tasti lette come lettere ("Swing Sword O", "Back O"): l'icona resta del gioco;
//   - letture spazzatura ("T wig h T", "Carry;" letto da un nome, "CORK!e");
//   - mai due traduzioni disegnate una sopra l'altra.

import (
	"image"
	"image/color"
	"math"
	"strings"
	"sync"
	"time"
	"unicode"
)

// verbiTasto: comandi che da soli non chiudono mai una riga: dopo viene l'icona del tasto e il
// resto della frase ("Press [X] to reveal the HUD", "Hold [A] while steering…").
var verbiTasto = map[string]bool{"press": true, "hold": true, "tap": true, "click": true, "use": true, "push": true}

// segnoIcona: una "parola" di un solo segno che non è punteggiatura: l'OCR legge così le icone dei
// tasti e dei pulsanti (•, ●, ⊗, @, ©…).
func segnoIcona(w string) bool {
	r := []rune(strings.TrimSpace(w))
	if len(r) != 1 || unicode.IsLetter(r[0]) || unicode.IsDigit(r[0]) {
		return false
	}
	return !strings.ContainsRune("-–—\"'“”‘’()[],.;:!?/&%+#$*<>«»›‹|=_~", r[0])
}

// lettereIcona: lettere e sigle che sono nomi di pulsanti del controller o tasti letti dall'icona
// accanto a un comando ("Swing Sword O", "Back B", "LB Previous").
var lettereIcona = map[string]bool{"O": true, "X": true, "A": true, "B": true, "Y": true, "E": true, "F": true,
	"Q": true, "R": true, "LB": true, "RB": true, "LT": true, "RT": true, "L1": true, "R1": true, "L2": true,
	"R2": true, "L3": true, "R3": true, "LS": true, "RS": true}

// lettereIconaSicure: senza le posizioni delle parole tolgo solo queste (A, E, F, Q, R possono
// essere parole o tasti nominati nella frase).
var lettereIconaSicure = map[string]bool{"O": true, "X": true, "B": true, "Y": true, "LB": true, "RB": true,
	"LT": true, "RT": true, "L1": true, "R1": true, "L2": true, "R2": true}

// tokenIcona: la parola è l'icona di un tasto (lettera del controller o segno). sicuro: senza
// posizioni (non so se c'è lo spazio di un'icona attorno).
func tokenIcona(w string, sicuro bool) bool {
	t := strings.TrimSpace(w)
	if segnoIcona(t) {
		return true
	}
	if sicuro {
		return lettereIconaSicure[t]
	}
	return lettereIcona[t]
}

// togliIconeTasti: un comando con l'icona del pulsante prima o dopo ("Swing Sword O", "Back O",
// "• Interact"): l'icona letta come lettera non si traduce né si copre, resta quella del gioco.
// Tolgo la parola dell'icona (e il suo riquadro) se: è in testa o in fondo, il resto è un comando
// corto (1-5 parole) senza punteggiatura da frase, prima non c'è un verbo come "Press" (lì la
// lettera è il tasto nominato nella frase: "Press A") e, se so le posizioni, l'icona è staccata
// dalla parola vicina come un'icona (almeno un terzo di altezza).
func togliIconeTasti(b []Blocco) []Blocco {
	out := make([]Blocco, 0, len(b))
	for _, bl := range b {
		out = append(out, senzaIcone(bl))
	}
	return out
}

func senzaIcone(bl Blocco) Blocco {
	if max(1, bl.Righe) != 1 {
		return bl
	}
	campi := strings.Fields(bl.Testo)
	if len(campi) < 2 || len(campi) > 6 {
		return bl
	}
	p := bl.parole()
	conPos := len(p) == len(campi)
	hMed := float64(bl.H)
	if conPos {
		hs := 0.0
		for _, w := range p {
			hs += w.H
		}
		hMed = hs / float64(len(p))
	}
	staccata := func(i, j int) bool { // parola i (icona) e la vicina j
		if !conPos {
			return true
		}
		a, c := p[min(i, j)], p[max(i, j)]
		return c.X-(a.X+a.W) >= hMed/3
	}
	resto := func(r []string) bool {
		if len(r) == 0 || len(r) > 5 {
			return false
		}
		t := strings.Join(r, " ")
		if strings.ContainsAny(t, ".!?,;:") {
			return false
		}
		lettere := 0
		for _, w := range r {
			if len([]rune(strings.Trim(w, "\"'()"))) >= 2 {
				lettere++
			}
		}
		return lettere > 0
	}
	da, a := 0, len(campi)
	n := len(campi)
	if tokenIcona(campi[n-1], !conPos) && staccata(n-1, n-2) {
		prima := strings.ToLower(strings.Trim(campi[n-2], ".,:;"))
		if !verbiTasto[prima] && prima != "key" && prima != "button" && prima != "or" && prima != "and" && prima != "+" {
			a = n - 1
		}
	}
	if a-da >= 2 && campi[0] != "A" && tokenIcona(campi[0], !conPos) && staccata(0, 1) && inizioVoce(campi[1]) {
		da = 1
	}
	if da == 0 && a == n || !resto(campi[da:a]) {
		return bl
	}
	if !conPos {
		bl.Testo = strings.Join(campi[da:a], " ")
		bl.Grezzo = bl.Testo
		return bl
	}
	n2 := bloccoDaPosizioni(p[da:a], bl)
	if a < n { // l'icona tolta a destra: la traduzione non deve andarci sopra
		n2.IconaDx = int(p[a].X)
	}
	if da > 0 {
		n2.IconaSx = int(p[da-1].X + p[da-1].W + 0.5)
	}
	return n2
}

// letturaSpazzatura: righe che sono pezzi di lettere e simboli, non testo: più lettere sole che
// parole vere ("T wig h T" → "T parrucca h T"), una parola sola chiusa dal punto e virgola (un
// nome letto male: "Garry" → "Carry;"). "" = niente da dire.
func letturaSpazzatura(s string) string {
	campi := strings.Fields(s)
	if len(campi) == 1 {
		w := campi[0]
		if strings.HasSuffix(w, ";") && len([]rune(strings.TrimRight(w, ";"))) >= 2 {
			return "non è testo da tradurre"
		}
		return ""
	}
	sole, vere := 0, 0
	tasto := false
	for _, c := range campi {
		l := strings.ToLower(strings.Trim(c, ".,;:!?\"'()[]"))
		if verbiTasto[l] || l == "key" || l == "button" {
			tasto = true
		}
		r := []rune(l)
		switch {
		case len(r) == 1 && unicode.IsLetter(r[0]) && l != "a" && l != "i":
			sole++
		case len(r) >= 2 && esiste(l):
			vere++
		}
	}
	if !tasto && sole >= 2 && sole >= vere {
		return "non è testo da tradurre"
	}
	// lettere attaccate dopo un punto esclamativo dentro una parola maiuscola ("CORK!e") o due
	// icone di fila in testa ("••FORCED"): la riga è letta male
	for _, c := range campi {
		r := []rune(c)
		for i := 1; i+1 < len(r); i++ {
			if (r[i] == '!' || r[i] == '?') && unicode.IsUpper(r[i-1]) && unicode.IsLower(r[i+1]) {
				return "lettura incerta"
			}
		}
	}
	if r := []rune(campi[0]); len(r) >= 3 && segnoIcona(string(r[0])) && segnoIcona(string(r[1])) {
		return "lettura incerta"
	}
	return ""
}

// ---------------------------------------------------------------- interfaccia o mondo

// postoScritta: dove era una scritta in un fotogramma passato (per vedere se si muove con la
// telecamera).
type postoScritta struct {
	testo  string
	rett   image.Rectangle
	quando time.Time
}

// mossa: la stessa scritta (stesso testo, letto quasi uguale) era pochi secondi fa in un altro
// posto dello schermo, spostata di lato o cresciuta/rimpicciolita: si muove con la telecamera,
// è una scritta del mondo. Lo scorrimento solo in verticale con la stessa grandezza è una lista
// che scorre (menu): non conta. Registra anche il posto di adesso.
func (v *votazione) mossa(b Blocco, adesso time.Time) bool {
	if v == nil || len([]rune(lettereConfuse(b.Testo))) < 3 {
		return false
	}
	r := image.Rect(b.X, b.Y, b.X+b.W, b.Y+b.H)
	v.mu.Lock()
	defer v.mu.Unlock()
	tieni := v.posti[:0]
	for _, p := range v.posti {
		if adesso.Sub(p.quando) <= 6*time.Second {
			tieni = append(tieni, p)
		}
	}
	v.posti = tieni
	visti, ferma := 0, false
	var prima *postoScritta // la lettura più vecchia della stessa scritta nei 6 secondi
	for i := range v.posti {
		p := v.posti[i]
		if !p.quando.Before(adesso) || !stessaScritta(p.testo, b.Testo) {
			continue
		}
		if prima == nil {
			prima = &v.posti[i]
		}
		visti++
		h := max(r.Dy(), p.rett.Dy())
		dx := abs((r.Min.X + r.Max.X - p.rett.Min.X - p.rett.Max.X) / 2)
		dy := abs((r.Min.Y + r.Max.Y - p.rett.Min.Y - p.rett.Max.Y) / 2)
		dimensione := abs(r.Dy()-p.rett.Dy())*5 > h || abs(r.Dx()-p.rett.Dx())*4 > max(r.Dx(), p.rett.Dx())
		lato := dx > max(h/2, 6)
		// centro fermo (entro mezza riga): il riquadro un po' più largo o più alto è l'OCR che
		// legge le lettere in modo un po' diverso, non la telecamera che si avvicina (banco del
		// mattino 26/11: "Close Tutorial", "TUTORIAL: SPELLS" e la firma del canale, fermi, presi
		// per scritte del mondo a fotogrammi alterni)
		tol := max(h/2, 6)
		// (cresciuta poco: un'insegna che si avvicina cresce di molto, l'OCR sbaglia di poco)
		pocoDiversa := max(r.Dy(), p.rett.Dy())*5 <= min(r.Dy(), p.rett.Dy())*7 &&
			max(r.Dx(), p.rett.Dx())*2 <= min(r.Dx(), p.rett.Dx())*3
		centroFermo := pocoDiversa && dx <= tol && dy <= tol
		// o un angolo fermo: stesso bordo a sinistra (o a destra) e stesso bordo in alto (o in
		// basso); cambia solo l'altro lato, dove l'OCR ha preso o perso un segno
		angoloFermo := pocoDiversa && (abs(r.Min.X-p.rett.Min.X) <= tol || abs(r.Max.X-p.rett.Max.X) <= tol) &&
			(abs(r.Min.Y-p.rett.Min.Y) <= tol || abs(r.Max.Y-p.rett.Max.Y) <= tol)
		if centroFermo || angoloFermo || (!dimensione && (!lato || dy > 3*dx)) {
			ferma = true // nello stesso posto (o lista che scorre in verticale)
		}
	}
	// crescita contata dalla prima lettura, non solo dall'ultima: un'insegna che si avvicina
	// cresce di un 30% a ogni lettura (ogni passo sembra "l'OCR che sbaglia di poco"), ma in
	// qualche secondo è diventata molto più grande (collaudo 02b, regressione di f9b1c22)
	if prima != nil && ferma {
		q := prima.rett
		cresciuta := max(r.Dy(), q.Dy())*5 > min(r.Dy(), q.Dy())*7 || max(r.Dx(), q.Dx())*2 > min(r.Dx(), q.Dx())*3
		if cresciuta {
			ferma = false
		}
	}
	if len(v.posti) >= 300 {
		v.posti = v.posti[1:]
	}
	v.posti = append(v.posti, postoScritta{testo: b.Testo, rett: r, quando: adesso})
	return visti > 0 && !ferma
}

// luceImg: luminosità di un pixel (0-255).
func luceImg(img *image.RGBA, x, y int) int {
	i := img.PixOffset(x, y)
	return (299*int(img.Pix[i]) + 587*int(img.Pix[i+1]) + 114*int(img.Pix[i+2])) / 1000
}

// trama: quanto è "mossa" la zona attorno alla scritta (fuori dalle lettere): media della
// differenza di luce tra pixel vicini sull'anello attorno al riquadro. Pannello piatto ≈ 0-4,
// scena 3D con dettagli molto di più.
func trama(img *image.RGBA, b Blocco) float64 {
	m := max(3, b.H/2)
	fuori := image.Rect(b.X-m, b.Y-m, b.X+b.W+m, b.Y+b.H+m).Intersect(img.Bounds())
	dentroR := image.Rect(b.X-1, b.Y-1, b.X+b.W+1, b.Y+b.H+1)
	if fuori.Dx() < 3 || fuori.Dy() < 3 {
		return 0
	}
	passo := max(1, int(math.Sqrt(float64(fuori.Dx()*fuori.Dy())/6000)))
	tot, n := 0, 0
	for y := fuori.Min.Y; y < fuori.Max.Y-1; y += passo {
		for x := fuori.Min.X; x < fuori.Max.X-1; x += passo {
			if (image.Point{x, y}).In(dentroR) || (image.Point{x + 1, y + 1}).In(dentroR) {
				continue
			}
			l := luceImg(img, x, y)
			tot += abs(l-luceImg(img, x+1, y)) + abs(l-luceImg(img, x, y+1))
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return float64(tot) / float64(n)
}

// pendenza: la scritta è inclinata (prospettiva, cartello storto)? Confronto l'altezza del
// centro dell'inchiostro nel terzo sinistro e nel terzo destro della riga: nelle scritte dritte
// è quasi uguale. Restituisce |dy/dx| (0 = dritta).
func pendenza(img *image.RGBA, b Blocco) float64 {
	if max(1, b.Righe) != 1 || b.W < 4*b.H || b.H < 6 {
		return 0
	}
	if b.Inchiostro == b.Sfondo {
		coloriBlocco(img, &b)
	}
	m := b.H / 2
	centro := func(x0, x1 int) (float64, bool) {
		sy, n := 0, 0
		for x := x0; x < x1; x++ {
			for y := b.Y - m; y < b.Y+b.H+m; y++ {
				if !(image.Point{x, y}).In(img.Bounds()) {
					continue
				}
				if eInchiostro(img.RGBAAt(x, y), &b) {
					sy += y
					n++
				}
			}
		}
		if n < b.H {
			return 0, false
		}
		return float64(sy) / float64(n), true
	}
	t := b.W / 3
	ys, ok1 := centro(b.X, b.X+t)
	yd, ok2 := centro(b.X+b.W-t, b.X+b.W)
	if !ok1 || !ok2 {
		return 0
	}
	return math.Abs(yd-ys) / float64(b.W-t)
}

// paroleFrase: la scritta è una frase (sottotitolo, descrizione, suggerimento): almeno 4 parole
// con lettere minuscole, o finisce con . ! ?
func paroleFrase(t string) bool {
	t = strings.TrimSpace(t)
	if t == "" {
		return false
	}
	w := strings.Fields(t)
	if len(w) >= 4 && strings.ContainsFunc(t, unicode.IsLower) {
		return true
	}
	return len(w) >= 2 && strings.ContainsAny(t[len(t)-1:], ".!?")
}

// etichettaCorta: 1-3 parole senza punteggiatura da frase (nome, insegna, voce).
func etichettaCorta(t string) bool {
	w := strings.Fields(t)
	return len(w) >= 1 && len(w) <= 3 && !strings.ContainsAny(t, ".!?,;") && strings.ContainsFunc(t, unicode.IsLetter)
}

// allineata: almeno altre due scritte sono in colonna con b (stesso inizio a sinistra o stesso
// centro, non lontane in verticale) o sulla stessa riga con la stessa altezza: le voci di un menu,
// una fila di schede, un elenco. Le insegne del mondo stanno da sole o in coppia (due righe dello
// stesso cartello).
func allineata(b Blocco, tutte []Blocco) bool {
	h := max(b.H/max(1, b.Righe), 4)
	compagne := 0
	for _, s := range tutte {
		if s.X == b.X && s.Y == b.Y && s.W == b.W && s.H == b.H {
			continue
		}
		hs := max(s.H/max(1, s.Righe), 4)
		if float64(max(h, hs)) > 1.6*float64(min(h, hs)) {
			continue
		}
		dy := abs((s.Y + s.H/2) - (b.Y + b.H/2))
		colonna := (abs(s.X-b.X) <= h/2 || abs((s.X+s.W/2)-(b.X+b.W/2)) <= h/2) && dy <= 5*h
		riga := dy <= h/3 && abs(s.X-b.X) <= 25*h
		if colonna || riga {
			compagne++
		}
	}
	return compagne >= 2
}

// isolata: nessun'altra scritta entro 4 righe in nessuna direzione.
func isolata(b Blocco, tutte []Blocco) bool {
	h := max(b.H/max(1, b.Righe), 4)
	r := image.Rect(b.X-4*h, b.Y-4*h, b.X+b.W+4*h, b.Y+b.H+4*h)
	for _, s := range tutte {
		if s.X == b.X && s.Y == b.Y && s.W == b.W && s.H == b.H {
			continue
		}
		if r.Overlaps(image.Rect(s.X, s.Y, s.X+s.W, s.Y+s.H)) {
			return false
		}
	}
	return true
}

// targaNome: un'etichetta corta tutta maiuscola sopra o sotto un codice ("LURE PREDATOR" sopra
// "L35P"): è il nome di un veicolo, di un'arma, di un prodotto. Resta com'è.
func targaNome(b Blocco, tutte []Blocco) bool {
	if !tuttoMaiuscolo(b.Testo) || !etichettaCorta(b.Testo) || b.Affid == letturaCatalog {
		return false
	}
	h := max(b.H, 4)
	for _, s := range tutte {
		t := strings.TrimSpace(s.Testo)
		if strings.ContainsAny(t, " ") || len([]rune(t)) < 2 || len([]rune(t)) > 8 {
			continue
		}
		if !strings.ContainsFunc(t, unicode.IsDigit) || !strings.ContainsFunc(t, unicode.IsLetter) {
			continue
		}
		dy := min(abs(s.Y-(b.Y+b.H)), abs(b.Y-(s.Y+s.H)))
		if dy <= 2*h && abs((s.X+s.W/2)-(b.X+b.W/2)) <= max(b.W, s.W)/2+h {
			return true
		}
	}
	return false
}

// etichettaSparsa: una scritta corta da sola su uno sfondo pieno di dettagli (nome di un luogo
// sulla mappa, insegna): né frase, né frase pronta, né in colonna o in fila con altre.
func etichettaSparsa(img *image.RGBA, b Blocco, tutte []Blocco) bool {
	return max(1, b.Righe) == 1 && etichettaCorta(b.Testo) && b.Affid != letturaCatalog &&
		!allineata(b, tutte) && trama(img, b) >= sogliaTramaScena
}

const (
	sogliaTramaPiatta = 5.0  // sotto: pannello piatto dell'interfaccia
	sogliaTramaScena  = 12.0 // sopra: scena 3D, mappa, foto
)

// contaSparse: quante scritte con lettere ha la schermata e quante sono etichette sparse (una
// volta sola per schermata: le scritte di una lettura si controllano tutte con le stesse "tutte").
func contaSparse(img *image.RGBA, tutte []Blocco) (int, int) {
	k := chiaveSparse{img: img, n: len(tutte)}
	for _, s := range tutte {
		k.somma += s.X*31 + s.Y*17 + s.W*7 + s.H
	}
	muSparse.Lock()
	if v, ok := cacheSparse[k]; ok {
		muSparse.Unlock()
		return v[0], v[1]
	}
	muSparse.Unlock()
	conLettere, sparse := 0, 0
	for _, s := range tutte {
		if strings.ContainsFunc(s.Testo, unicode.IsLetter) {
			conLettere++
			if etichettaSparsa(img, s, tutte) {
				sparse++
			}
		}
	}
	muSparse.Lock()
	if len(cacheSparse) >= 4 { // poche: la chiave tiene viva la cattura
		cacheSparse = map[chiaveSparse][2]int{}
	}
	cacheSparse[k] = [2]int{conLettere, sparse}
	muSparse.Unlock()
	return conLettere, sparse
}

type chiaveSparse struct {
	img      *image.RGBA
	n, somma int
}

var (
	muSparse    sync.Mutex
	cacheSparse = map[chiaveSparse][2]int{}
)

// punteggioMondo: più è alto, più la scritta sembra parte della scena (insegna, cartello, nome
// sulla mappa); negativo = interfaccia. motivi: i segni trovati (per il controllo a mano).
func punteggioMondo(img *image.RGBA, b Blocco, tutte []Blocco) (int, []string) {
	if img == nil {
		return 0, nil
	}
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	p := 0
	var motivi []string
	segna := func(n int, m string) {
		p += n
		motivi = append(motivi, m)
	}
	etichettaOggetto := tuttoMaiuscolo(b.Testo) && etichettaCorta(b.Testo) && !strings.ContainsAny(b.Testo, ".!?,") && b.Affid != letturaCatalog
	if b.Mossa {
		segna(3, "si muove con la scena")
	}
	if pd := pendenza(img, b); pd > 0.04 {
		segna(3, "storta") // l'interfaccia è sempre dritta: una riga inclinata è nella scena
	}
	switch t := trama(img, b); {
	case t >= sogliaTramaScena:
		segna(1, "sfondo con dettagli")
	case t < sogliaTramaPiatta:
		segna(-1, "pannello piatto")
	}
	if b.Inchiostro == b.Sfondo {
		coloriBlocco(img, &b)
	}
	if luce(b.Inchiostro) < 90 && luce(b.Sfondo) > 150 && trama(img, b) >= sogliaTramaPiatta {
		segna(1, "lettere scure su un cartello") // l'interfaccia sopra la scena ha lettere chiare
	}
	if etichettaOggetto && sfondoColorato(b.Sfondo) {
		// "TAKE" su un cassetto arancione: i pannelli dell'interfaccia sono scuri o grigi, una
		// parola maiuscola su un fondo molto colorato è stampata su un oggetto della scena
		segna(1, "scritta su un oggetto colorato")
	}
	// le bande nere sopra e sotto (video, filmati) non sono lo schermo del gioco
	g := areaGioco(img)
	W, H = g.Dx(), g.Dy()
	cx, cy := b.X+b.W/2-g.Min.X, b.Y+b.H/2-g.Min.Y
	if cy*100 > H*72 && abs(cx-W/2)*100 < W*20 && !etichettaOggetto {
		// ("DO NOT TAKE" stampato su un cassetto in basso al centro non è un sottotitolo)
		if len(strings.Fields(b.Testo)) >= 3 || paroleFrase(b.Testo) {
			segna(-3, "zona dei sottotitoli")
		} else {
			segna(-1, "in basso al centro")
		}
	}
	if bordoSchermo(b, g) {
		segna(-1, "bordo dello schermo")
	}
	if b.Affid == letturaCatalog {
		segna(-2, "frase dell'interfaccia")
	}
	if paroleFrase(b.Testo) {
		segna(-2, "frase")
	} else if tuttoMaiuscolo(b.Testo) && etichettaCorta(b.Testo) && b.Affid != letturaCatalog {
		segna(1, "etichetta maiuscola")
	}
	if allineata(b, tutte) {
		segna(-1, "in fila con altre scritte")
	} else if isolata(b, tutte) {
		segna(1, "da sola")
	}
	conLettere, sparse := contaSparse(img, tutte)
	if conLettere >= 8 && sparse < 4 {
		segna(-1, "schermata piena di scritte")
	}
	if sparse >= 4 && etichettaSparsa(img, b, tutte) {
		segna(2, "nome sparso (mappa)")
	}
	if H >= 500 && b.H/max(1, b.Righe)*1000 < H*13 && !bordoSchermo(b, g) {
		segna(1, "scritta piccola nella scena")
	}
	return p, motivi
}

// bordoSchermo: la scritta tocca la fascia dei bordi (7% ai lati, 8% sopra e sotto), dove
// stanno HUD, schede e comandi.
func bordoSchermo(b Blocco, g image.Rectangle) bool {
	W, H := g.Dx(), g.Dy()
	x, y := b.X-g.Min.X, b.Y-g.Min.Y
	return x*100 < W*7 || (x+b.W)*100 > W*93 || y*100 < H*8 || (y+b.H)*100 > H*92
}

// areaGioco: lo schermo senza le bande nere sopra e sotto (righe tutte scure e uniformi), come
// nei video e nei filmati in formato cinema.
func areaGioco(img *image.RGBA) image.Rectangle {
	r := img.Bounds()
	scura := func(y int) bool {
		passo := max(1, r.Dx()/200)
		for x := r.Min.X; x < r.Max.X; x += passo {
			if luceImg(img, x, y) > 28 {
				return false
			}
		}
		return true
	}
	y0, y1 := r.Min.Y, r.Max.Y
	for y0 < r.Min.Y+r.Dy()/5 && scura(y0) {
		y0++
	}
	for y1 > r.Max.Y-r.Dy()/5 && scura(y1-1) {
		y1--
	}
	if y0 == r.Min.Y+r.Dy()/5 || y1 == r.Max.Y-r.Dy()/5 || y1-y0 < r.Dy()/2 {
		return r // tutto scuro (schermata nera, menu scuro): niente bande
	}
	return image.Rect(r.Min.X, y0, r.Max.X, y1)
}

// motivoMondo: la scritta resta in inglese perché fa parte della scena (o è un nome su una
// targa). "" = è interfaccia, si traduce.
func motivoMondo(img *image.RGBA, b Blocco, tutte []Blocco) string {
	if img == nil {
		return ""
	}
	if targaNome(b, tutte) {
		return "nome di un veicolo o di un oggetto"
	}
	if firmaNellAngolo(img, b, tutte) {
		return motivoFirmaAngolo
	}
	if p, m := punteggioMondo(img, b, tutte); p >= 3 {
		return "scritta del mondo (" + strings.Join(m, ", ") + ")"
	}
	return ""
}

// ---------------------------------------------------------------- traduzioni sovrapposte

// senzaSovrapposte: mai due traduzioni disegnate una sopra l'altra. Due scritte pronte i cui
// riquadri si coprono (più di un quinto della più piccola) sono la stessa scritta letta in due
// modi o pezzi uniti male: restano in inglese tutte e due (mai mezzo riquadro tradotto).
func senzaSovrapposte(pronti []Blocco) []Blocco {
	via := make([]bool, len(pronti))
	for i := range pronti {
		a := image.Rect(pronti[i].X, pronti[i].Y, pronti[i].X+pronti[i].W, pronti[i].Y+pronti[i].H)
		for j := i + 1; j < len(pronti); j++ {
			c := image.Rect(pronti[j].X, pronti[j].Y, pronti[j].X+pronti[j].W, pronti[j].Y+pronti[j].H)
			in := a.Intersect(c)
			if in.Empty() {
				continue
			}
			if in.Dx()*in.Dy()*5 >= min(a.Dx()*a.Dy(), c.Dx()*c.Dy()) {
				// una delle due è già tradotta e confermata (stessa scritta, decisione tenuta):
				// resta quella e va via solo l'altra lettura (niente salti italiano/inglese)
				switch {
				case pronti[i].GiaMostrata && !pronti[j].GiaMostrata:
					via[j] = true
				case pronti[j].GiaMostrata && !pronti[i].GiaMostrata:
					via[i] = true
				default:
					via[i], via[j] = true, true
				}
			}
		}
	}
	out := pronti[:0:0]
	for i, b := range pronti {
		if !via[i] {
			out = append(out, b)
		}
	}
	return out
}

// copertaDaTenuta: p, tolta da senzaSovrapposte, sta sotto una traduzione tenuta (un'altra
// lettura della stessa scritta già tradotta e confermata): non conta come pezzo in inglese.
func copertaDaTenuta(p Blocco, tenute []Blocco) bool {
	a := image.Rect(p.X, p.Y, p.X+p.W, p.Y+p.H)
	for _, t := range tenute {
		if !t.GiaMostrata {
			continue
		}
		c := image.Rect(t.X, t.Y, t.X+t.W, t.Y+t.H)
		in := a.Intersect(c)
		if !in.Empty() && in.Dx()*in.Dy()*5 >= min(a.Dx()*a.Dy(), c.Dx()*c.Dy()) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- righe sopra e sotto

// fineFraseTesto: la scritta finisce con . ! ? … (anche dentro virgolette o parentesi).
func fineFraseTesto(t string) bool {
	t = strings.TrimRight(strings.TrimSpace(t), "\"'”’»)]")
	return t != "" && strings.ContainsAny(t[len(t)-1:], ".!?") || strings.HasSuffix(t, "…")
}

// inchiostroDiRiga: nella fascia ci sono lettere del colore della scritta b (una riga di testo
// non letta): abbastanza colonne con inchiostro, alto come una riga, non una macchia piena.
func inchiostroDiRiga(img *image.RGBA, b Blocco, fascia image.Rectangle) bool {
	fascia = fascia.Intersect(img.Bounds())
	if fascia.Dx() < 8 || fascia.Dy() < 4 || distanza(b.Inchiostro, b.Sfondo) < 120 {
		return false
	}
	colonne, tot := 0, 0
	righe := make([]bool, fascia.Dy())
	for x := fascia.Min.X; x < fascia.Max.X; x++ {
		c := false
		for y := fascia.Min.Y; y < fascia.Max.Y; y++ {
			p := img.RGBAAt(x, y)
			if distanza(p, b.Inchiostro) < 70 && distanza(p, b.Sfondo) > 90 {
				c = true
				tot++
				righe[y-fascia.Min.Y] = true
			}
		}
		if c {
			colonne++
		}
	}
	nr := 0
	for _, r := range righe {
		if r {
			nr++
		}
	}
	area := fascia.Dx() * fascia.Dy()
	h := max(b.H/max(1, b.Righe), 4)
	return colonne*100 >= fascia.Dx()*12 && nr*100 >= h*35 && tot*100 >= area*3 && tot*100 <= area*50
}

// scrittaNellaFascia: una scritta letta (diversa da b) tocca la fascia.
func scrittaNellaFascia(fascia image.Rectangle, tutte []Blocco, b Blocco) bool {
	for _, s := range tutte {
		if s.X == b.X && s.Y == b.Y && s.W == b.W && s.H == b.H {
			continue
		}
		if fascia.Overlaps(image.Rect(s.X, s.Y, s.X+s.W, s.Y+s.H)) {
			return true
		}
	}
	return false
}

// rigaVicinaMancante: la scritta è una riga di una frase su più righe e l'altra riga non è
// stata letta (o non è stata unita): tradotta da sola si vedrebbe mezza frase in italiano sopra
// o sotto l'inglese ("Hold while steering to perform" con "a sharper turn." sotto non letta;
// "contact you." sotto una prima riga letta male). Due casi:
//   - frase di 4+ parole che non finisce con . ! ? e sotto, a una riga di distanza, ci sono
//     lettere dello stesso colore non lette;
//   - riga che comincia minuscola (seguito di una frase) con una scritta o lettere subito sopra.
func rigaVicinaMancante(img *image.RGBA, b Blocco, tutte []Blocco) string {
	if img == nil || max(1, b.Righe) != 1 || b.Affid == letturaCatalog {
		return ""
	}
	t := strings.TrimSpace(b.Testo)
	w := strings.Fields(t)
	h := max(b.H, 4)
	if len(w) >= 4 && !fineFraseTesto(t) {
		sotto := image.Rect(b.X, b.Y+b.H+1, b.X+b.W, b.Y+b.H+h*13/10)
		if !scrittaNellaFascia(sotto, tutte, b) && inchiostroDiRiga(img, b, sotto) {
			return "riga incompleta (la riga sotto non è stata letta)"
		}
	}
	if len(w) >= 2 && iniziaMinuscola(t) {
		sopra := image.Rect(b.X-2*b.W, b.Y-h*13/10, b.X+3*b.W, b.Y-1)
		if scrittaNellaFascia(sopra, tutte, b) {
			return "seguito di una riga sopra (tradotto da solo sarebbe mezza frase)"
		}
		if inchiostroDiRiga(img, b, image.Rect(b.X, b.Y-h*13/10, b.X+b.W, b.Y-1)) {
			return "riga incompleta (la riga sopra non è stata letta)"
		}
	}
	return ""
}

// ---------------------------------------------------------------- firma nell'angolo

// motivoFirmaAngolo: il nome del canale o il logo stampato in un angolo dello schermo.
const motivoFirmaAngolo = "scritta del mondo (firma del canale o logo nell'angolo)"

// paroleFunzione: parole che in un nome proprio di un canale o di un logo non ci sono, ma nei
// comandi e nelle frasi sì ("Back to Menu", "Close the Map").
var paroleFunzione = map[string]bool{"to": true, "the": true, "of": true, "a": true, "an": true, "and": true,
	"for": true, "your": true, "my": true, "in": true, "on": true, "at": true, "with": true, "is": true, "or": true}

// firmaNellAngolo: una scritta piccola, da sola, nell'angolo in alto a destra o in basso a destra
// dello schermo, fatta solo di parole con l'iniziale maiuscola ("Horror Games House",
// "MKIceAndFire" no: una parola sola non basta). È la firma del canale del video o il logo di chi
// trasmette: resta in inglese (regola del proprietario: loghi e firme dei canali restano come
// sono). Banco del mattino 26/11: "Horror Games House" tradotta "Casa Giochi Horror" a
// fotogrammi alterni. I pezzi letti a metà davanti ("or Games House") non contano.
func firmaNellAngolo(img *image.RGBA, b Blocco, tutte []Blocco) bool {
	if img == nil || max(1, b.Righe) != 1 || b.Affid == letturaCatalog {
		return false
	}
	g := areaGioco(img)
	W, H := g.Dx(), g.Dy()
	if W <= 0 || H <= 0 {
		return false
	}
	cx, cy := b.X+b.W/2-g.Min.X, b.Y+b.H/2-g.Min.Y
	if cx*100 < W*88 || (cy*100 > H*12 && cy*100 < H*88) || b.H*100 > H*4 {
		return false
	}
	if allineata(b, tutte) {
		return false // in colonna o in fila con altre voci: un elenco del gioco
	}
	parole := strings.Fields(b.Testo)
	for len(parole) > 0 { // pezzo letto a metà davanti: "or", "•or"
		r := []rune(parole[0])
		if unicode.IsUpper(r[0]) {
			break
		}
		parole = parole[1:]
	}
	if len(parole) < 2 || len(parole) > 4 {
		return false
	}
	if verbiObiettivo[strings.ToLower(parole[0])] {
		return false // "Find Marcus", "Reach The Camp": l'obiettivo della missione, non una firma
	}
	if !scrittaTrasparente(img, b) {
		return false // lettere nette (bianche su scuro): interfaccia del gioco ("Abandoned Mine")
	}
	for _, p := range parole {
		r := []rune(p)
		if len(r) < 2 || !unicode.IsUpper(r[0]) || paroleFunzione[strings.ToLower(p)] {
			return false
		}
		for _, c := range r[1:] {
			if !unicode.IsLower(c) {
				return false // tutto maiuscolo ("TAC-COM"), cifre, segni: interfaccia del gioco
			}
		}
	}
	return true
}

// verbiObiettivo: verbi con cui cominciano gli obiettivi delle missioni.
var verbiObiettivo = map[string]bool{"find": true, "reach": true, "locate": true, "follow": true, "talk": true,
	"go": true, "defeat": true, "kill": true, "return": true, "bring": true, "collect": true, "search": true,
	"escape": true, "survive": true, "protect": true, "meet": true, "investigate": true, "explore": true,
	"destroy": true, "open": true, "get": true, "head": true, "speak": true, "use": true, "deliver": true}

// scrittaTrasparente: la firma del canale è stampata semitrasparente sopra il video: dentro il
// riquadro la luce cambia poco tra lettere e sfondo (le scritte dell'interfaccia del gioco sono
// nette, quasi bianche su scuro). Guarda la differenza tra i pixel più chiari e più scuri (5% e
// 95%) del riquadro.
func scrittaTrasparente(img *image.RGBA, b Blocco) bool {
	r := image.Rect(b.X, b.Y, b.X+b.W, b.Y+b.H).Intersect(img.Bounds())
	if r.Dx() < 2 || r.Dy() < 2 {
		return false
	}
	var conta [256]int
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			conta[luceImg(img, x, y)]++
			n++
		}
	}
	basso, alto, acc := -1, -1, 0
	for v := 0; v < 256; v++ {
		acc += conta[v]
		if basso < 0 && acc*20 >= n {
			basso = v
		}
		if alto < 0 && acc*20 >= n*19 {
			alto = v
		}
	}
	return alto-basso <= 130
}

// sfondoColorato: colore acceso e non scuro (arancione, rosso, verde pieno): saturazione alta.
func sfondoColorato(c color.RGBA) bool {
	mx := max(int(c.R), int(c.G), int(c.B))
	mn := min(int(c.R), int(c.G), int(c.B))
	return mx >= 120 && mx-mn >= mx*6/10
}
