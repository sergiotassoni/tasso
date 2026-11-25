package traduce

// Il percorso della traduzione di tutta la schermata, senza le parti di Windows: dalle righe
// lette dall'OCR alle scritte pronte da disegnare. Lo usano la traduzione vera (F7), il banco
// di prova (--prova-traduzione) e i test su Linux, così si prova davvero quello che gira nel gioco.

import (
	"context"
	"image"
	"strings"
	"sync"
	"time"
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
	bl = unisciStessaRiga(bl)
	bl = unisciEtichette(img, bl)
	bl = spezzaSeparatori(bl)
	bl = spezzaPulsanti(bl)
	bl = togliFrecce(bl)
	for i := range bl {
		if bl[i].Grezzo == "" {
			bl[i].Grezzo = bl[i].Testo
		}
		bl[i].Testo, bl[i].Affid = correggiRigaOCR(bl[i].Grezzo)
		if voti != nil && img != nil {
			bl[i].Testo, bl[i].Affid = voti.scegli(img, bl[i], bl[i].Testo, bl[i].Affid, adesso)
		}
	}
	bl = unisciParagrafi(bl)
	for i := range bl {
		if bl[i].Righe > 1 {
			// paragrafo: il voto è quello del testo intero (anche la frase pronta intera)
			t, a := correggiRigaOCR(bl[i].Testo)
			bl[i].Testo, bl[i].Affid = t, a
		}
	}
	return bl
}

// motivoNonTradurre: perché questa scritta non va tradotta ("" = va tradotta).
func motivoNonTradurre(b Blocco) string {
	switch {
	case !daTradurre(b.Testo):
		return "non è testo da tradurre"
	case !formaPlausibile(b):
		return "forma strana"
	case b.Affid == letturaDubbia:
		return "lettura incerta"
	case !letturaAffidabile(b.Testo):
		return "lettura incerta"
	}
	return ""
}

// controllaCompleta: colori della scritta, riquadro adattato alle lettere vere, e se accanto c'è
// una parola non letta (riga incompleta: non si traduce). Restituisce il motivo per scartarla.
// tutte: tutte le scritte lette della schermata (l'inchiostro di un'altra scritta letta accanto,
// come il pulsante vicino di una fila, non è una parola persa); può essere nil.
func controllaCompleta(img *image.RGBA, b *Blocco, tutte []Blocco) string {
	if img == nil {
		return ""
	}
	coloriBlocco(img, b)
	if b.Righe <= 1 && b.Affid != letturaCatalog && scrittaContinuaTra(img, *b, tutte) {
		return "riga incompleta (parola accanto non letta)"
	}
	adattaAInchiostro(img, b)
	return ""
}

// traduciTutte: traduce le scritte (8 alla volta). veloce=true: solo frasi pronte, librerie e
// memoria (il primo giro, che si mostra subito); le altre restano con l'errore errSoloVeloce.
func traduciTutte(ctx context.Context, tr *Traduttore, da []Blocco, veloce bool) []Risultato {
	risultati := make([]Risultato, len(da))
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
	case r.Motore != "catalogo" && rigaMezzaTradotta(b.Testo, t):
		// (le frasi pronte sono scritte a mano: "Esci al desktop" è giusta così)
		return "mezza in inglese: scartata"
	}
	return ""
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
