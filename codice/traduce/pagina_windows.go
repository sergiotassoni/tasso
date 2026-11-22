//go:build windows

package traduce

// Finestra della modalità "tutta la schermata" (F6): grande come la finestra del gioco,
// trasparente, non cliccabile, esclusa dalle catture. Sopra ogni scritta inglese disegna un
// rettangolo con il colore dello sfondo del gioco e la traduzione italiana con il colore del testo.

import (
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
	pSetLayeredAttr.Call(h, chiaveTrsp, 250, 1|2) // LWA_COLORKEY | LWA_ALPHA
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

func NascondiPagina() {
	pag.mu.Lock()
	pag.vis = false
	h := pag.hwnd
	pag.mu.Unlock()
	if h != 0 {
		pPostMessage.Call(h, wmPagina, 0, 0)
	}
}

func fontPagina(px int32, grassetto bool) uintptr {
	chiave, peso := px, uintptr(500)
	if grassetto { // come nel gioco: titoli e pulsanti in neretto restano in neretto
		chiave, peso = -px, 700
	}
	if f, ok := pag.font[chiave]; ok {
		return f
	}
	face, _ := windows.UTF16PtrFromString("Segoe UI")
	f, _, _ := pCreateFont.Call(uintptr(-px), 0, 0, 0, peso, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(face)))
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
	minPx := max(10, px*78/100)
	if righe == 1 {
		// una riga sola (voce di menu, etichetta): meglio un po' più piccola che andare a capo
		// sopra la riga sotto (visto nelle foto del 22/11: colonne accavallate)
		minPx = max(9, px*62/100)
	}
	var w, h int32
	var font uintptr
	// stessa grandezza del gioco; se l'italiano è più lungo uso lo spazio libero a destra,
	// poi rimpicciolisco un po' (fino al 78%), e solo alla fine vado a capo
	for prova := px; ; prova-- {
		font = fontPagina(prova, b.grassetto())
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
	x0, y0 := int32(b.X)-3, int32(b.Y)-3
	x1 := x0 + max(larg, w) + 6
	y1 := y0 + max(int32(b.H), h) + 6
	sf := [3]uint8{b.Sfondo.R, b.Sfondo.G, b.Sfondo.B}
	if colore(sf) == chiaveTrsp {
		sf[0]-- // non deve coincidere con il colore trasparente
	}
	if fase == 0 {
		pen, _, _ := pCreateBrush.Call(colore(sf))
		r := rect{x0, y0, x1, y1}
		pFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), pen)
		pDeleteObject.Call(pen)
		if b.Fondo != nil { // sopra il rettangolo pieno, lo sfondo ricostruito con le sfumature del gioco
			fw, fh := b.Fondo.Bounds().Dx(), b.Fondo.Bounds().Dy()
			bgra := make([]byte, fw*fh*4)
			for i := 0; i < len(bgra); i += 4 {
				bgra[i], bgra[i+1], bgra[i+2], bgra[i+3] = b.Fondo.Pix[i+2], b.Fondo.Pix[i+1], b.Fondo.Pix[i], 255
				if uintptr(bgra[i+2])|uintptr(bgra[i+1])<<8|uintptr(bgra[i])<<16 == chiaveTrsp {
					bgra[i+2]--
				}
			}
			bi := bitmapInfoHeader{biWidth: int32(fw), biHeight: int32(-fh), biPlanes: 1, biBitCount: 32}
			bi.biSize = uint32(unsafe.Sizeof(bi))
			pSetDIBitsToDevice.Call(hdc, uintptr(x0), uintptr(y0), uintptr(fw), uintptr(fh), 0, 0, 0, uintptr(fh),
				uintptr(unsafe.Pointer(&bgra[0])), uintptr(unsafe.Pointer(&bi)), 0)
		}
		return
	}
	pSelectObject.Call(hdc, font)
	pSetTextColor.Call(hdc, colore([3]uint8{b.Inchiostro.R, b.Inchiostro.G, b.Inchiostro.B}))
	t, _ := windows.UTF16FromString(b.Tradotto)
	rt := rect{int32(b.X), int32(b.Y) + (int32(b.H)-h)/2*boolI(h < int32(b.H)), x1 - 3, y1}
	pDrawText.Call(hdc, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), uintptr(unsafe.Pointer(&rt)), dtWordBreak|dtNoPrefix|dtLeft)
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
	for fase := 0; fase < 2; fase++ {
		for _, b := range blocchi {
			disegnaBlocco(mem, b, fase)
		}
	}
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
