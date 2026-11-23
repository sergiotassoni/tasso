//go:build windows

package traduce

// F6: traduce tutte le scritte della finestra del gioco e le sostituisce con l'italiano.
// Premi di nuovo F6 per tornare all'inglese. Se la schermata cambia (altro menu), ritraduce da sola.

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// TempiPagina: quanto è servito per l'ultima schermata (per il collaudo e la finestra)
type TempiPagina struct {
	OCR, Traduzione, Totale time.Duration
	Visto                   time.Time // quando mi sono accorto che la schermata era cambiata
	Scritte, Nuove          int
}

var (
	paginaMostrata atomic.Int64 // quando è stata disegnata l'ultima traduzione della schermata
	cambioVisto    atomic.Int64
	tempiPagina    atomic.Pointer[TempiPagina]
	paginaAttiva   atomic.Bool
	paginaGiocoA   atomic.Uintptr
	paginaGiro     atomic.Int64
	ultimeScritte  atomic.Int64 // quante scritte ho tradotto l'ultima volta
	paginaRileggi  atomic.Bool  // F10: rileggi la schermata anche se non è cambiata
)

func attivaPagina() {
	fg, _, _ := pGetForeground.Call()
	if paginaAttiva.Load() {
		paginaAttiva.Store(false)
		paginaGiro.Add(1)
		NascondiPagina()
		impostaPausa(exeDiFinestra(paginaGiocoA.Load()), true) // il tutto automatico non riparte da solo su questo gioco
		emetti("info", map[string]string{"testo": "In pausa: schermata di nuovo in inglese (" + nomeTasto("pagina") + " per riprendere)."})
		return
	}
	impostaPausa(exeDiFinestra(fg), false)
	if wv != nil && fg == uintptr(wv.Window()) {
		emetti("info", map[string]string{"testo": "Vai nel gioco e premi " + nomeTasto("pagina") + " lì: traduco tutte le scritte della sua finestra."})
		return
	}
	if _, ok := areaFinestra(fg); !ok {
		emetti("info", map[string]string{"testo": "Non trovo la finestra del gioco: mettilo in finestra o a schermo intero senza bordi e riprova."})
		return
	}
	avviaPaginaSu(fg)
}

func avviaPaginaSu(fg uintptr) {
	paginaAttiva.Store(false)
	paginaGiro.Add(1)
	paginaGiocoA.Store(fg)
	apriLibreriaPer(fg)
	paginaAttiva.Store(true)
	ov.Spegni() // la finestrella dei dialoghi non serve mentre traduco tutta la schermata
	giro := paginaGiro.Add(1)
	go cicloPagina(giro)
}

func cicloPagina(giro int64) {
	defer guard("pagina")
	var ultima, ultimaColori Impronta
	prima := true
	nascostaPerAltTab := false
	nere := 0
	for paginaAttiva.Load() && paginaGiro.Load() == giro {
		gioco := paginaGiocoA.Load()
		if v, _, _ := pIsWindow.Call(gioco); v == 0 { // il gioco è stato chiuso
			paginaAttiva.Store(false)
			NascondiPagina()
			return
		}
		fg, _, _ := pGetForeground.Call()
		icona, _, _ := pIsIconic.Call(gioco)
		area, ok := areaFinestra(gioco)
		if fg != gioco || icona != 0 || !ok {
			if !nascostaPerAltTab {
				NascondiPagina()
				nascostaPerAltTab = true
			}
			time.Sleep(400 * time.Millisecond)
			continue
		}
		if nascostaPerAltTab {
			nascostaPerAltTab, prima = false, true // tornato nel gioco: rileggo
		}
		if paginaRileggi.Swap(false) {
			prima = true
		}
		img, err := cattura(area)
		if err == nil && catturaNera(img) {
			NascondiPagina() // scena scura: le traduzioni della schermata di prima non valgono più
			prima = true
			nere++
			if nere >= 3 { // tre di fila: non è una scena scura, Windows non mi lascia vedere il gioco
				avvisaSchermoEsclusivo(paginaGiocoA.Load())
			}
			time.Sleep(1500 * time.Millisecond)
			continue
		}
		nere = 0
		if err == nil {
			imp := calcolaImpronta(img)
			if prima || imp.cambiata(ultima) {
				cambioVisto.Store(time.Now().UnixNano())
				// lettura e traduzione partono subito, in anticipo, sulla prima immagine...
				type esitoPagina struct {
					p        *paginaPronta
					occupata bool
				}
				anticipo := make(chan esitoPagina, 1)
				go func(img *image.RGBA) {
					defer guard("pagina-anticipo")
					var e esitoPagina
					defer func() { anticipo <- e }() // anche dopo un errore fermato da guard
					e.p, e.occupata = preparaPagina(area, img, img, giro)
				}(img)
				// ...intanto controllo che la schermata smetta di muoversi (menu che scorre, dissolvenza)
				time.Sleep(80 * time.Millisecond)
				if img2, err := cattura(area); err == nil {
					imp2 := calcolaImpronta(img2)
					if imp2.differenza(imp) > 3*sogliaCambiato && !prima {
						NascondiPagina() // cambio di scena: tolgo subito le scritte vecchie
					}
					e := <-anticipo
					pronta, occupata := e.p, e.occupata
					if pronta == nil || imp2.cambiata(imp) {
						// si muoveva ancora: rifaccio sull'immagine nuova. Anche se l'immagine si
						// muove (video) traduco: le scritte sopra un video diventano una fascia scura
						pronta, occupata = preparaPagina(area, img2, img, giro)
					}
					if pronta != nil && mostraPaginaPronta(pronta, giro) {
						ultima, ultimaColori, prima = imp2, imp2, false
					} else if pronta == nil && occupata {
						// la lettura serviva ai dialoghi o alla chat: non è "niente da tradurre",
						// riprovo al giro dopo sulla stessa schermata
						time.Sleep(150 * time.Millisecond)
						continue
					} else if pronta == nil {
						// niente da tradurre qui (già nella lingua giusta, lingua non scaricata,
						// nessuna scritta): tolgo le scritte vecchie e aspetto un cambio vero,
						// invece di rileggere lo schermo all'infinito
						NascondiPagina()
						ultima, ultimaColori, prima = imp2, imp2, false
						ultimeScritte.Store(0)
					}
				} else {
					<-anticipo
				}
			} else if bl := blocchiPagina(); len(bl) > 0 && imp.differenza(ultimaColori) > 0.2 {
				// piccolo cambio (mouse sopra un pulsante): aggiorno solo i colori. Se l'immagine
				// è identica all'ultima volta non rifaccio il lavoro (il giro resta svelto)
				ultimaColori = imp
				if nuovi, ok := ricolora(img, bl); ok && paginaGiro.Load() == giro {
					MostraPagina(area, nuovi)
				}
			}
		}
		if ultimeScritte.Load() == 0 {
			time.Sleep(400 * time.Millisecond) // niente scritte (si sta giocando): controllo più di rado
		} else {
			time.Sleep(40 * time.Millisecond) // svelto: le schermate nuove e i pulsanti evidenziati cambiano subito
		}
	}
}

// paginaPronta: scritte lette e tradotte, da disegnare.
type paginaPronta struct {
	area        Zona
	img         *image.RGBA
	pronti      []Blocco
	nuove       int
	t0          time.Time
	tOCR, tTrad time.Duration
}

// traduciPagina: legge tutte le scritte con la loro posizione, le traduce e le disegna.
func traduciPagina(area Zona, img, primaImg *image.RGBA, giro int64) bool {
	p, _ := preparaPagina(area, img, primaImg, giro)
	return p != nil && mostraPaginaPronta(p, giro)
}

// preparaPagina: legge tutte le scritte con la loro posizione e le traduce (senza disegnare).
// Senza niente da disegnare restituisce nil; occupata dice che la lettura del testo era presa
// da un altro lavoro (dialoghi, chat degli altri) e conviene riprovare.
func preparaPagina(area Zona, img, primaImg *image.RGBA, giro int64) (p *paginaPronta, occupata bool) {
	if !occupato.CompareAndSwap(false, true) {
		return nil, true
	}
	defer occupato.Store(false)
	t0 := time.Now()
	if err := assicuraOCR(); err != nil {
		emetti("info", map[string]string{"testo": "La lettura del testo di Windows non parte: " + err.Error()})
		return nil, false
	}
	var buf bytes.Buffer
	f, err := preparaPaginaPerOCR(img, &buf)
	if err != nil {
		return nil, false
	}
	righe, err := ocr.LeggiPosizioni(buf.Bytes())
	if err != nil {
		emetti("info", map[string]string{"testo": "Lettura del testo: " + err.Error()})
		return nil, false
	}
	blocchi := unisciParagrafi(segmenti(righe, f))
	nostraL, ceNostraL := nostraFinestraIn(area)
	var testiLetti []string
	for _, b := range blocchi {
		if ceNostraL && dentro(b, nostraL) {
			continue // la finestra di Tasso (in italiano) non deve contare
		}
		testiLetti = append(testiLetti, b.Testo)
	}
	if lingua := RilevaLingua(testiLetti); lingua != "" {
		cart, nome := "", "Il gioco"
		if g, ok := riconosciGioco(percorsoDiFinestra(paginaGiocoA.Load())); ok {
			cart, nome = g.Cartella, g.Nome
		}
		if !segnaLinguaVista(cart, nome, lingua) {
			return nil, false // lingua che non so ancora tradurre, o già quella che vuoi
		}
	}
	spazioLibero(blocchi, area.W)
	nostra, ceNostra := nostraFinestraIn(area)
	var pronti []Blocco
	nuove := 0
	tOCR := time.Since(t0)
	// tutte le scritte insieme (8 alla volta): con il traduttore "a gruppi" una schermata piena
	// di scritte si traduce in una frazione del tempo che serviva una alla volta
	var da []Blocco
	for _, b := range blocchi {
		if !daTradurre(b.Testo) || !formaPlausibile(b) {
			continue
		}
		if ceNostra && dentro(b, nostra) { // è la finestra di Tasso Traduce sopra il gioco: non la traduco
			continue
		}
		da = append(da, b)
	}
	risultati := make([]Risultato, len(da))
	var wg sync.WaitGroup
	posti := make(chan struct{}, 8)
	ctx := conLotto(context.Background())
	for i := range da {
		wg.Add(1)
		posti <- struct{}{}
		go func(i int) {
			defer func() { <-posti; wg.Done() }()
			defer guard("pagina-scritta")
			risultati[i] = traduttore.TraduciScritta(ctx, da[i].Testo)
		}(i)
	}
	wg.Wait()
	tTrad := time.Since(t0) - tOCR
	if !paginaAttiva.Load() || paginaGiro.Load() != giro {
		return nil, false
	}
	for i, b := range da {
		r := risultati[i]
		if r.Tradotto == "" || r.Tradotto == r.Originale {
			continue
		}
		if r.Motore != "memoria" && r.Motore != "catalogo" && r.Motore != "libreria" {
			nuove++
		}
		if sfondoInMovimento(primaImg, img, b) {
			b.Sfondo, b.Inchiostro, b.Fondo = coloreFascia, coloreTestoFascia, nil
		} else {
			coloriBlocco(img, &b)
			b.Fondo = fondoRicostruito(img, b)
		}
		b.Tradotto = r.Tradotto
		pronti = append(pronti, b)
		if len(pronti) <= 40 { // nella cronologia (F8) le scritte nuove, senza doppioni
			aggiungiCronologia(r, true)
		}
	}
	armonizza(pronti)
	return &paginaPronta{area: area, img: img, pronti: pronti, nuove: nuove, t0: t0, tOCR: tOCR, tTrad: tTrad}, false
}

// mostraPaginaPronta disegna le scritte tradotte sopra il gioco.
func mostraPaginaPronta(p *paginaPronta, giro int64) bool {
	area, img, pronti, nuove, t0, tOCR, tTrad := p.area, p.img, p.pronti, p.nuove, p.t0, p.tOCR, p.tTrad
	if nuove > 0 {
		go func() { // su disco in sottofondo: non ritarda il disegno
			defer guard("salva-memoria")
			traduttore.SalvaMemoria()
		}()
	}
	if paginaAttiva.Load() && paginaGiro.Load() == giro {
		MostraPagina(area, pronti)
		if !paginaAttiva.Load() || paginaGiro.Load() != giro {
			NascondiPagina() // spento proprio adesso: niente scritte rimaste sullo schermo
			return false
		}
		paginaMostrata.Store(time.Now().UnixNano())
		tempiPagina.Store(&TempiPagina{OCR: tOCR, Traduzione: tTrad, Totale: time.Since(t0), Visto: time.Unix(0, cambioVisto.Load()), Scritte: len(pronti), Nuove: nuove})
		ultimeScritte.Store(int64(len(pronti)))
		go func() {
			defer guard("anteprima")
			if !salvaAnteprime {
				return // l'immagine del gioco non resta sul disco (privacy)
			}
			_ = salvaAnteprima(img, pronti, filepath.Join(dirApp, "logs", "pagina-anteprima.png"))
		}()
		if len(pronti) > 0 {
			emetti("info", map[string]string{"testo": fmt.Sprintf("Schermata tradotta: %d scritte in %.1f s · %s per tornare all'inglese", len(pronti), time.Since(t0).Seconds(), nomeTasto("pagina"))})
		}
	}
	return true
}

var (
	muNero       sync.Mutex
	avvisatoNero = map[uintptr]bool{}
)

// catturaNera: l'immagine è tutta nera (o quasi). Succede con i giochi a schermo intero
// "esclusivo": Windows non lascia catturare la loro immagine.
func catturaNera(img *image.RGBA) bool {
	b := img.Bounds()
	passo := max(1, b.Dx()/320)
	for y := b.Min.Y; y < b.Max.Y; y += max(1, b.Dy()/180) {
		for x := b.Min.X; x < b.Max.X; x += passo {
			c := img.RGBAAt(x, y)
			if c.R > 6 || c.G > 6 || c.B > 6 {
				return false
			}
		}
	}
	return true
}

func avvisaSchermoEsclusivo(h uintptr) {
	muNero.Lock()
	gia := avvisatoNero[h]
	avvisatoNero[h] = true
	muNero.Unlock()
	if gia {
		return
	}
	emetti("info", map[string]string{"testo": "Non riesco a vedere l'immagine di questo gioco: forse è a schermo intero esclusivo. Nelle impostazioni video del gioco scegli \"Finestra senza bordi\" (borderless) e la traduzione funzionerà."})
}

var pGetWindow = user32.NewProc("GetWindow")

// sottoA: la finestra h sta sotto la finestra sopra nell'ordine dello schermo.
func sottoA(h, sopra uintptr) bool {
	for w, i := h, 0; w != 0 && i < 5000; i++ {
		w, _, _ = pGetWindow.Call(w, 3) // GW_HWNDPREV: la finestra appena sopra
		if w == sopra {
			return true
		}
	}
	return false
}

var (
	pIsWindowVisible = user32.NewProc("IsWindowVisible")
	pIsIconic        = user32.NewProc("IsIconic")
)

// nostraFinestraIn: il rettangolo della finestra di Tasso Traduce, in coordinate dell'area
// catturata, se è visibile e non ridotta a icona.
func nostraFinestraIn(area Zona) (Zona, bool) {
	if wv == nil {
		return Zona{}, false
	}
	h := uintptr(wv.Window())
	if v, _, _ := pIsWindowVisible.Call(h); v == 0 {
		return Zona{}, false
	}
	if i, _, _ := pIsIconic.Call(h); i != 0 {
		return Zona{}, false
	}
	// se il gioco sta sopra la finestra di Tasso (gioco a schermo intero), la finestra di Tasso
	// non si vede: le scritte in quel punto sono del gioco e vanno tradotte
	if g := paginaGiocoA.Load(); g != 0 && sottoA(h, g) {
		return Zona{}, false
	}
	var r rect
	if ok, _, _ := pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&r))); ok == 0 {
		return Zona{}, false
	}
	return Zona{X: int(r.Left) - area.X, Y: int(r.Top) - area.Y, W: int(r.Right - r.Left), H: int(r.Bottom - r.Top)}, true
}

// salvaAnteprime: solo il programma di prova salva l'ultima schermata tradotta.
var salvaAnteprime = false
