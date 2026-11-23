//go:build windows

package traduce

// Finestra della modalità "tutta la schermata" (F6): grande come la finestra del gioco,
// trasparente, non cliccabile, esclusa dalle catture. Sopra ogni scritta inglese disegna un
// rettangolo con il colore dello sfondo del gioco e la traduzione italiana con il colore del testo.

import (
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"strings"
	"sync"
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
	font    map[int32]uintptr
	chiave  uintptr
}

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
	pag.hwnd, pag.chiave, pag.font = h, k, map[int32]uintptr{}
	pag.mu.Unlock()
}

// MostraPagina disegna le traduzioni (coordinate dei blocchi relative all'area).
// blocchiPagina: copia delle scritte mostrate adesso.
func blocchiPagina() []Blocco {
	pag.mu.Lock()
	defer pag.mu.Unlock()
	return append([]Blocco(nil), pag.blocchi...)
}

func MostraPagina(area Zona, b []Blocco) {
	pag.mu.Lock()
	pag.area, pag.blocchi, pag.vis = area, b, true
	h := pag.hwnd
	pag.mu.Unlock()
	if h != 0 {
		pPostMessage.Call(h, wmPagina, 0, 0)
	}
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
	if cambiate && h != 0 {
		pPostMessage.Call(h, wmPagina, 0, 0)
	}
	return cambiate
}

// NascondiPagina toglie le scritte e le dimentica: un ricolora dopo non le rimette sullo schermo.
func NascondiPagina() {
	pag.mu.Lock()
	pag.vis, pag.blocchi = false, nil
	h := pag.hwnd
	pag.mu.Unlock()
	if h != 0 {
		pPostMessage.Call(h, wmPagina, 0, 0)
	}
}

// famiglieFont: 0 normale; 1 e 2 strette, per i giochi che scrivono con lettere strette
// (Liar's Bar): la traduzione ha la stessa forma e ci sta nello stesso spazio.
var famiglieFont = []string{"Segoe UI", "Bahnschrift SemiCondensed", "Bahnschrift Condensed"}

func fontPagina(px int32, peso uintptr) uintptr { return fontPaginaF(px, peso, 0) }

func fontPaginaF(px int32, peso uintptr, fam int) uintptr {
	chiave := (px*10+int32(peso/100))*4 + int32(fam)
	if f, ok := pag.font[chiave]; ok {
		return f
	}
	face, _ := windows.UTF16PtrFromString(famiglieFont[fam])
	f, _, _ := pCreateFont.Call(uintptr(-px), 0, 0, 0, peso, 0, 0, 0, 1, 0, 0, 4, 0, uintptr(unsafe.Pointer(face))) // ANTIALIASED (grigi): niente frange colorate
	pag.font[chiave] = f
	return f
}

func colore(c [3]uint8) uintptr { return uintptr(c[0]) | uintptr(c[1])<<8 | uintptr(c[2])<<16 }

// misuraRiga: larghezza e altezza del testo (a capo dentro la larghezza larg se aCapo).
func misuraTesto(hdc, font uintptr, s string, larg int32, aCapo bool) (int32, int32) {
	pSelectObject.Call(hdc, font)
	r := rect{0, 0, larg, 0}
	t, _ := windows.UTF16FromString(s)
	fl := uintptr(dtCalcRect | dtNoPrefix)
	if aCapo {
		fl |= dtWordBreak
	} else {
		fl |= 0x20 // DT_SINGLELINE
	}
	pDrawText.Call(hdc, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), uintptr(unsafe.Pointer(&r)), fl)
	return r.Right - r.Left, r.Bottom - r.Top
}

// fase 0: sfondo, fase 1: scritta. Prima tutti gli sfondi, poi tutte le scritte, così uno
// sfondo non taglia le lettere della riga sopra.
func disegnaBlocco(hdc uintptr, b Blocco, fase int) {
	if b.Tradotto == "" {
		return
	}
	righe := int32(max(1, b.Righe))
	altRiga := int32(b.H) / righe
	px := int32(b.Em + 0.5)
	if px <= 0 || px > altRiga*16/10 {
		px = altRiga * 115 / 100
	}
	if px < 10 {
		px = 10
	}
	larg, maxLarg := int32(b.W), int32(max(b.LargMax, b.W))
	if b.Centro && righe == 1 {
		maxLarg = larg + 2*(maxLarg-larg) // centrata: cresce dai due lati
		if b.Img != nil {                 // ma non oltre i bordi dello schermo
			cx := int32(b.X + b.W/2)
			maxLarg = min(maxLarg, 2*min(cx, int32(b.Img.Bounds().Dx())-cx)-16)
		}
	}
	minPx := max(10, px*78/100)
	if righe == 1 {
		// una riga sola (voce di menu, etichetta): meglio un po' più piccola che andare a capo
		// sopra la riga sotto (visto nelle foto del 22/11: colonne accavallate)
		minPx = max(9, px*62/100)
	}
	var w, h int32
	var font uintptr
	// lettere del gioco strette? Misuro l'inglese con il carattere normale: se viene molto più
	// largo dell'originale, uso un carattere stretto (stessa forma del gioco)
	fam := 0
	if b.Testo != "" && b.W > 0 {
		wIng, _ := misuraTesto(hdc, fontPaginaF(px, b.pesoCarattere(), 0), b.Testo, 0, false)
		spazio := float64(b.W)
		if righe > 1 {
			spazio = float64(b.W) * float64(righe) * 0.9
		}
		switch r := float64(wIng) / spazio; {
		case r > 1.24:
			fam = 2
		case r > 1.1:
			fam = 1
		}
	}
	// stessa grandezza del gioco; se l'italiano è più lungo uso lo spazio libero a destra,
	// poi rimpicciolisco un po' (fino al 78%), e solo alla fine vado a capo
	for prova := px; ; prova-- {
		font = fontPaginaF(prova, b.pesoCarattere(), fam)
		if righe == 1 {
			w, h = misuraTesto(hdc, font, b.Tradotto, 0, false)
			if w <= maxLarg || prova <= minPx {
				if w > maxLarg {
					w, h = misuraTesto(hdc, font, b.Tradotto, maxLarg, true)
				}
				break
			}
		} else {
			w, h = misuraTesto(hdc, font, b.Tradotto, larg, true)
			if h <= int32(b.H)*115/100 || prova <= minPx {
				break
			}
		}
	}
	// dove va la traduzione: allineata a sinistra come l'originale, o centrata sullo stesso centro
	tx := int32(b.X)
	if b.Centro && righe == 1 {
		tx = int32(b.X+b.W/2) - w/2
	}
	if b.Img != nil { // mai fuori dallo schermo
		tx = max(4, min(tx, int32(b.Img.Bounds().Dx())-w-4))
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
	if righe == 1 && !(w > maxLarg) {
		// stessa linea di base dell'originale (la cella del carattere ha spazio sopra le lettere)
		var tm [60]byte // TEXTMETRICW: tmHeight, tmAscent…
		if r, _, _ := pGetTextMetrics.Call(hdc, uintptr(unsafe.Pointer(&tm[0]))); r != 0 {
			asc := int32(binary.LittleEndian.Uint32(tm[4:]))
			ty = int32(lineaDiBase(b)) - asc
		}
	}
	if fase == 0 {
		if b.Pulita != nil {
			// la toppa copre anche tutta la scritta nuova (gambe sotto comprese): niente bordi
			// colorati attorno alle lettere
			testo := image.Rect(int(tx)-2, int(ty)-2, int(tx+w)+2, int(ty+h)+2)
			if b.Centro && righe > 1 {
				testo = image.Rect(int(x0), int(ty)-2, int(x1), int(ty+h)+2)
			}
			disegnaToppaPulita(hdc, b, image.Rect(int(x0), int(y0), int(x1), int(y1)).Union(testo), image.Rectangle{})
			return
		}
		disegnaSfondo(hdc, b, x0, y0, x1, y1)
		return
	}
	t, _ := windows.UTF16FromString(b.Tradotto)
	fl := uintptr(dtWordBreak | dtNoPrefix | dtLeft)
	rt := rect{tx, ty, x1 - m, max(y1, ty+h+2)}
	if b.Centro && righe > 1 {
		rt, fl = rect{x0 + m, ty, x1 - m, y1}, dtWordBreak|dtNoPrefix|0x1 // DT_CENTER
	}
	if b.Pulita != nil {
		sp := 1
		if px >= 26 {
			sp = 2
		}
		disegnaTestoComposto(hdc, font, b, t, rt, fl, sp)
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
		if px >= 26 {
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
		r := rect{x0, y0, x1, y1}
		pFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), pen)
		pDeleteObject.Call(pen)
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

// toppe già calcolate (la finestra si ridisegna spesso): si usano solo dal thread che disegna
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
// trasparenza, la cattura sotto è già il gioco. Solo con pag.mu preso.
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

func pagProc(hwnd, msg, wp, lp uintptr) uintptr {
	switch msg {
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		var r rect
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
		pag.mu.Lock()
		pFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), pag.chiave)
		pSetBkMode.Call(hdc, 1)
		for fase := 0; fase < 2; fase++ {
			for _, b := range pag.blocchi {
				disegnaBlocco(hdc, b, fase)
			}
		}
		pag.mu.Unlock()
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case 0x0084: // WM_NCHITTEST: il mouse passa attraverso, il clic arriva al gioco
		return ^uintptr(0) // HTTRANSPARENT (-1)
	case 0x0021: // WM_MOUSEACTIVATE: non rubo il fuoco al gioco
		return 3 // MA_NOACTIVATE
	case wmPagina:
		pag.mu.Lock()
		a, vis := pag.area, pag.vis && len(pag.blocchi) > 0
		pag.mu.Unlock()
		if !vis {
			pShowWindow.Call(hwnd, 0)
			return 0
		}
		pSetWindowPos.Call(hwnd, ^uintptr(0), uintptr(int32(a.X)), uintptr(int32(a.Y)), uintptr(a.W), uintptr(a.H), swpNoActivate)
		pShowWindow.Call(hwnd, 4)
		pInvalidateRect.Call(hwnd, 0, 1)
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, msg, wp, lp)
	return r
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
	scr, _, _ := pGetDC.Call(0)
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
	defer pSelectObject.Call(mem, old)
	bi := bitmapInfoHeader{biWidth: int32(w), biHeight: int32(-h), biPlanes: 1, biBitCount: 32}
	bi.biSize = uint32(unsafe.Sizeof(bi))
	buf := make([]byte, w*h*4)
	for i := 0; i < len(buf); i += 4 { // RGBA -> BGRA
		buf[i], buf[i+1], buf[i+2], buf[i+3] = img.Pix[i+2], img.Pix[i+1], img.Pix[i], 255
	}
	pSetDIBits.Call(mem, bmp, 0, uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bi)), 0)
	pSetBkMode.Call(mem, 1)
	pag.mu.Lock()
	inAnteprima = true
	for fase := 0; fase < 2; fase++ {
		for _, b := range blocchi {
			disegnaBlocco(mem, b, fase)
		}
	}
	inAnteprima = false
	pag.mu.Unlock()
	pGetDIBits.Call(mem, bmp, 0, uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bi)), 0)
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(buf); i += 4 {
		out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = buf[i+2], buf[i+1], buf[i], 255
	}
	f, err := os.Create(percorso)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, out)
}

// disegnaToppaPulita: copia dalla pagina "pulita" (tutte le scritte inglesi cancellate). Dove la
// pagina pulita è uguale al gioco e non c'è la scritta nuova, la finestra resta trasparente: si
// vede il gioco vero, che si muove. Coperto resta solo dove c'erano le lettere inglesi e dove va
// la traduzione.
func disegnaToppaPulita(hdc uintptr, b Blocco, r, testo image.Rectangle) {
	r = r.Intersect(b.Pulita.Bounds())
	w, h := int32(r.Dx()), int32(r.Dy())
	if w <= 0 || h <= 0 {
		return
	}
	bgra := make([]byte, int(w*h)*4)
	i := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := b.Pulita.RGBAAt(x, y)
			if c == b.Img.RGBAAt(x, y) && !(image.Point{x, y}.In(testo)) && !inAnteprima {
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
	marg := int32(sp + 3)
	r := image.Rect(int(rt.Left-marg), int(rt.Top-marg), int(rt.Right+marg), int(rt.Bottom+marg)).Intersect(b.Pulita.Bounds())
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
	pSelectObject.Call(mem, font)
	r2 := rect{rt.Left - int32(r.Min.X), rt.Top - int32(r.Min.Y), rt.Right - int32(r.Min.X), rt.Bottom - int32(r.Min.Y)}
	pDrawText.Call(mem, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), uintptr(unsafe.Pointer(&r2)), fl)
	bi := bitmapInfoHeader{biWidth: w, biHeight: -h, biPlanes: 1, biBitCount: 32}
	bi.biSize = uint32(unsafe.Sizeof(bi))
	buf := make([]byte, int(w*h)*4)
	pGetDIBits.Call(mem, bmp, 0, uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bi)), 0)
	alfa := make([]uint8, int(w*h))
	for i := range alfa {
		alfa[i] = max(buf[i*4], buf[i*4+1], buf[i*4+2])
	}
	out := componiTesto(b, r, alfa, sp, inAnteprima)
	for i := 0; i < len(out); i += 4 {
		buf[i], buf[i+1], buf[i+2], buf[i+3] = out[i+2], out[i+1], out[i], 255 // RGBA -> BGRA
	}
	pSetDIBitsToDevice.Call(hdc, uintptr(r.Min.X), uintptr(r.Min.Y), uintptr(w), uintptr(h), 0, 0, 0, uintptr(h),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bi)), 0)
}
