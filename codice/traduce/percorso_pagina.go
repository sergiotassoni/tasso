package traduce

// Il percorso della traduzione di tutta la schermata, senza le parti di Windows: dalle righe
// lette dall'OCR alle scritte pronte da disegnare. Lo usano la traduzione vera (F7), il banco
// di prova (--prova-traduzione) e i test su Linux, così si prova davvero quello che gira nel gioco.

import (
	"context"
	"image"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// infoRiga: cosa è successo a una scritta (per il banco di prova e il controllo a mano).
type infoRiga struct {
	Grezzo, Corretto string
	Affid            int
	Tradotto, Fonte  string
	Ms               int64
	Scartata         string // perché non si mostra ("" = si mostra)
	X, Y, W, H       int
}

// blocchiLetti: dalle righe dell'OCR (scala f) alle scritte: divise per colonne, unite se l'OCR
// ha spezzato una riga, divise ai separatori, corrette, votate nel tempo (voti può essere nil),
// unite in paragrafi.
func blocchiLetti(img *image.RGBA, righe []RigaOCR, f float64, voti *votazione, adesso time.Time) []Blocco {
	bl := segmenti(righe, f)
	for i := range bl {
		bl[i].Grezzo = bl[i].Testo
	}
	bl = unisciLettureDoppie(bl) // la stessa riga letta due volte in pezzi accavallati: una riga sola
	bl = unisciStessaRiga(bl)
	bl = unisciEtichette(img, bl)
	bl = spezzaSeparatori(bl)
	bl = spezzaPulsanti(bl)
	bl = togliFrecce(bl)
	bl = togliIconeTasti(bl) // "Swing Sword O": l'icona del pulsante resta del gioco
	bl = togliSegniIcona(bl) // "/Back", "'Close", "[Shipwreck": il segno è dell'icona
	for i := range bl {
		if bl[i].Grezzo == "" {
			bl[i].Grezzo = bl[i].Testo
		}
		bl[i].Testo, bl[i].Affid = correggiRigaOCR(bl[i].Grezzo)
		if voti != nil && img != nil {
			bl[i].Testo, bl[i].Affid = voti.scegli(img, bl[i], bl[i].Testo, bl[i].Affid, adesso)
		}
	}
	bl = spezzaIconeInMezzo(bl) // "Press t to sprint.": la "t" è la freccia dell'icona del tasto
	bl = unisciParagrafi(bl)
	for i := range bl {
		if bl[i].Righe > 1 {
			// paragrafo: il voto è quello del testo intero (anche la frase pronta intera)
			t, a := correggiRigaOCR(bl[i].Testo)
			bl[i].Testo, bl[i].Affid = t, a
		}
		if voti != nil {
			bl[i].Mossa = voti.mossa(bl[i], adesso) // si muove con la telecamera?
		}
	}
	return bl
}

// motivoNonTradurre: perché questa scritta non va tradotta ("" = va tradotta).
func motivoNonTradurre(b Blocco) string {
	switch {
	case !daTradurre(b.Testo), soloParoleVuote(b.Testo):
		return "non è testo da tradurre"
	case letturaSpazzatura(b.Testo) != "":
		return letturaSpazzatura(b.Testo)
	case !formaPlausibile(b):
		return "forma strana"
	case b.Affid == letturaDubbia:
		return "lettura incerta"
	case !letturaAffidabile(b.Testo):
		return "lettura incerta"
	case logoLettoMale(b.Testo):
		return "lettura incerta" // logo o nome del canale letto male
	case parolacciaDubbia(b.Testo):
		return "lettura incerta" // parolaccia che può essere un errore dell'OCR
	case parolaSenzaVocali(b.Testo):
		return "lettura incerta" // "rm son'" al posto di "I'm sorry"
	case nomeConCodice(b.Testo):
		return motivoNomeCodice
	}
	return ""
}

// controllaCompleta: colori della scritta, riquadro adattato alle lettere vere, e se accanto c'è
// una parola non letta (riga incompleta: non si traduce). Restituisce il motivo per scartarla.
// tutte: tutte le scritte lette della schermata (l'inchiostro di un'altra scritta letta accanto,
// come il pulsante vicino di una fila, non è una parola persa); può essere nil.
func controllaCompleta(img *image.RGBA, b *Blocco, tutte []Blocco) string {
	if m := motivoFratello(*b, tutte); m != "" {
		return m
	}
	if img == nil {
		return ""
	}
	if m := motivoMondo(img, *b, tutte); m != "" {
		return m // insegna, cartello, nome sulla mappa: resta del gioco
	}
	coloriBlocco(img, b)
	if m := rigaVicinaMancante(img, *b, tutte); m != "" {
		return m
	}
	if b.Righe <= 1 && b.Affid != letturaCatalog && scrittaContinuaTra(img, *b, tutte) && !fraseConPocoAccanto(img, *b, tutte) {
		return "riga incompleta (parola accanto non letta)"
	}
	// una parola maiuscola del catalogo con lettere scure su un fondo più chiaro e altre lettere
	// uguali accanto non lette ("CLUMP [WEIGHT]" sulla targa gialla di una macchina): è una
	// parola di un cartello del mondo letto a metà, non una voce dell'interfaccia
	if b.Righe <= 1 && b.Affid == letturaCatalog && len(strings.Fields(b.Testo)) == 1 && tuttoMaiuscolo(b.Testo) &&
		luce(b.Inchiostro)+40 < luce(b.Sfondo) && scrittaContinuaTra(img, *b, tutte) && !fraseConPocoAccanto(img, *b, tutte) {
		return "riga incompleta (parola accanto non letta)"
	}
	adattaAInchiostro(img, b)
	return ""
}

// traduciTutte: traduce le scritte (8 alla volta). veloce=true: solo frasi pronte, librerie e
// memoria (il primo giro, che si mostra subito); le altre restano con l'errore errSoloVeloce.
func traduciTutte(ctx context.Context, tr *Traduttore, da []Blocco, veloce bool) []Risultato {
	return traduciTutteFisse(ctx, tr, da, veloce, nil)
}

// traduciTutteFisse: come traduciTutte, ma le scritte che hanno già una traduzione fissa
// (fisse.go: già mostrate in questa sessione) escono subito con quella, senza traduttore.
func traduciTutteFisse(ctx context.Context, tr *Traduttore, da []Blocco, veloce bool, f *traduzioniFisse) []Risultato {
	risultati := make([]Risultato, len(da))
	coppia := coppiaDi(tr)
	fatte := make([]bool, len(da))
	for i := range da {
		if v, ok := f.cerca(coppia, da[i].Testo); ok {
			risultati[i] = Risultato{Originale: da[i].Testo, Tradotto: v, Motore: "memoria"}
			fatte[i] = true
		}
	}
	var wg sync.WaitGroup
	posti := make(chan struct{}, 8)
	var scritte []string
	for _, b := range da {
		scritte = append(scritte, b.Testo)
	}
	ctx = ConSchermata(ctx, scritte)
	if veloce {
		ctx = soloVeloce(ctx)
	}
	for i := range da {
		if fatte[i] {
			continue
		}
		wg.Add(1)
		posti <- struct{}{}
		go func(i int) {
			defer func() { <-posti; wg.Done() }()
			defer guard("pagina-scritta")
			risultati[i] = tr.TraduciScritta(ctx, da[i].Testo)
		}(i)
	}
	wg.Wait()
	return risultati
}

// motivoUgualeIt: la scritta in italiano è identica (non c'è niente da coprire, non è un errore).
const motivoUgualeIt = "uguale in italiano (va bene così)"

// motivoScartaTraduzione: la traduzione non si mostra (vuota, uguale all'inglese, mezza inglese).
func motivoScartaTraduzione(b Blocco, r Risultato) string {
	t := strings.TrimSpace(r.Tradotto)
	switch {
	case t == "" && r.Errore == errSoloVeloce:
		return "in attesa del traduttore"
	case t == "":
		if r.Errore != "" {
			return "nessuna traduzione: " + r.Errore
		}
		return "nessuna traduzione"
	case strings.EqualFold(t, strings.TrimSpace(b.Testo)):
		if r.Motore == "catalogo" || r.Motore == "glossario" || paroleUgualiIt[strings.ToLower(t)] {
			return motivoUgualeIt // "Video", "Audio", "No", "TAA": in italiano si scrive così
		}
		return "uguale all'inglese"
	case pezzoDiParolaInFondo(b.Testo) && pezzoCopiato(b.Testo, t):
		return motivoPezzoInFondo
	case r.Motore != "catalogo" && rigaMezzaTradotta(b.Testo, t):
		// (le frasi pronte sono scritte a mano: "Esci al desktop" è giusta così)
		return "mezza in inglese: scartata"
	}
	return ""
}

// motivoPezzoInFondo: la lettura finisce con un pezzo di parola ("Start s", "Survival is the r"):
// la riga è tagliata (lettura a metà) e la traduzione ha copiato il pezzo ("Inizia s").
const motivoPezzoInFondo = "riga incompleta (pezzo di parola in fondo)"

// paroleCorteFinali: parole inglesi di 1-2 lettere che possono chiudere una frase vera.
var paroleCorteFinali = map[string]bool{"a": true, "i": true, "an": true, "to": true, "of": true, "on": true, "in": true,
	"is": true, "it": true, "up": true, "me": true, "my": true, "so": true, "go": true, "no": true, "do": true, "be": true,
	"we": true, "he": true, "us": true, "or": true, "at": true, "by": true, "ok": true, "am": true, "as": true, "if": true,
	"oh": true, "hi": true, "ya": true, "yo": true, "ah": true, "eh": true, "uh": true, "x": true, "vs": true, "id": true}

// pezzoDiParolaInFondo: l'ultima parola della lettura è un pezzo minuscolo di 1-2 lettere che non
// è una parola ("s", "n", "op"), dopo una parola vera: l'OCR ha letto solo l'inizio della parola.
// Non conta dopo un numero ("5 s", "10 m": unità di misura).
func pezzoDiParolaInFondo(testo string) bool {
	campi := strings.Fields(testo)
	if len(campi) < 2 {
		return false
	}
	ult, prima := campi[len(campi)-1], campi[len(campi)-2]
	r := []rune(ult)
	if len(r) > 2 {
		return false
	}
	for _, c := range r {
		if !unicode.IsLower(c) {
			return false
		}
	}
	if paroleCorteFinali[ult] || strings.ContainsFunc(prima, unicode.IsDigit) {
		return false
	}
	return true
}

// pezzoCopiato: il pezzo di parola in fondo all'inglese è rimasto, uguale, in fondo alla
// traduzione ("Start s" -> "Inizia s"): si vedrebbe. Se il traduttore l'ha lasciato fuori
// ("STATISTICS s" -> "STATISTICHE") la traduzione va bene.
func pezzoCopiato(en, it string) bool {
	ce, ci := strings.Fields(en), strings.Fields(it)
	if len(ce) == 0 || len(ci) == 0 {
		return false
	}
	pezzo := ce[len(ce)-1]
	ult := strings.TrimRightFunc(ci[len(ci)-1], unicode.IsPunct)
	return strings.EqualFold(ult, pezzo)
}

// rifinisciPagina: dopo la traduzione, le scritte prendono grandezza e colori armonizzati, il
// fondo ricostruito e la pagina pulita (tutte le scritte inglesi tradotte cancellate).
func rifinisciPagina(img *image.RGBA, pronti, tutte []Blocco) {
	armonizza(pronti)
	colonnePiccole(pronti)
	for i := range pronti { // titoli al centro dello schermo: la traduzione resta al centro
		cx := pronti[i].X + pronti[i].W/2
		if pronti[i].Righe <= 1 && abs(cx-img.Bounds().Dx()/2) <= max(10, img.Bounds().Dx()/100) && pronti[i].W < img.Bounds().Dx()*9/10 {
			pronti[i].Centro = true
		}
	}
	for i := range pronti { // dopo armonizza: le scritte centrate hanno bisogno di sfondo dai due lati
		pronti[i].FondoX, _ = margineFondo(pronti[i])
		pronti[i].Fondo = fondoRicostruito(img, pronti[i])
		pronti[i].Marg = margineToppa(pronti[i], tutte)
	}
	pulita := pulisciPagina(img, pronti)
	for i := range pronti {
		pronti[i].Pulita = pulita
		pronti[i].Tinta = tintaBlocco(img, pronti[i])
	}
}

// preparaScritta: la scritta tradotta pronta da disegnare (colori, ombre, spazio libero).
func preparaScritta(img, primaImg *image.RGBA, b Blocco, r Risultato) Blocco {
	if b.Inchiostro == (b.Sfondo) { // colori non ancora letti
		coloriBlocco(img, &b)
	}
	b.Ombra = sfondoInMovimento(primaImg, img, b)
	b.Img = img
	contornoLettere(img, &b)
	ombraLettere(img, &b)
	if x := iconaAccanto(img, b); x > 0 && (b.IconaDx <= b.X || x < b.IconaDx) {
		b.IconaDx = x // icona del tasto non letta dall'OCR: la traduzione si ferma prima
	}
	if b.IconaDx > b.X {
		b.LargMax = min(b.LargMax, max(b.W, b.IconaDx-b.X-max(4, b.H/4)))
	}
	b.LargMax, b.Bordo = liberoFinoAlBordoB(img, b)
	b.Tradotto = rifinisci(b.Testo, r.Tradotto)
	return b
}

type chiaveLotto struct{}

// conLotto: le traduzioni fatte con questo contesto vanno al traduttore "a gruppi" (più fili
// insieme), usato per le schermate con tante scritte tradotte tutte insieme.
func conLotto(ctx context.Context) context.Context {
	return context.WithValue(ctx, chiaveLotto{}, true)
}

// vociDiUnaFila: una riga scartata come "mezza in inglese" che è in realtà una fila di voci con
// le iniziali maiuscole ("Apply Reset to Default Back": tre pulsanti, tradotti insieme mescolano
// le voci). La divido dove una parola con la maiuscola segue una parola che chiude una voce e lo
// spazio tra le due è più largo dello spazio normale della riga (1,5 volte la mediana dei buchi):
// "Apply" | "Reset to Default" | "Back". Solo righe senza punteggiatura da frase, con tutte le
// parole con la maiuscola (o parole di legame come "to", "of"). nil = non è una fila di voci.
func vociDiUnaFila(b Blocco) []Blocco {
	p := append([]ParolaOCR(nil), b.parole()...)
	if max(1, b.Righe) != 1 || len(p) < 3 || strings.ContainsAny(b.Testo, ".!?,;") {
		return nil
	}
	sort.SliceStable(p, func(i, j int) bool { return p[i].X < p[j].X })
	for _, w := range p {
		t := strings.Trim(strings.TrimSpace(w.T), "\"'()[]")
		if t == "" {
			return nil
		}
		if !inizioVoce(t) && !paroleSospese[strings.ToLower(t)] {
			return nil // frase normale, non una fila di voci
		}
	}
	buchi := make([]float64, len(p)-1)
	for i := 1; i < len(p); i++ {
		buchi[i-1] = p[i].X - (p[i-1].X + p[i-1].W)
	}
	ord := append([]float64(nil), buchi...)
	sort.Float64s(ord)
	normale := ord[(len(ord)-1)/2]
	var gruppi [][]ParolaOCR
	cur := []ParolaOCR{p[0]}
	for i := 1; i < len(p); i++ {
		if buchi[i-1] > 0 && buchi[i-1] >= 1.5*normale && inizioVoce(p[i].T) && fineVoce(p[i-1].T) && !chiedeSeguito(testoParole(cur)) {
			gruppi = append(gruppi, cur)
			cur = nil
		}
		cur = append(cur, p[i])
	}
	gruppi = append(gruppi, cur)
	if len(gruppi) < 2 {
		return nil
	}
	out := make([]Blocco, 0, len(gruppi))
	for _, g := range gruppi {
		n := bloccoDaPosizioni(g, b)
		n.Testo, n.Affid = correggiRigaOCR(n.Grezzo)
		out = append(out, n)
	}
	return out
}

// riprovaFile: le righe scartate come "mezza in inglese" che sono file di voci (vociDiUnaFila)
// tornano divise: ogni voce si controlla, si traduce e si copre per conto suo. Le altre righe
// restano come sono. Lo usano la traduzione vera (F7) e il banco di prova.
func riprovaFile(ctx context.Context, tr *Traduttore, img *image.RGBA, da []Blocco, ris []Risultato, tutte []Blocco) ([]Blocco, []Risultato) {
	var nuoveDa []Blocco
	var nuoveRis []Risultato
	cambiato := false
	for i, b := range da {
		if !strings.HasPrefix(motivoScartaTraduzione(b, ris[i]), "mezza") {
			nuoveDa, nuoveRis = append(nuoveDa, b), append(nuoveRis, ris[i])
			continue
		}
		voci := vociDiUnaFila(b)
		if len(voci) == 0 {
			nuoveDa, nuoveRis = append(nuoveDa, b), append(nuoveRis, ris[i])
			continue
		}
		vicine := append(append([]Blocco(nil), tutte...), voci...)
		var tenute []Blocco
		for _, v := range voci {
			if motivoNonTradurre(v) != "" || controllaCompleta(img, &v, vicine) != "" {
				continue
			}
			tenute = append(tenute, v)
		}
		if len(tenute) == 0 {
			nuoveDa, nuoveRis = append(nuoveDa, b), append(nuoveRis, ris[i])
			continue
		}
		cambiato = true
		r := traduciTutte(ctx, tr, tenute, false)
		nuoveDa, nuoveRis = append(nuoveDa, tenute...), append(nuoveRis, r...)
	}
	if !cambiato {
		return da, ris
	}
	return nuoveDa, nuoveRis
}

// soloParoleVuote: la scritta è fatta solo di parole che da sole non dicono niente ("the", "to a",
// "and"): è un pezzo di una frase letta male. Tradotta da sola ("il") sarebbe un frammento sullo
// schermo, quindi resta com'è.
func soloParoleVuote(s string) bool {
	parole := strings.Fields(strings.ToLower(s))
	if len(parole) == 0 || len(parole) > 3 {
		return false
	}
	for _, p := range parole {
		p = strings.Trim(p, ".,;:!?\"'()[]-…")
		if p == "" {
			continue
		}
		if !paroleSospese[p] && !paroleSeguito[p] {
			return false
		}
	}
	return true
}
