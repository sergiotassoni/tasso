//go:build windows

package traduce

// F6: traduce tutte le scritte della finestra del gioco e le sostituisce con l'italiano.
// Premi di nuovo F6 per tornare all'inglese. Se la schermata cambia (altro menu), ritraduce da sola.

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
		if area, ok := areaFinestra(paginaGiocoA.Load()); ok { // anche nel gioco: F7 non deve sembrare morto
			mostraAvviso(area, testoTasso("Tasso: traduzione in pausa. "+nomeTasto("pagina")+" per riprendere.",
				"Tasso: translation paused. "+nomeTasto("pagina")+" to resume."), 22, 20, 3*time.Second)
		}
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
	sveglia() // l'IA si accende subito, non dopo 30 secondi
	if linguaIlleggibile(fg) {
		return
	}
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
	var ultimaImg *image.RGBA // la schermata dell'ultima lettura: per vedere se le scritte sono cambiate
	// zone che si muovono da sole senza cambiare le scritte (personaggio animato accanto al menu,
	// pianeta che gira): imparate quando una rilettura trova le stesse scritte, poi non contano
	animate := make([]bool, celleBordiX*celleBordiY)
	animateDa := time.Now()
	var celleUltime []bool // zone cambiate che hanno fatto partire l'ultima rilettura
	prima := true
	perche := "avvio" // perché prima è vero (solo per pagina-tempi.txt)
	nascostaPerAltTab := false
	nere := 0
	mosse := 0         // giri di fila in cui la schermata si muoveva ancora
	erroriOCR := 0     // letture fallite di fila (errore della lettura di Windows, non "niente da tradurre")
	fermo := false     // il giro prima ha finito il suo lavoro sull'ultima cattura
	var genCatt uint64 // fotogramma dell'ultima cattura (0 = non so)
	var imgCatt *image.RGBA
	var impCatt Impronta
	tinteViste := map[[4]int][12]uint8{} // ultima tinta controllata di ogni scritta (vedi blocchiDaRicolorare)
	impara := func(celle, sotto []bool) {
		// zone che cambiano da sole: d'ora in poi non fanno rileggere. Mai quelle sotto le
		// scritte (lì un cambio vero va sempre visto) e al massimo un quarto dello schermo:
		// altrimenti una schermata nuova non verrebbe più riletta
		quante := 0
		for _, a := range animate {
			if a {
				quante++
			}
		}
		for c, si := range celle {
			if si && (sotto == nil || !sotto[c]) && !animate[c] && quante < len(animate)/4 {
				animate[c] = true
				quante++
			}
		}
	}
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
			fermo = false
			time.Sleep(400 * time.Millisecond)
			continue
		}
		if nascostaPerAltTab {
			nascostaPerAltTab, prima, perche = false, true, "alt-tab" // tornato nel gioco: rileggo
		}
		if paginaRileggi.Swap(false) {
			prima, perche = true, "rileggi"
		}
		img, gen, err := catturaGen(area, genCatt)
		stesso := err == errStessoFotogramma
		if stesso {
			if fermo && !prima {
				// lo schermo non ha mandato fotogrammi nuovi: niente da rifare (prima a ogni giro
				// convertivo e confrontavo di nuovo tutta la schermata)
				time.Sleep(pausaPagina())
				continue
			}
			img, err = imgCatt, nil
			if img == nil {
				img, gen, err = catturaGen(area, 0)
				stesso = false
			}
		}
		fermo = false
		if err == nil && catturaNera(img) {
			NascondiPagina() // scena scura: le traduzioni della schermata di prima non valgono più
			prima, perche = true, "nera"
			nere++
			if nere >= 3 { // tre di fila: non è una scena scura, Windows non mi lascia vedere il gioco
				avvisaSchermoEsclusivo(paginaGiocoA.Load())
			}
			genCatt = 0
			time.Sleep(1500 * time.Millisecond)
			continue
		}
		nere = 0
		if err == nil {
			imp := impCatt
			if !stesso {
				imp = calcolaImpronta(img)
			}
			genCatt, imgCatt, impCatt = gen, img, imp
			// cambia solo la luce (sfondo animato e sfocato, scritta che si colora sotto il mouse):
			// i contorni delle scritte sono gli stessi, niente rilettura, solo i colori. Il
			// riferimento resta l'ultima lettura: un cambio lento che si somma viene visto lo stesso
			soloLuce := false
			celleUltime = nil
			if !prima && imp.cambiata(ultima) {
				if time.Since(animateDa) > 60*time.Second { // ogni tanto ricomincio da capo
					clear(animate)
					animateDa = time.Now()
				}
				soloLuce = true
				celleUltime = celleBordiCambiate(img, ultimaImg)
				bl0 := blocchiPagina()
				nc := 0
				for c, si := range celleUltime {
					if si && !animate[c] {
						nc++
					}
				}
				// sotto una scritta tradotta conta la forma delle lettere, non i contorni della zona
				// (il riquadro illuminato dal mouse cambia i contorni ma non le lettere)
				tocca := false
				for _, b := range bl0 {
					sotto := celleDiBlocchi([]Blocco{b}, img.Bounds())
					for c, si := range celleUltime {
						if si && sotto[c] {
							tocca = inchiostroCambiato(ultimaImg, img, b)
							break
						}
					}
					if tocca {
						break
					}
				}
				// una zona sola che cambia (lucina che lampeggia, icona animata) non basta: le
				// scritte nuove, le finestrelle e le righe di dialogo occupano almeno due zone.
				// Ma se cambiano i contorni sotto una scritta tradotta ("On" → "Off", descrizione
				// nuova) o compare qualcosa dove non c'era niente da tradurre, rileggo sempre.
				soloLuce = nc < 2 && !tocca && !(nc == 1 && len(bl0) == 0)
				motivoLettura.Store(fmt.Sprintf("zone=%d", nc))
			} else if prima {
				motivoLettura.Store("prima:" + perche)
			}
			if !soloLuce && (prima || imp.cambiata(ultima)) {
				cambioVisto.Store(time.Now().UnixNano())
				// subito, sulla prima immagine (uno scatto di rotella dura meno di un giro): righe
				// scorse → via tutte le traduzioni; scritte cambiate sotto una traduzione → via
				// quella. Così una traduzione vecchia non resta mai sopra una riga diversa
				if !prima {
					togliVecchie(area, img)
				}
				// lettura e traduzione partono subito, in anticipo, sulla prima immagine...
				annulla := &atomic.Bool{}
				anticipo := make(chan esitoPagina, 1)
				go func(img *image.RGBA) {
					defer guard("pagina-anticipo")
					e := esitoPagina{esito: letturaErrore}
					defer func() { anticipo <- e }() // anche dopo un errore fermato da guard
					e.p, e.esito = preparaPagina(area, img, img, giro, annulla)
				}(img)
				// ...intanto controllo che la schermata smetta di muoversi (menu che scorre, dissolvenza)
				time.Sleep(80 * time.Millisecond)
				img2, gen2, err := catturaGen(area, genCatt)
				imp2 := imp
				switch {
				case err == errStessoFotogramma: // fermo da 80 ms: è ancora la prima immagine
					img2, err = img, nil
				case err == nil:
					imp2 = calcolaImpronta(img2)
					genCatt, imgCatt, impCatt = gen2, img2, imp2
				}
				if err != nil {
					annulla.Store(true)
					<-anticipo
					genCatt = 0
					prima, perche = true, "cattura"
					continue
				}
				if imp2.cambiata(imp) && !prima {
					togliVecchie(area, img2) // cambiato ancora negli ultimi 80 ms
				}
				if bl := blocchiPagina(); imp2.cambiata(imp) && mosse < 12 && ((len(bl) == 0 && scritteMosse(img, img2, animate)) || paginaSpostata(img2, bl)) {
					// si muove ancora (rotella, menu che scorre): niente scritte finché non si
					// ferma, così inglese e italiano non si sovrappongono mai. Se si muove
					// sempre (video dietro al menu) dopo circa un secondo e mezzo traduco lo stesso.
					// Se le scritte sono ferme e cambia solo il resto (mouse sopra un oggetto,
					// riquadro illuminato, sfondo animato) le traduzioni restano: niente sfarfallio.
					// Il lavoro anticipato si ferma dopo la lettura: niente traduzioni sprecate
					annulla.Store(true)
					NascondiPagina()
					mosse++
					prima, perche = true, "mosse"
					<-anticipo
					continue
				}
				if !imp2.cambiata(imp) {
					mosse = 0
				}
				rifare := imp2.cambiata(imp) && scritteMosse(img, img2, animate)
				if rifare {
					annulla.Store(true) // si muoveva ancora: la prima immagine non serve più
				}
				e := <-anticipo
				pronta, esito := e.p, e.esito
				if rifare || (pronta == nil && esito != letturaNiente) {
					// rifaccio sull'immagine nuova. Anche se l'immagine si muove (video) traduco:
					// le scritte sopra un video diventano una fascia scura
					pronta, esito = preparaPagina(area, img2, img, giro, nil)
				}
				if pronta != nil && celleUltime != nil && stesseScritte(blocchiPagina(), pronta.pronti) {
					// riletto per niente: le zone cambiate si muovono da sole, d'ora in poi non contano
					impara(celleUltime, celleDiBlocchi(pronta.pronti, img2.Bounds()))
				}
				switch {
				case pronta != nil:
					if mostraPaginaPronta(pronta, giro) {
						ultima, ultimaColori, prima, ultimaImg = imp2, imp2, false, img2
						erroriOCR = 0
						clear(tinteViste)
					}
				case esito == letturaOccupata || esito == letturaAnnullata:
					// la lettura serviva ai dialoghi o alla chat: non è "niente da tradurre",
					// riprovo al giro dopo sulla stessa schermata
					time.Sleep(150 * time.Millisecond)
					continue
				case esito == letturaErrore:
					// errore momentaneo della lettura di Windows: non vuol dire "niente da
					// tradurre". Riprovo presto, senza imparare zone animate e senza dimenticare
					// la schermata (un po' più tardi a ogni errore di fila)
					erroriOCR++
					prima, perche = true, "errore-lettura"
					time.Sleep(min(time.Duration(erroriOCR)*300*time.Millisecond, 3*time.Second))
					continue
				default:
					// niente da tradurre e le zone cambiate erano un'icona o un cursore che
					// lampeggia: d'ora in poi non fanno rileggere (stesso tetto di un quarto)
					erroriOCR = 0
					if celleUltime != nil {
						impara(celleUltime, nil)
					}
					// niente da tradurre qui (già nella lingua giusta, lingua non scaricata,
					// nessuna scritta): tolgo le scritte vecchie e aspetto un cambio vero,
					// invece di rileggere lo schermo all'infinito
					NascondiPagina()
					ultima, ultimaColori, prima, ultimaImg = imp2, imp2, false, img2
					ultimeScritte.Store(0)
				}
			} else if bl := blocchiPagina(); len(bl) > 0 {
				if imp.differenza(ultimaColori) > 0.2 && paginaSpostata(img, bl) {
					// la rotella ha spostato le scritte: via subito le traduzioni (sarebbero nel posto
					// sbagliato, sopra l'inglese) e rileggo appena la schermata si ferma
					NascondiPagina()
					prima, perche = true, "rotella"
					continue
				}
				// mouse sopra un pulsante (diventa dorato), voce selezionata: ricoloro solo le
				// scritte la cui tinta è cambiata, anche se il resto dello schermo è identico.
				// Se niente è cambiato non rifaccio nessun lavoro (il giro resta svelto)
				if solo, n := blocchiDaRicolorare(img, bl, tinteViste); n > 0 {
					ultimaColori = imp
					if nuovi, ok := ricoloraSolo(img, bl, solo); ok && paginaGiro.Load() == giro {
						MostraPagina(area, nuovi)
						if esisteVoce(filepath.Join(dirApp, "logs", "salva-anteprima")) { // controllo a mano: anche il mouse sopra
							img2 := img
							go func() {
								defer guard("anteprima-mouse")
								_ = salvaAnteprima(img2, nuovi, filepath.Join(dirApp, "logs", "pagina-anteprima-mouse.png"))
							}()
						}
					}
				}
			}
			fermo = true
		}
		time.Sleep(pausaPagina())
	}
}

// pausaPagina: quanto aspettare tra un giro e l'altro.
func pausaPagina() time.Duration {
	if ultimeScritte.Load() == 0 {
		return 400 * time.Millisecond // niente scritte (si sta giocando): controllo più di rado
	}
	return 40 * time.Millisecond // svelto: le schermate nuove e i pulsanti evidenziati cambiano subito
}

// togliVecchie: appena vedo che la schermata è cambiata, prima ancora di rileggerla. Se le righe
// sono scorse (rotella) tolgo tutte le traduzioni; se sotto qualche traduzione l'inglese è
// diventato un altro, tolgo solo quella. Le altre restano ferme: niente sfarfallio.
func togliVecchie(area Zona, img *image.RGBA) {
	bl := blocchiPagina()
	if len(bl) == 0 {
		return
	}
	if paginaSpostata(img, bl) {
		NascondiPagina()
		return
	}
	resto, tolte := togliCambiate(img, bl)
	switch {
	case tolte == 0:
	case len(resto) == 0:
		NascondiPagina()
	default:
		MostraPagina(area, resto)
	}
}

// esito di preparaPagina quando non c'è niente da disegnare
type esitoLettura int

const (
	letturaFatta     esitoLettura = iota // scritte pronte
	letturaNiente                        // letto, niente da tradurre
	letturaOccupata                      // la lettura del testo era presa da un altro lavoro: riprovare
	letturaErrore                        // la lettura di Windows non ha funzionato (riprovare presto)
	letturaAnnullata                     // lo schermo si muoveva ancora: lavoro fermato
)

type esitoPagina struct {
	p     *paginaPronta
	esito esitoLettura
}

// avvisoLettura: un errore della lettura si dice al massimo ogni 30 secondi (si riprova spesso).
var avvisoLettura atomic.Int64

func avvisaErroreLettura(testo string) {
	if t := avvisoLettura.Load(); time.Since(time.Unix(0, t)) < 30*time.Second {
		return
	}
	avvisoLettura.Store(time.Now().UnixNano())
	emetti("info", map[string]string{"testo": testo})
}

// paginaPronta: scritte lette e tradotte, da disegnare.
type paginaPronta struct {
	area        Zona
	img         *image.RGBA
	pronti      []Blocco
	nuove       int
	t0          time.Time
	tOCR, tTrad time.Duration
	cron        []Risultato // da mettere nella cronologia (F8) quando la pagina si vede
}

// preparaPagina: legge tutte le scritte con la loro posizione e le traduce (senza disegnare).
// Senza niente da disegnare restituisce nil e il perché (esitoLettura). annulla (può essere nil):
// se diventa vero, dopo la lettura il lavoro si ferma senza tradurre (lo schermo si muove ancora).
func preparaPagina(area Zona, img, primaImg *image.RGBA, giro int64, annulla *atomic.Bool) (p *paginaPronta, esito esitoLettura) {
	if !occupato.CompareAndSwap(false, true) {
		return nil, letturaOccupata
	}
	defer occupato.Store(false)
	fermato := func() bool { return annulla != nil && annulla.Load() }
	t0 := time.Now()
	if err := assicuraOCR(); err != nil {
		avvisaErroreLettura("La lettura del testo di Windows non parte: " + err.Error())
		return nil, letturaErrore
	}
	var buf bytes.Buffer
	f, err := preparaPaginaPerOCR(img, &buf)
	if err != nil {
		return nil, letturaErrore
	}
	righe, err := ocr.LeggiPosizioni(buf.Bytes())
	if err != nil {
		avvisaErroreLettura("Lettura del testo: " + err.Error())
		return nil, letturaErrore
	}
	if fermato() {
		return nil, letturaAnnullata
	}
	righe = rileggiPiccole(img, righe, f)
	if fermato() {
		return nil, letturaAnnullata
	}
	blocchi := segmenti(righe, f)
	for i := range blocchi {
		blocchi[i].Testo = correggiParole(separaAttaccate(blocchi[i].Testo))
	}
	blocchi = unisciParagrafi(blocchi)
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
			return nil, letturaNiente // lingua che non so ancora tradurre, o già quella che vuoi
		}
	}
	spazioLibero(blocchi, area.W)
	if esisteVoce(filepath.Join(dirApp, "logs", "salva-anteprima")) { // controllo a mano: cosa ha letto
		var sb strings.Builder
		for _, b := range blocchi {
			fmt.Fprintf(&sb, "%d,%d %dx%d righe=%d em=%.1f tradurre=%v affidabile=%v | %s\n", b.X, b.Y, b.W, b.H, b.Righe, b.Em, daTradurre(b.Testo), letturaAffidabile(b.Testo), b.Testo)
		}
		_ = os.WriteFile(filepath.Join(dirApp, "logs", "pagina-letto.txt"), []byte(sb.String()), 0o644)
	}
	nostra, ceNostra := nostraFinestraIn(area)
	var pronti []Blocco
	nuove := 0
	tOCR := time.Since(t0)
	// tutte le scritte insieme (8 alla volta): con il traduttore "a gruppi" una schermata piena
	// di scritte si traduce in una frazione del tempo che serviva una alla volta
	var da []Blocco
	for _, b := range blocchi {
		if !daTradurre(b.Testo) || !formaPlausibile(b) || !letturaAffidabile(b.Testo) {
			continue
		}
		if ceNostra && dentro(b, nostra) { // è la finestra di Tasso Traduce sopra il gioco: non la traduco
			continue
		}
		da = append(da, b)
	}
	if g, ok := riconosciGioco(percorsoDiFinestra(paginaGiocoA.Load())); ok {
		impostaNomeGiocoAttivo(g.Nome) // il nome del gioco resta uguale anche dentro le frasi
		var tenuti []Blocco
		for i := range da { // "Lia (s Bar" letto dall'OCR -> "Liar's Bar": il nome del gioco resta giusto
			da[i].Testo = correggiNomeGioco(da[i].Testo, g.Nome)
			if soloNome(da[i].Testo) == soloNome(g.Nome) {
				continue // il titolo del gioco ("NO MAN'S SKY") non si traduce
			}
			tenuti = append(tenuti, da[i])
		}
		da = tenuti
	}
	if fermato() { // ultima occasione prima di tradurre: lo schermo si muove ancora
		return nil, letturaAnnullata
	}
	risultati := make([]Risultato, len(da))
	var wg sync.WaitGroup
	posti := make(chan struct{}, 8)
	var scritte []string
	for _, b := range da {
		scritte = append(scritte, b.Testo)
	}
	ctx := ConSchermata(conLotto(context.Background()), scritte)
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
		return nil, letturaAnnullata
	}
	var cron []Risultato
	for i, b := range da {
		r := risultati[i]
		if strings.TrimSpace(r.Tradotto) == "" || strings.EqualFold(strings.TrimSpace(r.Tradotto), strings.TrimSpace(b.Testo)) {
			continue
		}
		if r.Motore != "memoria" && r.Motore != "catalogo" && r.Motore != "libreria" && r.Motore != "libreria (IA)" {
			nuove++
		}
		// anche sopra uno sfondo che si muove niente fascia nera: sfondo ricostruito dal gioco e
		// un'ombra sotto le lettere, così la scritta sembra del gioco e si legge lo stesso
		coloriBlocco(img, &b)
		b.Ombra = sfondoInMovimento(primaImg, img, b)
		b.Img = img
		contornoLettere(img, &b)
		ombraLettere(img, &b)
		b.LargMax, b.Bordo = liberoFinoAlBordoB(img, b)
		b.Tradotto = rifinisci(b.Testo, r.Tradotto)
		pronti = append(pronti, b)
		if len(pronti) <= 40 { // nella cronologia (F8), ma solo quando la pagina si vede davvero
			cron = append(cron, r)
		}
	}
	if esisteVoce(filepath.Join(dirApp, "logs", "salva-anteprima")) {
		var sb strings.Builder
		for i, b := range da {
			fmt.Fprintf(&sb, "%d,%d | %s => %s (%s)\n", b.X, b.Y, b.Testo, risultati[i].Tradotto, risultati[i].Motore)
		}
		_ = os.WriteFile(filepath.Join(dirApp, "logs", "pagina-tradotto.txt"), []byte(sb.String()), 0o644)
	}
	stabilizza(pronti, blocchiPagina())
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
		pronti[i].Marg = margineToppa(pronti[i], blocchi)
	}
	pulita := pulisciPagina(img, pronti)
	for i := range pronti {
		pronti[i].Pulita = pulita
		pronti[i].Tinta = tintaBlocco(img, pronti[i])
	}
	return &paginaPronta{area: area, img: img, pronti: pronti, nuove: nuove, t0: t0, tOCR: tOCR, tTrad: tTrad, cron: cron}, letturaFatta
}

// mostraPaginaPronta disegna le scritte tradotte sopra il gioco.
// scritteMosse: tra le due immagini i contorni netti sono cambiati fuori dalle zone che si
// muovono da sole. Se cambia solo la luce dello sfondo la lettura già fatta va bene: prima la
// pagina veniva letta due volte di fila nei menu con lo sfondo animato.
func scritteMosse(a, b *image.RGBA, animate []bool) bool {
	for c, si := range celleBordiCambiate(a, b) {
		if si && (c >= len(animate) || !animate[c]) {
			return true
		}
	}
	return false
}

// motivoLettura: perché è partita l'ultima lettura (solo per pagina-tempi.txt, controllo a mano).
var motivoLettura atomic.Value

func mostraPaginaPronta(p *paginaPronta, giro int64) bool {
	area, img, pronti, nuove, t0, tOCR, tTrad := p.area, p.img, p.pronti, p.nuove, p.t0, p.tOCR, p.tTrad
	if nuove > 0 {
		go func() { // su disco in sottofondo: non ritarda il disegno
			defer guard("salva-memoria")
			traduttore.SalvaMemoria()
		}()
	}
	if paginaAttiva.Load() && paginaGiro.Load() == giro {
		pronti = tieniFerme(pronti, blocchiPagina(), img)
		MostraPagina(area, pronti)
		if !paginaAttiva.Load() || paginaGiro.Load() != giro {
			NascondiPagina() // spento proprio adesso: niente scritte rimaste sullo schermo
			return false
		}
		paginaMostrata.Store(time.Now().UnixNano())
		for _, r := range p.cron { // adesso la pagina si vede: le sue scritte vanno nella cronologia
			aggiungiCronologia(r, true)
		}
		tempiPagina.Store(&TempiPagina{OCR: tOCR, Traduzione: tTrad, Totale: time.Since(t0), Visto: time.Unix(0, cambioVisto.Load()), Scritte: len(pronti), Nuove: nuove})
		ultimeScritte.Store(int64(len(pronti)))
		go func() {
			defer guard("anteprima")
			if !salvaAnteprime && !esisteVoce(filepath.Join(dirApp, "logs", "salva-anteprima")) {
				return // l'immagine del gioco non resta sul disco (privacy); per un controllo si
				// accende a mano creando il file logs\salva-anteprima
			}
			_ = salvaAnteprima(img, pronti, filepath.Join(dirApp, "logs", "pagina-anteprima.png"))
		}()
		if esisteVoce(filepath.Join(dirApp, "logs", "salva-anteprima")) { // controllo a mano: i tempi veri
			visto := time.Unix(0, cambioVisto.Load())
			motivo, _ := motivoLettura.Load().(string)
			riga := fmt.Sprintf("%s lettura=%dms traduzione=%dms totale=%dms dal-cambio=%dms scritte=%d nuove=%d motivo=%s\n",
				time.Now().Format("15:04:05.000"), tOCR.Milliseconds(), tTrad.Milliseconds(), time.Since(t0).Milliseconds(),
				time.Since(visto).Milliseconds(), len(pronti), nuove, motivo)
			if f, err := os.OpenFile(filepath.Join(dirApp, "logs", "pagina-tempi.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
				_, _ = f.WriteString(riga)
				_ = f.Close()
			}
		}
		// niente avviso di Windows a ogni schermata tradotta: nel gioco se ne accumulavano a decine
		// (Sergio, No Man's Sky, 24/11). Le scritte italiane al loro posto bastano
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

// linguaIlleggibile: se Steam dice che il gioco è in una lingua che Windows su questo PC non sa
// leggere (es. coreano senza il suo pacchetto), lo dico dentro il gioco e non parto: senza
// lettura non comparirebbe niente e sembrerebbe che Tasso non funzioni. Una volta per gioco e lingua.
type linguaSteamLetta struct {
	lingua string
	quando time.Time
}

var (
	muIlleggibile     sync.Mutex
	linguaSteamVista  = map[string]linguaSteamLetta{}
	avvisoIlleggibile = map[uintptr]time.Time{}
)

func linguaIlleggibile(fg uintptr) bool {
	g, ok := riconosciGioco(percorsoDiFinestra(fg))
	if !ok {
		return false
	}
	muIlleggibile.Lock()
	v, ok := linguaSteamVista[g.Cartella]
	if !ok || time.Since(v.quando) > 20*time.Second { // il tutto automatico chiede ogni secondo
		v = linguaSteamLetta{linguaSteam(g.Cartella), time.Now()}
		linguaSteamVista[g.Cartella] = v
	}
	muIlleggibile.Unlock()
	l := v.lingua
	if l == "" || l == linguaArrivo(leggiImp()) {
		return false
	}
	if _, ok := ocrPerLingua(l); ok {
		return false
	}
	n := nomeLingua(l)
	testo := testoTasso("Tasso: Windows non sa ancora leggere il "+n+". Aggiungilo in Impostazioni di Windows → Lingua, oppure metti il gioco in inglese.",
		"Tasso: Windows can't read "+n+" yet. Add it in Windows Settings → Language, or set the game to English.")
	muIlleggibile.Lock()
	gia := time.Since(avvisoIlleggibile[fg]) < 2*time.Minute
	if !gia {
		avvisoIlleggibile[fg] = time.Now()
	}
	muIlleggibile.Unlock()
	if area, ok := areaFinestra(fg); ok && !gia {
		mostraAvviso(area, testo, 20, 20, 8*time.Second)
	}
	dillaUnaVolta("ocr-"+l+g.Cartella, testoTasso("Il gioco "+g.Nome+" è in "+n+", ma Windows su questo PC non sa ancora leggere questa lingua. Aggiungi la lingua nelle Impostazioni di Windows (con il riconoscimento del testo), oppure metti il gioco in inglese da Steam: Proprietà → Lingua.",
		"The game "+g.Nome+" is in "+n+", but Windows on this PC can't read that language yet. Add it in Windows Settings (with text recognition), or switch the game to English in Steam: Properties → Language."))
	return true
}

// rileggiPiccole: le scritte piccole (testo delle notizie, descrizioni: lettere alte 10-15 pixel)
// l'OCR di Windows le legge male a grandezza naturale ("La cucitura curmt"). Ritaglio la zona
// dove stanno, la ingrandisco 3 volte e la rileggo: le righe nuove prendono il posto di quelle.
func rileggiPiccole(img *image.RGBA, righe []RigaOCR, f float64) []RigaOCR {
	if f <= 0 {
		f = 1
	}
	// zone separate (il pannello delle notizie, la scritta in basso...): ognuna letta da sola,
	// così resta piccola e si può ingrandire 3 volte
	var zone []image.Rectangle
	for _, r := range righe {
		x0, y0, x1, y1, h := rettRiga(r, f)
		if h > 0 && h < 17 {
			zone = append(zone, image.Rect(x0-12, y0-8, x1+12, y1+8))
		}
	}
	for cambiato := true; cambiato; {
		cambiato = false
		for i := 0; i < len(zone) && !cambiato; i++ {
			for j := i + 1; j < len(zone); j++ {
				a := zone[i]
				vicina := image.Rect(a.Min.X-40, a.Min.Y-40, a.Max.X+40, a.Max.Y+40)
				if vicina.Overlaps(zone[j]) {
					zone[i] = a.Union(zone[j])
					zone = append(zone[:j], zone[j+1:]...)
					cambiato = true
					break
				}
			}
		}
	}
	out := righe
	for _, z := range zone {
		out = rileggiZona(img, out, f, z.Intersect(img.Bounds()))
	}
	return out
}

func rileggiZona(img *image.RGBA, righe []RigaOCR, f float64, zona image.Rectangle) []RigaOCR {
	k := 3
	if zona.Dx()*k > 4000 || zona.Dy()*k > 4000 {
		k = 2
	}
	if zona.Dx()*k > 8000 || zona.Dy()*k > 8000 || zona.Dx() < 8 || zona.Dy() < 8 {
		return righe
	}
	ritaglio := image.NewRGBA(image.Rect(0, 0, zona.Dx(), zona.Dy()))
	draw.Draw(ritaglio, ritaglio.Bounds(), img, zona.Min, draw.Src)
	g := ingrandisci(grigio(ritaglio), k)
	contrasto(g)
	var buf bytes.Buffer
	if scriviBMP(&buf, g) != nil {
		return righe
	}
	nuove, err := ocr.LeggiPosizioni(buf.Bytes())
	if err != nil || len(nuove) == 0 {
		return righe
	}
	// coordinate del ritaglio ingrandito -> coordinate della prima lettura (scala f)
	for i := range nuove {
		for j := range nuove[i].Parole {
			w := &nuove[i].Parole[j]
			w.X = (w.X/float64(k) + float64(zona.Min.X)) * f
			w.Y = (w.Y/float64(k) + float64(zona.Min.Y)) * f
			w.W = w.W / float64(k) * f
			w.H = w.H / float64(k) * f
			w.T = lettereOCRStrane(w.T)
		}
	}
	var rettNuove []image.Rectangle
	for _, r := range nuove {
		x0, y0, x1, y1, _ := rettRiga(r, f)
		rettNuove = append(rettNuove, image.Rect(x0, y0, x1, y1))
	}
	var out []RigaOCR
	for _, r := range righe { // tengo le righe fuori dalla zona (e le grandi dentro: titoli)
		x0, y0, x1, y1, h := rettRiga(r, f)
		c := image.Pt((x0+x1)/2, (y0+y1)/2)
		if c.In(zona) && h < 17 {
			// la seconda lettura a volte salta una riga intera: se nessuna riga nuova sta
			// dove stava questa, tengo quella della prima lettura
			vecchia, coperta := image.Rect(x0, y0, x1, y1), false
			for _, n := range rettNuove {
				in := vecchia.Intersect(n)
				if in.Dx()*in.Dy()*10 >= vecchia.Dx()*vecchia.Dy()*3 {
					coperta = true
					break
				}
			}
			if coperta {
				continue
			}
		}
		out = append(out, r)
	}
	for _, r := range nuove {
		_, _, _, _, h := rettRiga(r, f)
		if h >= 17 { // le scritte grandi le aveva già lette bene la prima volta
			continue
		}
		out = append(out, r)
	}
	return out
}

// rettRiga: rettangolo della riga e altezza mediana delle parole, in pixel della cattura.
func rettRiga(r RigaOCR, f float64) (x0, y0, x1, y1 int, h float64) {
	if len(r.Parole) == 0 {
		return
	}
	a, b, c, d := 1e18, 1e18, -1e18, -1e18
	var hs []float64
	for _, w := range r.Parole {
		a, b = min(a, w.X), min(b, w.Y)
		c, d = max(c, w.X+w.W), max(d, w.Y+w.H)
		hs = append(hs, w.H/f)
	}
	sort.Float64s(hs)
	return int(a / f), int(b / f), int(c/f + 0.5), int(d/f + 0.5), hs[len(hs)/2]
}
