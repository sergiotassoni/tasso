//go:build windows

package traduce

// Finestra della modalità "tutta la schermata" (F6): grande come la finestra del gioco,
// trasparente, non cliccabile, esclusa dalle catture. Sopra ogni scritta inglese disegna un
// rettangolo con il colore dello sfondo del gioco e la traduzione italiana con il colore del testo.
//
// Niente sfarfallio: tutto si disegna prima in un'immagine nascosta grande come la finestra
// (fuori da WM_PAINT, e solo le scritte cambiate), poi WM_PAINT la copia sullo schermo con una
// sola operazione. Sullo schermo non si vede mai una pagina a metà.

import (
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmPagina   = wmApp + 10
	chiaveTrsp = 0x00FF00FF // magenta: dove la finestra è di questo colore, si vede il gioco
)

var pag struct {
	mu      sync.Mutex
	hwnd    uintptr
	area    Zona
	blocchi []Blocco
	vis     bool
	chiave  uintptr
	inCoda  atomic.Bool // c'è già un wmPagina in attesa: non ne mando un altro
}

var (
	pUpdateWindow = user32.NewProc("UpdateWindow")
	pCreateRgn    = gdi32.NewProc("CreateRectRgn")
	pCombineRgn   = gdi32.NewProc("CombineRgn")
	pSelectClip   = gdi32.NewProc("SelectClipRgn")
)

// muDisegno protegge tutto quello che serve per disegnare: caratteri, toppe, impaginazioni
// già calcolate, l'immagine nascosta (tela) e inAnteprima.
var muDisegno sync.Mutex

func creaFinestraPagina() {
	classe, _ := windows.UTF16PtrFromString("TassoTraducePagina")
	wc := wndClassEx{lpfnWndProc: windows.NewCallback(pagProc), lpszClassName: classe}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	ex := uintptr(0x80000 | 0x20 | 0x8 | 0x80 | 0x08000000) // LAYERED|TRANSPARENT|TOPMOST|TOOLWINDOW|NOACTIVATE
	h, _, _ := pCreateWindowEx.Call(ex, uintptr(unsafe.Pointer(classe)), 0, 0x80000000, 0, 0, 100, 100, 0, 0, 0, 0)
	if h == 0 {
		return
	}
	pSetLayeredAttr.Call(h, chiaveTrsp, 255, 1|2) // LWA_COLORKEY | LWA_ALPHA: pieno, l'inglese sotto non deve trasparire
	pSetDisplayAffin.Call(h, affinita())          // WDA_EXCLUDEFROMCAPTURE: non rileggo le mie scritte
	k, _, _ := pCreateBrush.Call(chiaveTrsp)
	pag.mu.Lock()
	pag.hwnd, pag.chiave = h, k
	pag.mu.Unlock()
}

// chiediDisegno: chiede alla finestra di ridisegnarsi. Se una richiesta è già in coda basta
// quella (legge i dati più nuovi quando arriva): niente ridisegni inutili uno dietro l'altro.
func chiediDisegno(h uintptr) {
	if h == 0 || !pag.inCoda.CompareAndSwap(false, true) {
		return
	}
	if r, _, _ := pPostMessage.Call(h, wmPagina, 0, 0); r == 0 {
		pag.inCoda.Store(false)
	}
}

// blocchiPagina: copia delle scritte mostrate adesso.
func blocchiPagina() []Blocco {
	pag.mu.Lock()
	defer pag.mu.Unlock()
	return append([]Blocco(nil), pag.blocchi...)
}

// MostraPagina disegna le traduzioni (coordinate dei blocchi relative all'area).
func MostraPagina(area Zona, b []Blocco) {
	pag.mu.Lock()
	pag.area, pag.blocchi, pag.vis = area, b, true
	h := pag.hwnd
	pag.mu.Unlock()
	chiediDisegno(h)
}

// sostituisciTraduzione: cambia il testo delle scritte già mostrate che dicono "vecchio"
// (correzione arrivata dall'IA). Ridisegna solo se la pagina è visibile.
func sostituisciTraduzione(inglese, vecchio, nuovo string) bool {
	pag.mu.Lock()
	cambiate := false
	if pag.vis {
		nuovi := append([]Blocco(nil), pag.blocchi...)
		for i := range nuovi {
			if nuovi[i].Tradotto == vecchio && strings.EqualFold(strings.Join(strings.Fields(pulisci(nuovi[i].Testo)), " "), inglese) {
				nuovi[i].Tradotto = nuovo
				cambiate = true
			}
		}
		if cambiate {
			pag.blocchi = nuovi
		}
	}
	h := pag.hwnd
	pag.mu.Unlock()
	if cambiate {
		chiediDisegno(h)
	}
	return cambiate
}

// NascondiPagina toglie le scritte e le dimentica: un ricolora dopo non le rimette sullo schermo.
func NascondiPagina() {
	pag.mu.Lock()
	pag.vis, pag.blocchi = false, nil
	h := pag.hwnd
	pag.mu.Unlock()
	chiediDisegno(h)
}

// famiglieFont: 0 normale; 1 e 2 strette, per i giochi che scrivono con lettere strette
// (Liar's Bar): la traduzione ha la stessa forma e ci sta nello stesso spazio.
var famiglieFont = []string{"Segoe UI", "Bahnschrift SemiCondensed", "Bahnschrift Condensed"}

var fontCache = map[int32]uintptr{} // solo con muDisegno preso

func fontPagina(px int32, peso uintptr) uintptr { return fontPaginaF(px, peso, 0) }

func fontPaginaF(px int32, peso uintptr, fam int) uintptr {
	chiave := (px*10+int32(peso/100))*4 + int32(fam)
	if f, ok := fontCache[chiave]; ok {
		return f
	}
	face, _ := windows.UTF16PtrFromString(famiglieFont[fam])
	f, _, _ := pCreateFont.Call(uintptr(-px), 0, 0, 0, peso, 0, 0, 0, 1, 0, 0, 4, 0, uintptr(unsafe.Pointer(face))) // ANTIALIASED (grigi): niente frange colorate
	fontCache[chiave] = f
	return f
}

func colore(c [3]uint8) uintptr { return uintptr(c[0]) | uintptr(c[1])<<8 | uintptr(c[2])<<16 }

// testoW: il testo per DrawText, sempre con almeno lo zero finale. Un carattere nullo dentro la
// traduzione faceva fallire la conversione e chiudeva Tasso (&t[0] su una lista vuota).
func testoW(s string) []uint16 {
	t, err := windows.UTF16FromString(strings.ReplaceAll(s, "\x00", ""))
	if err != nil || len(t) == 0 {
		return []uint16{0}
	}
	return t
}

const (
	dtSingleLine  = 0x20
	dtEditControl = 0x2000 // non mostra un'ultima riga tagliata a metà
	dtEndEllipsis = 0x8000 // "…" alla fine se non ci sta
)

// misuraTesto: larghezza e altezza del testo (a capo dentro la larghezza larg se aCapo).
func misuraTesto(hdc, font uintptr, s string, larg int32, aCapo bool) (int32, int32) {
	pSelectObject.Call(hdc, font)
	r := rect{0, 0, larg, 0}
	t := testoW(s)
	fl := uintptr(dtCalcRect | dtNoPrefix)
	if aCapo {
		fl |= dtWordBreak
	} else {
		fl |= dtSingleLine
	}
	pDrawText.Call(hdc, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), uintptr(unsafe.Pointer(&r)), fl)
	return r.Right - r.Left, r.Bottom - r.Top
}

// impaginazione: dove e come va la traduzione di una scritta. Si calcola una volta sola per
// scritta (le misure per tentativi costano) e si riusa a ogni ridisegno.
type impaginazione struct {
	font           uintptr
	px             int32
	w, h           int32
	tx, ty         int32
	m              int32
	x0, y0, x1, y1 int32
	toppa          image.Rectangle // quello che copre la toppa dalla pagina pulita
	rt             rect            // rettangolo della scritta (DrawText non esce mai da qui)
	fl             uintptr
	zona           image.Rectangle // tutto quello che questa scritta può toccare sulla finestra
}

// chiaveDisegno: la scritta senza i puntatori alle immagini (cambiano a ogni lettura anche
// quando il disegno è identico) e senza la tinta. Se la chiave non cambia, il disegno della
// scritta non cambia (la cattura e la pagina pulita nuove hanno gli stessi pixel lì).
type chiaveDisegno struct {
	b           Blocco
	img, pulita bool
	imgW        int
}

func chiaveDi(b Blocco) chiaveDisegno {
	k := chiaveDisegno{img: b.Img != nil, pulita: b.Pulita != nil}
	if b.Img != nil {
		k.imgW = b.Img.Bounds().Dx()
	}
	b.Img, b.Pulita, b.Tinta = nil, nil, [12]uint8{} // la tinta non cambia il disegno
	k.b = b
	return k
}

var impaginazioni = map[chiaveDisegno]impaginazione{} // solo con muDisegno preso

func impaginaCache(hdc uintptr, b Blocco) impaginazione {
	k := chiaveDi(b)
	if l, ok := impaginazioni[k]; ok {
		return l
	}
	if len(impaginazioni) > 600 {
		impaginazioni = map[chiaveDisegno]impaginazione{}
	}
	l := impagina(hdc, b)
	impaginazioni[k] = l
	return l
}

// impagina: grandezza, carattere e posto della traduzione. Regola del proprietario: la scritta
// italiana resta SEMPRE dentro il riquadro dell'originale. Una riga sola non va mai a capo (finiva
// sulla voce sotto: "Gear" → "Equipaggiamento"): prima rimpicciolisce un po', poi prova le
// lettere strette, poi rimpicciolisce ancora, e solo alla fine mette i puntini. Un paragrafo
// rimpicciolisce finché sta nel suo spazio in basso; se proprio non ci sta, viene tagliato
// all'ultima riga intera.
func impagina(hdc uintptr, b Blocco) impaginazione {
	var l impaginazione
	righe := int32(max(1, b.Righe))
	altRiga := int32(b.H) / righe
	px := int32(b.Em + 0.5)
	if px <= 0 || px > altRiga*16/10 {
		px = altRiga * 115 / 100
	}
	if px < 10 {
		px = 10
	}
	if b.PxColonna > 0 {
		px = int32(b.PxColonna)
	}
	imgW := int32(0)
	if b.Img != nil {
		imgW = int32(b.Img.Bounds().Dx())
	}
	larg, maxLarg := int32(b.W), int32(largConsentita(b))
	if b.PxColonna > 0 && b.ColLarg > 0 {
		larg = int32(max(b.W, b.ColLarg)) // nella colonna si va a capo alla sua larghezza
	}
	if b.Centro && righe == 1 {
		maxLarg = larg + 2*(maxLarg-larg) // centrata: cresce dai due lati
		if b.Img != nil {                 // ma non oltre i bordi dello schermo
			cx := int32(b.X + b.W/2)
			maxLarg = min(maxLarg, 2*min(cx, imgW-cx)-16)
		}
	}
	if righe > 1 && b.Em < 20 && b.PxColonna == 0 {
		// testo piccolo su più righe (notizie, descrizioni): la riga italiana non deve essere
		// più alta di quella del gioco, se no il paragrafo esce dal suo spazio
		px = min(px, max(9, altRiga*80/100))
	}
	minPx := max(10, px*78/100)
	if righe > 1 && b.Em < 20 {
		minPx = max(9, px*66/100)
	}
	if b.PxColonna > 0 {
		minPx = max(9, px*85/100) // tutta la colonna resta quasi della stessa grandezza
		if righe > 1 {
			minPx = max(9, px*78/100) // un paragrafo non deve uscire dal suo spazio
		}
	}
	if righe == 1 {
		// una riga sola (voce di menu, etichetta): meglio un po' più piccola che andare a capo
		// sopra la riga sotto (visto nelle foto del 22/11: colonne accavallate)
		minPx = max(9, px*62/100)
		if float64(b.W) > 20*b.Em && !b.Centro {
			// frase lunga: resta quasi nel suo riquadro (non esce dal pannello del gioco verso
			// destra); se serve rimpicciolisce un po'
			maxLarg = min(maxLarg, larg*112/100)
		}
	}
	maxLarg = max(maxLarg, 8)
	minPx = min(minPx, px)
	// sotto questa grandezza non si legge più: meglio i puntini o l'ultima riga tagliata
	pavimento := min(minPx, max(7, px*45/100))
	peso := b.pesoCarattere()
	// lettere del gioco strette? Misuro l'inglese con il carattere normale: se viene molto più
	// largo dell'originale, uso un carattere stretto (stessa forma del gioco)
	fam := 0
	if testoRif, wRif := b.Testo, b.W; testoRif != "" && wRif > 0 {
		if b.ColRif != "" && b.ColRifW > 0 { // colonna: lo stesso carattere per tutte le righe
			testoRif, wRif = b.ColRif, b.ColRifW
		}
		wIng, _ := misuraTesto(hdc, fontPaginaF(px, peso, 0), testoRif, 0, false)
		spazio := float64(wRif)
		if righe > 1 && b.ColRif == "" {
			spazio = float64(b.W) * float64(righe) * 0.9
		}
		switch r := float64(wIng) / spazio; {
		case r > 1.45:
			fam = 2
		case r > 1.32:
			fam = 1
		}
	}
	var w, h int32
	var font uintptr
	ellissi, aCapo := false, false
	limH := int32(0)
	if righe == 1 && b.Img == nil {
		// avviso di Tasso (non una scritta del gioco): un po' più piccolo, poi va a capo come
		// prima, così il messaggio si legge tutto
		for prova := px; ; prova-- {
			font = fontPaginaF(prova, peso, fam)
			w, h = misuraTesto(hdc, font, b.Tradotto, 0, false)
			if w <= maxLarg || prova <= minPx {
				if w > maxLarg {
					w, h = misuraTesto(hdc, font, b.Tradotto, maxLarg, true)
					aCapo = true
				}
				break
			}
		}
	} else if righe == 1 {
		trovato := false
		// stessa grandezza del gioco; se l'italiano è più lungo uso lo spazio libero a destra
		// (solo dentro il riquadro), poi rimpicciolisco un po', poi lettere più strette
		for f := fam; f < len(famiglieFont) && !trovato; f++ {
			for prova := px; prova >= minPx; prova-- {
				font = fontPaginaF(prova, peso, f)
				w, h = misuraTesto(hdc, font, b.Tradotto, 0, false)
				if w <= maxLarg {
					trovato = true
					break
				}
			}
		}
		// ancora troppo lunga: le lettere più strette, sempre più piccole
		for prova := minPx - 1; !trovato && prova >= pavimento; prova-- {
			font = fontPaginaF(prova, peso, len(famiglieFont)-1)
			w, h = misuraTesto(hdc, font, b.Tradotto, 0, false)
			trovato = w <= maxLarg
		}
		if !trovato { // nemmeno così: i puntini alla fine, mai fuori dal riquadro
			ellissi = true
			w = maxLarg
		}
	} else {
		// stessa grandezza del gioco finché c'è posto sotto; poi rimpicciolisco finché il
		// paragrafo sta nel suo spazio (mai sopra la scritta sotto)
		limH = int32(b.AltMax)
		if limH <= 0 {
			limH = int32(b.H) * 115 / 100
		}
		limH = max(limH, int32(b.H))
		for prova := px; ; prova-- {
			font = fontPaginaF(prova, peso, fam)
			w, h = misuraTesto(hdc, font, b.Tradotto, larg, true)
			if h <= limH || prova <= pavimento {
				break
			}
		}
		if h > limH {
			ellissi = true
			h = limH
		}
	}
	// dove va la traduzione: allineata a sinistra come l'originale, o centrata sullo stesso centro
	tx := int32(b.X)
	if b.Centro && righe == 1 {
		tx = int32(b.X+b.W/2) - w/2
	}
	if b.Img != nil { // mai fuori dallo schermo
		tx = max(4, min(tx, imgW-w-4))
	}
	m := int32(3)
	if b.Img != nil && !b.Ombra { // toppa con i pixel veri: posso allargarla senza coprire nulla
		m = max(3, int32(b.Em*0.2+0.5))
		if b.Marg > 0 {
			m = int32(b.Marg)
		}
	}
	x0 := min(int32(b.X), tx) - m
	x1 := max(int32(b.X)+larg, tx+w) + m
	y0 := int32(b.Y) - m
	y1 := y0 + max(int32(b.H), h) + 2*m
	if righe == 1 && b.Img != nil && !b.Ombra {
		// solo l'altezza delle lettere inglesi: la toppa non deve arrivare sulla voce di sotto
		// (copierebbe le sue lettere inglesi sopra la sua traduzione)
		y1 = int32(b.Y+b.H) + m
	}
	pSelectObject.Call(hdc, font)
	ty := int32(b.Y) + (int32(b.H)-h)/2*boolI(h < int32(b.H))
	if righe == 1 && !aCapo {
		// stessa linea di base dell'originale (la cella del carattere ha spazio sopra le lettere)
		var tm [60]byte // TEXTMETRICW: tmHeight, tmAscent…
		if r, _, _ := pGetTextMetrics.Call(hdc, uintptr(unsafe.Pointer(&tm[0]))); r != 0 {
			asc := int32(binary.LittleEndian.Uint32(tm[4:]))
			ty = int32(lineaDiBase(b)) - asc
		}
	}
	// la toppa copre anche tutta la scritta nuova (gambe sotto comprese): niente bordi colorati
	// attorno alle lettere
	testo := image.Rect(int(tx)-2, int(ty)-2, int(tx+w)+2, int(ty+h)+2)
	if b.Centro && righe > 1 {
		testo = image.Rect(int(x0), int(ty)-2, int(x1), int(ty+h)+2)
	}
	toppa := image.Rect(int(x0), int(y0), int(x1), int(y1))
	if b.Pulita != nil {
		toppa = toppa.Union(testo)
	}
	fl := uintptr(dtWordBreak | dtNoPrefix | dtLeft)
	rt := rect{tx, ty, x1 - m, max(y1, ty+h+2)}
	if b.Centro && righe > 1 {
		rt, fl = rect{x0 + m, ty, x1 - m, y1}, dtWordBreak|dtNoPrefix|0x1 // DT_CENTER
	}
	if righe == 1 && !aCapo {
		fl = dtSingleLine | dtNoPrefix | dtLeft // una riga sola non va mai a capo
		if ellissi {
			fl |= dtEndEllipsis
			rt.Right = tx + maxLarg
		}
	} else if ellissi {
		fl |= dtEditControl // l'ultima riga che non ci sta tutta non si vede
		rt.Bottom = ty + limH
	}
	marg := 8 // bordino, ombra e margine della scritta composta
	zona := toppa.Union(image.Rect(int(rt.Left)-marg, int(rt.Top)-marg, int(rt.Right)+marg, int(rt.Bottom)+marg))
	l = impaginazione{font: font, px: px, w: w, h: h, tx: tx, ty: ty, m: m, x0: x0, y0: y0, x1: x1, y1: y1,
		toppa: toppa, rt: rt, fl: fl, zona: zona.Inset(-2)}
	return l
}

// fase 0: sfondo, fase 1: scritta. Prima tutti gli sfondi, poi tutte le scritte, così uno
// sfondo non taglia le lettere della riga sopra. Solo con muDisegno preso.
func disegnaBlocco(hdc uintptr, b Blocco, fase int) {
	if b.Tradotto == "" {
		return
	}
	l := impaginaCache(hdc, b)
	if fase == 0 {
		if b.Pulita != nil {
			disegnaToppaPulita(hdc, b, l.toppa, image.Rectangle{})
			return
		}
		disegnaSfondo(hdc, b, l.x0, l.y0, l.x1, l.y1)
		return
	}
	pSelectObject.Call(hdc, l.font)
	t := testoW(b.Tradotto)
	rt, fl := l.rt, l.fl
	if b.Pulita != nil {
		sp := 1
		if l.px >= 26 {
			sp = 2
		}
		disegnaTestoComposto(hdc, l.font, b, t, rt, fl, sp)
		return
	}
	if b.Ombra { // ombra morbida sotto le lettere, come nei menu dei giochi sopra le immagini
		om := contrastante(b.Inchiostro)
		pSetTextColor.Call(hdc, colore([3]uint8{om.R, om.G, om.B}))
		for _, d := range [][2]int32{{1, 1}, {2, 2}, {-1, 1}, {1, -1}} {
			r2 := rect{rt.Left + d[0], rt.Top + d[1], rt.Right + d[0], rt.Bottom + d[1]}
			pDrawText.Call(hdc, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), uintptr(unsafe.Pointer(&r2)), fl)
		}
	}
	if b.HaContorno && !b.Ombra { // lettere bordate come nel gioco
		pSetTextColor.Call(hdc, colore([3]uint8{b.Contorno.R, b.Contorno.G, b.Contorno.B}))
		sp := int32(1)
		if l.px >= 26 {
			sp = 2
		}
		for _, d := range [8][2]int32{{-1, -1}, {0, -1}, {1, -1}, {-1, 0}, {1, 0}, {-1, 1}, {0, 1}, {1, 1}} {
			r2 := rect{rt.Left + d[0]*sp, rt.Top + d[1]*sp, rt.Right + d[0]*sp, rt.Bottom + d[1]*sp}
			pDrawText.Call(hdc, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), uintptr(unsafe.Pointer(&r2)), fl)
		}
	}
	pSetTextColor.Call(hdc, colore([3]uint8{b.Inchiostro.R, b.Inchiostro.G, b.Inchiostro.B}))
	pDrawText.Call(hdc, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), uintptr(unsafe.Pointer(&rt)), fl)
}

// disegnaSfondo copre l'inglese nel rettangolo x0..x1, y0..y1: con lo sfondo ricostruito dal
// gioco (le sue sfumature) dove c'è, con il colore dello sfondo solo dove manca.
func disegnaSfondo(hdc uintptr, b Blocco, x0, y0, x1, y1 int32) {
	if b.Img != nil && !b.Ombra {
		// pixel veri del gioco, cancellate solo le lettere inglesi: bordi e caselle restano intatti
		k := chiaveToppa{b.Img, b.X, b.Y, b.W, b.H, x0, y0, x1, y1}
		t, ok := toppe[k]
		if !ok {
			if len(toppe) > 400 {
				toppe = map[chiaveToppa]toppa{}
			}
			t.f, t.x, t.y = fondoPreciso(b, int(x0), int(y0), int(x1), int(y1))
			toppe[k] = t
		}
		f, fx, fy := t.f, t.x, t.y
		if f != nil {
			disegnaImmagine(hdc, f, int32(fx), int32(fy))
			return
		}
	}
	sf := [3]uint8{b.Sfondo.R, b.Sfondo.G, b.Sfondo.B}
	if colore(sf) == chiaveTrsp {
		sf[0]-- // non deve coincidere con il colore trasparente
	}
	fx0, fy0, fw, fh := int32(0), int32(0), int32(0), int32(0)
	if b.Fondo != nil {
		fox := b.FondoX
		if fox <= 0 {
			fox = 3
		}
		fx0, fy0 = int32(b.X-fox), int32(b.Y)-3
		fw, fh = int32(b.Fondo.Bounds().Dx()), int32(b.Fondo.Bounds().Dy())
	}
	// colore pieno solo dove il fondo ricostruito non arriva
	if b.Fondo == nil || x0 < fx0 || x1 > fx0+fw || y0 < fy0 || y1 > fy0+fh {
		pen, _, _ := pCreateBrush.Call(colore(sf))
		if pen != 0 {
			r := rect{x0, y0, x1, y1}
			pFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), pen)
			pDeleteObject.Call(pen)
		}
	}
	if b.Fondo == nil {
		return
	}
	// solo la parte che serve: il resto del fondo ricostruito coprirebbe il gioco per niente
	cx0, cy0 := max(x0, fx0), max(y0, fy0)
	cx1, cy1 := min(x1, fx0+fw), min(y1, fy0+fh)
	cw, ch := cx1-cx0, cy1-cy0
	if cw <= 0 || ch <= 0 {
		return
	}
	bgra := make([]byte, int(cw*ch)*4)
	for y := int32(0); y < ch; y++ {
		for x := int32(0); x < cw; x++ {
			c := b.Fondo.RGBAAt(int(cx0-fx0+x), int(cy0-fy0+y))
			i := int(y*cw+x) * 4
			bgra[i], bgra[i+1], bgra[i+2], bgra[i+3] = c.B, c.G, c.R, 255
			if uintptr(c.R)|uintptr(c.G)<<8|uintptr(c.B)<<16 == chiaveTrsp {
				bgra[i+2]--
			}
		}
	}
	bi := bitmapInfoHeader{biWidth: cw, biHeight: -ch, biPlanes: 1, biBitCount: 32}
	bi.biSize = uint32(unsafe.Sizeof(bi))
	pSetDIBitsToDevice.Call(hdc, uintptr(cx0), uintptr(cy0), uintptr(cw), uintptr(ch), 0, 0, 0, uintptr(ch),
		uintptr(unsafe.Pointer(&bgra[0])), uintptr(unsafe.Pointer(&bi)), 0)
}

// toppe già calcolate (la finestra si ridisegna spesso): solo con muDisegno preso
type chiaveToppa struct {
	img            *image.RGBA
	x, y, w, h     int
	x0, y0, x1, y1 int32
}
type toppa struct {
	f    *image.RGBA
	x, y int
}

var toppe = map[chiaveToppa]toppa{}

// inAnteprima: sto disegnando la foto di prova (sopra la cattura, non sopra il gioco): niente
// trasparenza, la cattura sotto è già il gioco. Solo con muDisegno preso.
var inAnteprima bool

func disegnaImmagine(hdc uintptr, f *image.RGBA, x, y int32) {
	w, h := int32(f.Bounds().Dx()), int32(f.Bounds().Dy())
	if w <= 0 || h <= 0 {
		return
	}
	bgra := make([]byte, int(w*h)*4)
	for i := 0; i < len(bgra); i += 4 {
		bgra[i], bgra[i+1], bgra[i+2], bgra[i+3] = f.Pix[i+2], f.Pix[i+1], f.Pix[i], 255
		if uintptr(f.Pix[i])|uintptr(f.Pix[i+1])<<8|uintptr(f.Pix[i+2])<<16 == chiaveTrsp {
			bgra[i+2]--
		}
	}
	bi := bitmapInfoHeader{biWidth: w, biHeight: -h, biPlanes: 1, biBitCount: 32}
	bi.biSize = uint32(unsafe.Sizeof(bi))
	pSetDIBitsToDevice.Call(hdc, uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0, 0, 0, uintptr(h),
		uintptr(unsafe.Pointer(&bgra[0])), uintptr(unsafe.Pointer(&bi)), 0)
}

func boolI(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

// tela: l'immagine nascosta dove si prepara la pagina. Solo con muDisegno preso.
var tela struct {
	mem, bmp, vecchio uintptr
	w, h              int32
	zone              map[chiaveDisegno]image.Rectangle // le scritte disegnate adesso e dove (nil = da rifare tutta)
	mostrata          bool
}

// preparaTela: l'immagine nascosta grande w×h (la rifà se la finestra ha cambiato grandezza).
func preparaTela(w, h int32) bool {
	if tela.mem != 0 && tela.w == w && tela.h == h {
		return true
	}
	liberaTela()
	if w <= 0 || h <= 0 {
		return false
	}
	scr, _, _ := pGetDC.Call(0)
	if scr == 0 {
		return false
	}
	mem, _, _ := pCreateCompatDC.Call(scr)
	var bmp uintptr
	if mem != 0 {
		bmp, _, _ = pCreateCompatBmp.Call(scr, uintptr(w), uintptr(h))
	}
	pReleaseDC.Call(0, scr)
	if mem == 0 || bmp == 0 {
		if mem != 0 {
			pDeleteDC.Call(mem)
		}
		return false
	}
	tela.vecchio, _, _ = pSelectObject.Call(mem, bmp)
	tela.mem, tela.bmp, tela.w, tela.h = mem, bmp, w, h
	tela.zone = nil
	pSetBkMode.Call(mem, 1)
	return true
}

func liberaTela() {
	if tela.mem != 0 {
		pSelectObject.Call(tela.mem, tela.vecchio)
		pDeleteDC.Call(tela.mem)
	}
	if tela.bmp != 0 {
		pDeleteObject.Call(tela.bmp)
	}
	tela.mem, tela.bmp, tela.vecchio, tela.w, tela.h, tela.zone = 0, 0, 0, 0, 0, nil
}

// svuotaTela: tutta trasparente.
func svuotaTela() {
	r := rect{0, 0, tela.w, tela.h}
	pFillRect.Call(tela.mem, uintptr(unsafe.Pointer(&r)), pag.chiave)
	tela.zone = map[chiaveDisegno]image.Rectangle{}
}

// disegnaTela: prepara nella tela le scritte bl. Ridisegna solo le zone delle scritte cambiate
// (nuove, tolte o con un disegno diverso); restituisce quelle zone, o tutto=true se è stata
// rifatta tutta. Nessuna zona e tutto=false: niente da cambiare sullo schermo.
func disegnaTela(bl []Blocco) (sporche []image.Rectangle, tutto bool) {
	finito := false
	defer func() {
		if !finito {
			tela.zone = nil // fermato a metà da un errore: la volta dopo la rifaccio tutta
		}
	}()
	// le toppe di catture che nessuna scritta usa più: via, se no tengono in vita le immagini vecchie
	vive := map[*image.RGBA]bool{}
	for _, b := range bl {
		vive[b.Img] = true
	}
	for k := range toppe {
		if !vive[k.img] {
			delete(toppe, k)
		}
	}
	nuove := make(map[chiaveDisegno]image.Rectangle, len(bl))
	var zoneBl []image.Rectangle // zona di ogni scritta di bl (vuota se non si disegna)
	for _, b := range bl {
		if b.Tradotto == "" {
			zoneBl = append(zoneBl, image.Rectangle{})
			continue
		}
		z := impaginaCache(tela.mem, b).zona
		nuove[chiaveDi(b)] = z
		zoneBl = append(zoneBl, z)
	}
	tutto = tela.zone == nil
	if !tutto {
		area := 0
		for k, z := range nuove {
			if _, ok := tela.zone[k]; !ok {
				sporche = append(sporche, z)
				area += z.Dx() * z.Dy()
			}
		}
		for k, z := range tela.zone {
			if _, ok := nuove[k]; !ok {
				sporche = append(sporche, z)
				area += z.Dx() * z.Dy()
			}
		}
		if len(sporche) == 0 {
			finito = true
			return nil, false
		}
		// più di metà finestra, o tantissimi pezzi: si fa prima a rifare tutto
		tutto = area*2 > int(tela.w)*int(tela.h) || len(sporche) > 200
	}
	if tutto {
		svuotaTela()
		for fase := 0; fase < 2; fase++ {
			for _, b := range bl {
				disegnaBlocco(tela.mem, b, fase)
			}
		}
		tela.zone, finito = nuove, true
		return nil, true
	}
	// solo le zone cambiate: tutto quello che disegno fuori da lì viene scartato da Windows
	rgn, _, _ := pCreateRgn.Call(0, 0, 0, 0)
	if rgn == 0 {
		svuotaTela()
		for fase := 0; fase < 2; fase++ {
			for _, b := range bl {
				disegnaBlocco(tela.mem, b, fase)
			}
		}
		tela.zone, finito = nuove, true
		return nil, true
	}
	for _, z := range sporche {
		if r, _, _ := pCreateRgn.Call(uintptr(int32(z.Min.X)), uintptr(int32(z.Min.Y)), uintptr(int32(z.Max.X)), uintptr(int32(z.Max.Y))); r != 0 {
			pCombineRgn.Call(rgn, rgn, r, 2) // RGN_OR
			pDeleteObject.Call(r)
		}
	}
	pSelectClip.Call(tela.mem, rgn) // Windows ne fa una copia
	pDeleteObject.Call(rgn)
	defer pSelectClip.Call(tela.mem, 0) // alla fine di nuovo tutta la tela (anche dopo un errore)
	for _, z := range sporche {
		r := rect{int32(z.Min.X), int32(z.Min.Y), int32(z.Max.X), int32(z.Max.Y)}
		pFillRect.Call(tela.mem, uintptr(unsafe.Pointer(&r)), pag.chiave)
	}
	tocca := func(z image.Rectangle) bool {
		for _, s := range sporche {
			if z.Overlaps(s) {
				return true
			}
		}
		return false
	}
	for fase := 0; fase < 2; fase++ {
		for i, b := range bl {
			if tocca(zoneBl[i]) {
				disegnaBlocco(tela.mem, b, fase)
			}
		}
	}
	tela.zone, finito = nuove, true
	return sporche, false
}

func pagProc(hwnd, msg, wp, lp uintptr) uintptr {
	switch msg {
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		dipingi(hwnd, hdc, ps.rcPaint)
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case 0x0014: // WM_ERASEBKGND: niente da cancellare, la tela copre tutto
		return 1
	case 0x0084: // WM_NCHITTEST: il mouse passa attraverso, il clic arriva al gioco
		return ^uintptr(0) // HTTRANSPARENT (-1)
	case 0x0021: // WM_MOUSEACTIVATE: non rubo il fuoco al gioco
		return 3 // MA_NOACTIVATE
	case wmPagina:
		aggiornaPagina(hwnd)
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, msg, wp, lp)
	return r
}

// dipingi (WM_PAINT): una sola copia dalla tela allo schermo, niente cancellature in mezzo.
func dipingi(hwnd, hdc uintptr, r rect) {
	defer guard("pagina-dipingi")
	muDisegno.Lock()
	defer muDisegno.Unlock()
	if tela.mem == 0 || r.Right > tela.w || r.Bottom > tela.h {
		var tutta rect // fuori dalla tela (non dovrebbe succedere): trasparente
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&tutta)))
		pFillRect.Call(hdc, uintptr(unsafe.Pointer(&tutta)), pag.chiave)
	}
	if tela.mem == 0 {
		return
	}
	x0, y0 := max(r.Left, 0), max(r.Top, 0)
	x1, y1 := min(r.Right, tela.w), min(r.Bottom, tela.h)
	if x1 > x0 && y1 > y0 {
		pBitBlt.Call(hdc, uintptr(x0), uintptr(y0), uintptr(x1-x0), uintptr(y1-y0), tela.mem, uintptr(x0), uintptr(y0), 0x00CC0020) // SRCCOPY
	}
}

// aggiornaPagina (wmPagina): prepara la tela con le scritte di adesso e mostra solo quello che
// è cambiato. Tutti i conti si fanno qui, non in WM_PAINT.
func aggiornaPagina(hwnd uintptr) {
	defer guard("pagina-aggiorna")
	pag.inCoda.Store(false) // da qui in poi una richiesta nuova va rimessa in coda
	pag.mu.Lock()
	a, bl, vis := pag.area, append([]Blocco(nil), pag.blocchi...), pag.vis && len(pag.blocchi) > 0
	pag.mu.Unlock()
	if !vis {
		// prima di nascondere la svuoto e la mostro vuota: quando ricompare non si vedono per
		// un attimo le scritte vecchie rimaste nella finestra
		svuota := func() bool {
			muDisegno.Lock()
			defer muDisegno.Unlock()
			s := tela.mem != 0 && tela.mostrata
			if s {
				svuotaTela()
			}
			tela.mostrata = false
			clear(toppe) // niente catture vecchie tenute in vita mentre non mostro niente
			return s
		}()
		if v, _, _ := pIsWindowVisible.Call(hwnd); v != 0 {
			if svuota {
				pInvalidateRect.Call(hwnd, 0, 0)
				pUpdateWindow.Call(hwnd)
			}
			pShowWindow.Call(hwnd, 0)
		}
		return
	}
	var sporche []image.Rectangle
	var tutto, appena bool
	pronta := func() bool {
		muDisegno.Lock()
		defer muDisegno.Unlock() // anche se il disegno si ferma per un errore
		if !preparaTela(int32(a.W), int32(a.H)) {
			return false
		}
		sporche, tutto = disegnaTela(bl)
		appena = !tela.mostrata // era nascosta: va copiata tutta
		tela.mostrata = true
		return true
	}()
	if !pronta {
		return
	}
	if v, _, _ := pIsWindowVisible.Call(hwnd); v == 0 {
		appena = true
	}
	// sempre in cima (un gioco "sempre in primo piano" non mi copre) e al posto della finestra del gioco
	pSetWindowPos.Call(hwnd, ^uintptr(0), uintptr(int32(a.X)), uintptr(int32(a.Y)), uintptr(a.W), uintptr(a.H), swpNoActivate)
	if appena {
		pShowWindow.Call(hwnd, 4) // SW_SHOWNOACTIVATE
	}
	switch {
	case tutto || appena:
		pInvalidateRect.Call(hwnd, 0, 0) // 0: senza cancellare prima
	case len(sporche) > 0:
		for _, z := range sporche {
			r := rect{int32(z.Min.X), int32(z.Min.Y), int32(z.Max.X), int32(z.Max.Y)}
			pInvalidateRect.Call(hwnd, uintptr(unsafe.Pointer(&r)), 0)
		}
	default:
		return // stesse scritte di prima: lo schermo resta com'è
	}
	pUpdateWindow.Call(hwnd) // subito, senza aspettare che la coda dei messaggi si svuoti
}

// areaFinestra: la parte interna (senza bordi) della finestra in primo piano, in pixel dello schermo.
func areaFinestra(h uintptr) (Zona, bool) {
	if h == 0 {
		return Zona{}, false
	}
	if ok, _, _ := pIsWindow.Call(h); ok == 0 {
		return Zona{}, false
	}
	var r rect
	pGetClientRect.Call(h, uintptr(unsafe.Pointer(&r)))
	pt := [2]int32{0, 0}
	pClientToScreen.Call(h, uintptr(unsafe.Pointer(&pt)))
	z := Zona{X: int(pt[0]), Y: int(pt[1]), W: int(r.Right - r.Left), H: int(r.Bottom - r.Top)}
	return z, z.W >= 200 && z.H >= 150
}

// salvaAnteprima: disegna le traduzioni sopra la cattura, con lo stesso codice della finestra,
// e la salva in un PNG (logs\pagina-anteprima.png). La finestra vera è invisibile alle catture,
// quindi questo è l'unico modo per controllare il risultato da una foto dello schermo.
func salvaAnteprima(img *image.RGBA, blocchi []Blocco, percorso string) error {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if w <= 0 || h <= 0 {
		return errCattura
	}
	scr, _, _ := pGetDC.Call(0)
	if scr == 0 {
		return errCattura
	}
	defer pReleaseDC.Call(0, scr)
	mem, _, _ := pCreateCompatDC.Call(scr)
	if mem == 0 {
		return errCattura
	}
	defer pDeleteDC.Call(mem)
	bmp, _, _ := pCreateCompatBmp.Call(scr, uintptr(w), uintptr(h))
	if bmp == 0 {
		return errCattura
	}
	defer pDeleteObject.Call(bmp)
	old, _, _ := pSelectObject.Call(mem, bmp)
	defer pSelectObject.Call(mem, old) // (i defer vanno al contrario: prima questo, poi DeleteObject)
	bi := bitmapInfoHeader{biWidth: int32(w), biHeight: int32(-h), biPlanes: 1, biBitCount: 32}
	bi.biSize = uint32(unsafe.Sizeof(bi))
	buf := make([]byte, w*h*4)
	for i := 0; i < len(buf); i += 4 { // RGBA -> BGRA
		buf[i], buf[i+1], buf[i+2], buf[i+3] = img.Pix[i+2], img.Pix[i+1], img.Pix[i], 255
	}
	pSetDIBits.Call(mem, bmp, 0, uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bi)), 0)
	pSetBkMode.Call(mem, 1)
	func() {
		muDisegno.Lock()
		defer muDisegno.Unlock()
		inAnteprima = true
		defer func() { inAnteprima = false }()
		for fase := 0; fase < 2; fase++ {
			for _, b := range blocchi {
				disegnaBlocco(mem, b, fase)
			}
		}
	}()
	pGetDIBits.Call(mem, bmp, 0, uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bi)), 0)
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(buf); i += 4 {
		out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = buf[i+2], buf[i+1], buf[i], 255
	}
	// prima in un file a parte, poi al suo posto: chi lo legge non trova mai un'immagine a metà
	tmp := percorso + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := png.Encode(f, out); err != nil {
		f.Close()
		return err
	}
	f.Close()
	return os.Rename(tmp, percorso)
}

// disegnaToppaPulita: copia dalla pagina "pulita" (tutte le scritte inglesi cancellate). Dove la
// pagina pulita è uguale al gioco e non c'è la scritta nuova, la finestra resta trasparente: si
// vede il gioco vero, che si muove. Coperto resta solo dove c'erano le lettere inglesi e dove va
// la traduzione.
func disegnaToppaPulita(hdc uintptr, b Blocco, r, testo image.Rectangle) {
	r = r.Intersect(b.Pulita.Bounds())
	if b.Img != nil {
		r = r.Intersect(b.Img.Bounds())
	}
	w, h := int32(r.Dx()), int32(r.Dy())
	if w <= 0 || h <= 0 {
		return
	}
	bgra := make([]byte, int(w*h)*4)
	i := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := b.Pulita.RGBAAt(x, y)
			if b.Img != nil && c == b.Img.RGBAAt(x, y) && !(image.Point{x, y}.In(testo)) && !inAnteprima {
				bgra[i], bgra[i+1], bgra[i+2], bgra[i+3] = byte(chiaveTrsp>>16&0xFF), byte(chiaveTrsp>>8&0xFF), byte(chiaveTrsp&0xFF), 255
			} else {
				bgra[i], bgra[i+1], bgra[i+2], bgra[i+3] = c.B, c.G, c.R, 255
				if uintptr(c.R)|uintptr(c.G)<<8|uintptr(c.B)<<16 == chiaveTrsp {
					bgra[i+2]--
				}
			}
			i += 4
		}
	}
	bi := bitmapInfoHeader{biWidth: w, biHeight: -h, biPlanes: 1, biBitCount: 32}
	bi.biSize = uint32(unsafe.Sizeof(bi))
	pSetDIBitsToDevice.Call(hdc, uintptr(r.Min.X), uintptr(r.Min.Y), uintptr(w), uintptr(h), 0, 0, 0, uintptr(h),
		uintptr(unsafe.Pointer(&bgra[0])), uintptr(unsafe.Pointer(&bi)), 0)
}

// disegnaTestoComposto: la scritta disegnata da me pixel per pixel sopra la pagina pulita. Dove
// non ci sono lettere (né bordino, né ombra) la finestra resta trasparente e si vede il gioco
// vero: niente rettangoli fermi sopra un gioco che si muove, niente frange attorno alle lettere.
func disegnaTestoComposto(hdc, font uintptr, b Blocco, t []uint16, rt rect, fl uintptr, sp int) {
	if b.Img == nil {
		return
	}
	marg := int32(sp + 3)
	r := image.Rect(int(rt.Left-marg), int(rt.Top-marg), int(rt.Right+marg), int(rt.Bottom+marg)).Intersect(b.Pulita.Bounds()).Intersect(b.Img.Bounds())
	w, h := int32(r.Dx()), int32(r.Dy())
	if w <= 0 || h <= 0 {
		return
	}
	mem, _, _ := pCreateCompatDC.Call(hdc)
	if mem == 0 {
		return
	}
	defer pDeleteDC.Call(mem)
	bmp, _, _ := pCreateCompatBmp.Call(hdc, uintptr(w), uintptr(h))
	if bmp == 0 {
		return
	}
	defer pDeleteObject.Call(bmp)
	vecchio, _, _ := pSelectObject.Call(mem, bmp)
	defer pSelectObject.Call(mem, vecchio)
	nero, _, _ := pCreateBrush.Call(0)
	tutto := rect{0, 0, w, h}
	pFillRect.Call(mem, uintptr(unsafe.Pointer(&tutto)), nero)
	pDeleteObject.Call(nero)
	pSetBkMode.Call(mem, 1)
	pSetTextColor.Call(mem, 0xFFFFFF)
	vecchioF, _, _ := pSelectObject.Call(mem, font)
	r2 := rect{rt.Left - int32(r.Min.X), rt.Top - int32(r.Min.Y), rt.Right - int32(r.Min.X), rt.Bottom - int32(r.Min.Y)}
	pDrawText.Call(mem, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), uintptr(unsafe.Pointer(&r2)), fl)
	pSelectObject.Call(mem, vecchioF)
	bi := bitmapInfoHeader{biWidth: w, biHeight: -h, biPlanes: 1, biBitCount: 32}
	bi.biSize = uint32(unsafe.Sizeof(bi))
	buf := make([]byte, int(w*h)*4)
	pGetDIBits.Call(mem, bmp, 0, uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bi)), 0)
	alfa := make([]uint8, int(w*h))
	for i := range alfa {
		alfa[i] = max(buf[i*4], buf[i*4+1], buf[i*4+2])
	}
	out := componiTesto(b, r, alfa, sp, inAnteprima)
	// quello che c'è già nella tela: dove la riga nuova non ha lettere resta così (prima la
	// toppa di una riga copriva il fondo delle lettere della riga sopra, foto 23/11)
	prima := make([]byte, len(buf))
	if ok, _, _ := pBitBlt.Call(mem, 0, 0, uintptr(w), uintptr(h), hdc, uintptr(r.Min.X), uintptr(r.Min.Y), 0x00CC0020); ok != 0 { // SRCCOPY
		pGetDIBits.Call(mem, bmp, 0, uintptr(h), uintptr(unsafe.Pointer(&prima[0])), uintptr(unsafe.Pointer(&bi)), 0)
	} else {
		prima = nil
	}
	for i := 0; i < len(out); i += 4 {
		if out[i+3] == 0 && prima != nil {
			buf[i], buf[i+1], buf[i+2], buf[i+3] = prima[i], prima[i+1], prima[i+2], 255
			continue
		}
		buf[i], buf[i+1], buf[i+2], buf[i+3] = out[i+2], out[i+1], out[i], 255 // RGBA -> BGRA
	}
	pSetDIBitsToDevice.Call(hdc, uintptr(r.Min.X), uintptr(r.Min.Y), uintptr(w), uintptr(h), 0, 0, 0, uintptr(h),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bi)), 0)
}
