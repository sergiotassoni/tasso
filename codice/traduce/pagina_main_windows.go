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

// muAttiva: un F7 alla volta. Prima ogni pressione partiva su un filo suo: due F7 vicini
// potevano accendere due volte o lasciare lo stato a metà (e il terzo F7 sembrava non fare niente).
var (
	muAttiva       sync.Mutex
	ultimoF7       time.Time
	paginaUnaVolta atomic.Bool // la schermata tradotta è di una finestra che non è un gioco (a richiesta)
)

func attivaPagina() { attivaPaginaCome(false) }

// attivaPaginaCome: F7 accende o mette in pausa la traduzione della schermata. aRichiesta (tasto
// "ritraduci"): anche fuori da un gioco, ma una volta sola.
func attivaPaginaCome(aRichiesta bool) {
	muAttiva.Lock()
	defer muAttiva.Unlock()
	// rimbalzo del tasto (gancio + tasto rapido di Windows, o tasto premuto due volte di fila
	// per sbaglio): una pressione entro 250 ms dalla precedente non conta
	if time.Since(ultimoF7) < 250*time.Millisecond {
		return
	}
	ultimoF7 = time.Now()
	fg, _, _ := pGetForeground.Call()
	if paginaAttiva.Load() {
		paginaAttiva.Store(false)
		paginaGiro.Add(1)
		NascondiPagina()
		impostaPausa(exeDiFinestra(paginaGiocoA.Load()), true) // il tutto automatico non riparte da solo su questo gioco
		emetti("info", map[string]string{"testo": "In pausa: schermata di nuovo in inglese (" + nomeTasto("pagina") + " per riprendere)."})
		badgeStato(statoBadgeSpento) // anche nel gioco: F7 non deve sembrare morto
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
	gioco := finestraPerTradurre(fg)
	if !gioco && !aRichiesta {
		// desktop, browser, YouTube, programmi di lavoro: la traduzione dello schermo è solo per i
		// giochi (lì resta pronta ma non legge niente). Per tradurre qui una volta: il tasto zona o "ritraduci"
		emetti("info", map[string]string{"testo": "Qui non c'è un gioco: la traduzione dello schermo parte solo nei giochi. Per tradurre questa finestra una volta usa " + nomeTasto("traduci") + " o scegli una zona con " + nomeTasto("zona") + "."})
		badgeStato(statoBadgeNonGioco)
		return
	}
	paginaUnaVolta.Store(!gioco)
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
	votiPagina.svuota() // gioco nuovo (o ripartito): le letture vecchie non contano
	isteresiPagina.svuota()
	if exe := exeDiFinestra(fg); exe != giocoDelleFisse { // un altro gioco: le sue traduzioni fisse sono altre
		fissePagina.svuota()
		giocoDelleFisse = exe
	}
	paginaAttiva.Store(true)
	ov.Spegni() // la finestrella dei dialoghi non serve mentre traduco tutta la schermata
	giro := paginaGiro.Add(1)
	badgeStato(statoBadgeTraduco)
	go cicloPagina(giro)
}

func cicloPagina(giro int64) {
	defer guard("pagina")
	defer liberaCatturePronte() // pagina ferma: le immagini intere già pronte non restano in memoria
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
	mosse := 0                // giri di fila in cui la schermata si muoveva ancora
	erroriOCR := 0            // letture fallite di fila (errore della lettura di Windows, non "niente da tradurre")
	var fermoErrore time.Time // dopo tanti errori di fila non rileggo fino a qui
	fermo := false            // il giro prima ha finito il suo lavoro sull'ultima cattura
	var genCatt uint64        // fotogramma dell'ultima cattura (0 = non so)
	var imgCatt *image.RGBA
	var impCatt Impronta
	tinteViste := map[[4]int][12]uint8{} // ultima tinta controllata di ogni scritta (vedi blocchiDaRicolorare)
	var riprovaDopo time.Time            // rileggere a quest'ora (scritte rimaste senza traduzione)
	riprove := 0
	unaVolta := paginaUnaVolta.Load() // finestra che non è un gioco: una traduzione sola, a richiesta
	mostrataUnaVolta := false
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
			fermoErrore = time.Time{} // F10: riprovo subito anche dopo tanti errori
		}
		if !riprovaDopo.IsZero() && time.Now().After(riprovaDopo) {
			riprovaDopo = time.Time{}
			prima, perche = true, "traduttore-pronto"
		}
		if time.Now().Before(fermoErrore) {
			// la lettura di Windows continua a non funzionare: aspetto invece di riavviarla a
			// ogni giro (ogni tentativo può far partire PowerShell)
			time.Sleep(400 * time.Millisecond)
			continue
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
			if unaVolta && mostrataUnaVolta && !prima && imp.cambiata(ultima) && !soloLuce {
				// traduzione a richiesta fuori da un gioco: la schermata è cambiata, finita qui
				paginaAttiva.Store(false)
				NascondiPagina()
				return
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
					// le scritte già pronte (frasi pronte, memoria) si vedono subito, prima del traduttore
					e.p, e.esito = preparaPaginaCon(area, img, img, giro, annulla, func(p *paginaPronta) {
						if !annulla.Load() {
							mostraPaginaPronta(p, giro)
						}
					})
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
				if esito != letturaErrore && (rifare || (pronta == nil && esito != letturaNiente)) {
					// (dopo un errore della lettura non rilancio subito: ci riprova il giro dopo)
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
						if pronta.mancanti > 0 && riprove < 25 {
							// il traduttore non era ancora pronto per qualche scritta (appena acceso,
							// motore che si carica): riprovo presto invece di aspettare un cambio di schermata
							riprove++
							riprovaDopo = time.Now().Add(time.Duration(min(riprove, 6)) * 400 * time.Millisecond)
						} else if pronta.mancanti == 0 {
							riprove = 0
						}
						if unaVolta {
							mostrataUnaVolta = true
						}
						badgeStato(statoBadgeAttivo)
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
					if erroriOCR >= 4 {
						// errore che non passa: riprovo tra 45 secondi (o subito con F10), non
						// all'infinito ogni 3 secondi
						fermoErrore = time.Now().Add(45 * time.Second)
						erroriOCR = 2 // dopo l'attesa: altri 2 tentativi, poi di nuovo 45 secondi
						continue
					}
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
					// invece di rileggere lo schermo all'infinito. Restano solo quelle con le
					// lettere inglesi ancora lì, uguali: un fotogramma letto male (l'OCR non ha
					// trovato niente) non fa sparire tutto per un giro (sfarfallio, 26/11)
					resta := ancoraLi(img2, blocchiPagina())
					if len(resta) > 0 {
						MostraPagina(area, resta)
					} else {
						NascondiPagina()
					}
					ultima, ultimaColori, prima, ultimaImg = imp2, imp2, false, img2
					ultimeScritte.Store(int64(len(resta)))
					badgeStato(statoBadgeAttivo) // acceso, anche se qui non c'è niente da tradurre
				}
			} else if bl := blocchiPagina(); len(bl) > 0 {
				if imp.differenza(ultimaColori) > 0.2 {
					// scritte spostate che si ritrovano poco più in là: la traduzione le segue
					if seguite, n := seguiScritte(img, bl); n > 0 {
						bl = seguite
						MostraPagina(area, bl)
					}
				}
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
	// le scritte spostate (pagina che scorre, visuale che si sposta) che si ritrovano poco più
	// in là: la traduzione le segue invece di sparire e ricomparire dopo la lettura
	bl, seguite := seguiScritte(img, bl)
	if paginaSpostata(img, bl) {
		NascondiPagina()
		return
	}
	resto, tolte := togliCambiate(img, bl)
	switch {
	case tolte == 0:
		if seguite > 0 {
			MostraPagina(area, bl)
		}
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
	parziale    bool        // solo le scritte già pronte (frasi pronte, memoria): le altre arrivano dopo
	mancanti    int         // scritte senza traduzione perché il traduttore non era pronto: riprovare
	inglesi     []Blocco    // scritte lette che restano in inglese con parole vere (senzaMezzeRighe)
	letti       []Blocco    // tutte le scritte lette (senzaScritteCambiate)
}

// preparaPagina: legge tutte le scritte con la loro posizione e le traduce (senza disegnare).
// Senza niente da disegnare restituisce nil e il perché (esitoLettura). annulla (può essere nil):
// se diventa vero, dopo la lettura il lavoro si ferma senza tradurre (lo schermo si muove ancora).
func preparaPagina(area Zona, img, primaImg *image.RGBA, giro int64, annulla *atomic.Bool) (p *paginaPronta, esito esitoLettura) {
	return preparaPaginaCon(area, img, primaImg, giro, annulla, nil)
}

// preparaPaginaCon: come preparaPagina; anteprima (può essere nil) riceve subito le scritte già
// pronte dalle frasi pronte e dalla memoria, prima che il traduttore finisca le altre: dopo F7
// le scritte conosciute compaiono subito, senza aspettare il motore (o l'IA che si sta caricando).
func preparaPaginaCon(area Zona, img, primaImg *image.RGBA, giro int64, annulla *atomic.Bool, anteprima func(*paginaPronta)) (p *paginaPronta, esito esitoLettura) {
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
	righe, f, err := leggiPaginaOCR(img)
	if err != nil {
		avvisaErroreLettura("Lettura del testo: " + err.Error())
		return nil, letturaErrore
	}
	if fermato() {
		return nil, letturaAnnullata
	}
	// letture incerte rilette ingrandite, con il contrasto giusto (chiaro su scuro e viceversa)
	righe = rileggiIncerte(img, righe, f)
	if fermato() {
		return nil, letturaAnnullata
	}
	blocchi := blocchiLetti(img, righe, f, votiPagina, time.Now())
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
	segnaGiaMostrate(fissePagina, coppiaDi(traduttore), blocchi) // decisione già presa: resta (stabilita.go)
	controllo := esisteVoce(filepath.Join(dirApp, "logs", "salva-anteprima"))
	if controllo { // controllo a mano: cosa ha letto
		var sb strings.Builder
		for _, b := range blocchi {
			fmt.Fprintf(&sb, "%d,%d %dx%d righe=%d em=%.1f affid=%d motivo=%q | %s | letto: %s\n", b.X, b.Y, b.W, b.H, b.Righe, b.Em, b.Affid, motivoNonTradurre(b), b.Testo, b.Grezzo)
		}
		_ = os.WriteFile(filepath.Join(dirApp, "logs", "pagina-letto.txt"), []byte(sb.String()), 0o644)
	}
	nostra, ceNostra := nostraFinestraIn(area)
	tOCR := time.Since(t0)
	var da []Blocco
	var inglesi []Blocco // restano in inglese con parole vere: niente traduzioni accanto (mezze righe)
	for _, b := range blocchi {
		if ceNostra && dentro(b, nostra) { // è la finestra di Tasso Traduce sopra il gioco: non la traduco
			continue
		}
		if m := motivoNonTradurre(b); m != "" {
			if ingleseVisibile(m) {
				inglesi = append(inglesi, b)
			}
			continue // lettura incerta: resta l'inglese del gioco, pulito
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
	// colori, riquadro fino alle lettere vere, e righe incomplete (parola accanto non letta)
	completi := da[:0]
	for i := range da {
		if m := controllaCompleta(img, &da[i], blocchi); m != "" {
			if ingleseVisibile(m) {
				inglesi = append(inglesi, da[i])
			}
			continue
		}
		completi = append(completi, da[i])
	}
	da = completi
	if fermato() { // ultima occasione prima di tradurre: lo schermo si muove ancora
		return nil, letturaAnnullata
	}
	ctx := conLotto(context.Background())
	// primo giro: solo frasi pronte e memoria (subito); poi il traduttore per le altre
	risultati := traduciTutteFisse(ctx, traduttore, da, true, fissePagina)
	var daMotore []int
	for i, r := range risultati {
		if r.Errore == errSoloVeloce {
			daMotore = append(daMotore, i)
		}
	}
	costruisci := func(ris []Risultato) (pronti []Blocco, nuove, mancanti int, cron []Risultato, ingl []Blocco) {
		ingl = append([]Blocco(nil), inglesi...)
		for i, b := range da {
			r := ris[i]
			if m := motivoScartaTraduzione(b, r); m != "" {
				if ingleseVisibile(m) { // anche "in attesa del traduttore": per ora si vede l'inglese
					ingl = append(ingl, b)
				}
				if strings.TrimSpace(r.Tradotto) == "" && r.Errore != "" && r.Errore != errSoloVeloce {
					mancanti++ // traduttore non pronto o errore: riprovo appena possibile
				}
				continue
			}
			sc := preparaScritta(img, primaImg, b, r)
			// suggerimenti dei tasti: la traduzione sta nel suo riquadro (più piccola o più
			// corta), se no resta l'inglese (riquadro.go)
			if m := adattaAlRiquadro(&sc); m != "" {
				ingl = append(ingl, b)
				continue
			}
			// la traduzione si fissa (fisse.go) solo quando la scritta si vede davvero: in
			// mostraPaginaPronta, dopo tutti i filtri
			sc.DaFissare = daFissare(b, r.Tradotto)
			if r.Motore != "memoria" && r.Motore != "catalogo" && r.Motore != "libreria" && r.Motore != "libreria (IA)" {
				nuove++
			}
			pronti = append(pronti, sc)
			if len(pronti) <= 40 { // nella cronologia (F8), ma solo quando la pagina si vede davvero
				cron = append(cron, r)
			}
		}
		tenute := senzaSovrapposte(pronti) // mai due traduzioni una sopra l'altra
		for _, p := range pronti {
			if !contieneBlocco(tenute, p) {
				ingl = append(ingl, p)
			}
		}
		// mai una scritta tradotta accanto a un pezzo della stessa frase rimasto in inglese
		pronti, tolte := senzaMezzeRighe(tenute, ingl)
		ingl = append(ingl, tolte...)
		stabilizza(pronti, blocchiPagina())
		rifinisciPagina(img, pronti, blocchi)
		return
	}
	if len(daMotore) > 0 && anteprima != nil && len(daMotore) < len(da) && !fermato() &&
		paginaAttiva.Load() && paginaGiro.Load() == giro {
		if pr, n, _, _, ingl := costruisci(risultati); len(pr) > 0 {
			anteprima(&paginaPronta{area: area, img: img, pronti: pr, nuove: n, t0: t0, tOCR: tOCR, tTrad: time.Since(t0) - tOCR, parziale: true, inglesi: ingl, letti: blocchi})
		}
	}
	if len(daMotore) > 0 {
		sotto := make([]Blocco, len(daMotore))
		for k, i := range daMotore {
			sotto[k] = da[i]
		}
		r2 := traduciTutteFisse(ctx, traduttore, sotto, false, fissePagina)
		for k, i := range daMotore {
			risultati[i] = r2[k]
		}
	}
	// file di pulsanti lette come una riga e scartate come mezze: ogni voce per conto suo
	da, risultati = riprovaFile(ctx, traduttore, img, da, risultati, blocchi)
	tTrad := time.Since(t0) - tOCR
	if !paginaAttiva.Load() || paginaGiro.Load() != giro {
		return nil, letturaAnnullata
	}
	pronti, nuove, mancanti, cron, ingl := costruisci(risultati)
	if controllo {
		var sb strings.Builder
		for i, b := range da {
			fmt.Fprintf(&sb, "%d,%d | %s => %s (%s) %s\n", b.X, b.Y, b.Testo, risultati[i].Tradotto, risultati[i].Motore, motivoScartaTraduzione(b, risultati[i]))
		}
		_ = os.WriteFile(filepath.Join(dirApp, "logs", "pagina-tradotto.txt"), []byte(sb.String()), 0o644)
	}
	return &paginaPronta{area: area, img: img, pronti: pronti, nuove: nuove, t0: t0, tOCR: tOCR, tTrad: tTrad, cron: cron, mancanti: mancanti, inglesi: ingl, letti: blocchi}, letturaFatta
}

// rileggiIncerte: le righe lette male rilette ingrandite e preparate in tre modi, con una sola
// chiamata all'OCR di Windows (rilettura.go).
func rileggiIncerte(img *image.RGBA, righe []RigaOCR, f float64) []RigaOCR {
	return rileggiIncerteCon(img, righe, f, func(g *image.Gray, _ []image.Rectangle) ([]RigaOCR, error) {
		var buf bytes.Buffer
		if err := scriviBMP(&buf, g); err != nil {
			return nil, err
		}
		return ocr.LeggiPosizioni(buf.Bytes())
	})
}

// leggiPaginaOCR: tutta la schermata letta dall'OCR di Windows (scala f), più le scritte piccole
// rilette ingrandite.
func leggiPaginaOCR(img *image.RGBA) ([]RigaOCR, float64, error) {
	var buf bytes.Buffer
	f, err := preparaPaginaPerOCR(img, &buf)
	if err != nil {
		return nil, 0, err
	}
	righe, err := ocr.LeggiPosizioni(buf.Bytes())
	if err != nil {
		return nil, 0, err
	}
	return rileggiPiccole(img, righe, f), f, nil
}

// mostraPaginaPronta disegna le scritte tradotte sopra il gioco.
// scritteMosse: tra le due immagini i contorni netti sono cambiati fuori dalle zone che si
// muovono da sole. Se cambia solo la luce dello sfondo la lettura già fatta va bene: prima la
// pagina veniva letta due volte di fila nei menu con lo sfondo animato.
// giocoDelleFisse: il programma del gioco a cui valgono le traduzioni fisse di adesso.
var giocoDelleFisse string

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
		vecchi := blocchiPagina()
		// isteresi (isteresi.go): una traduzione già mostrata non sparisce e non cambia per una
		// sola lettura peggiore; un'altra traduzione nello stesso posto solo se confermata
		pronti, ingl := isteresiPagina.applica(pronti, vecchi, p.inglesi, img, time.Now())
		pronti = tieniFerme(pronti, vecchi, img)
		pronti = recuperaPerse(pronti, vecchi, img, ingl) // scritte saltate dalla lettura ma ancora lì: restano
		// mai la traduzione vecchia su un'altra scritta (la barra delle schede che scorre)
		pronti = senzaScritteCambiate(pronti, p.pronti, p.letti)
		// anche le traduzioni rimaste dalla lettura prima: mai accanto a un pezzo della stessa
		// frase che adesso resta in inglese
		pronti, _ = senzaMezzeRighe(pronti, ingl)
		MostraPagina(area, pronti)
		if !paginaAttiva.Load() || paginaGiro.Load() != giro {
			NascondiPagina() // spento proprio adesso: niente scritte rimaste sullo schermo
			return false
		}
		fissaMostrate(fissePagina, coppiaDi(traduttore), pronti) // da adesso si traducono sempre così
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
