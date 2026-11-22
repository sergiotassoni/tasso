//go:build windows && collaudo

package traduce

// Foto del programma vero: TassoTraduce-collaudo.exe --foto "C:\cartella" --scene "C:\scene"
// Apre Tasso Traduce, gira le sue schermate e fotografa la finestra; poi mostra le immagini di
// gioco della cartella scene in una finestra, accende la traduzione vera (OCR di Windows +
// Mozilla) e le voci (whisper) e fotografa lo schermo con le traduzioni sopra.
// Solo in questa modalità le finestre di Tasso si lasciano fotografare (di solito sono escluse
// dalle catture per non rileggere le proprie scritte).

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	cartellaFoto  string
	cartellaScene string
)

func init() {
	for i, a := range os.Args {
		if a == "--foto" && i+1 < len(os.Args) {
			cartellaFoto = os.Args[i+1]
			fotoVisibili = true
		}
		if a == "--scene" && i+1 < len(os.Args) {
			cartellaScene = os.Args[i+1]
		}
	}
}

func js(codice string) {
	wv.Dispatch(func() { wv.Eval(codice) })
}

func rettangolo(h uintptr) Zona {
	var r rect
	pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&r)))
	return Zona{X: int(r.Left), Y: int(r.Top), W: int(r.Right - r.Left), H: int(r.Bottom - r.Top)}
}

func fotografa(z Zona, nome string) error {
	img, err := cattura(z)
	if err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(cartellaFoto, nome))
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

var (
	pAttachThreadInput  = user32.NewProc("AttachThreadInput")
	pGetCurrentThreadId = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetCurrentThreadId")
	pBringWindowToTop   = user32.NewProc("BringWindowToTop")
)

// avanti: porta la finestra davanti a tutto (sopra le altre finestre aperte sul PC) e in primo piano.
func avanti(h uintptr) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pShowWindow.Call(h, 9)
	pSetWindowPos.Call(h, ^uintptr(0), 0, 0, 0, 0, 0x0001|0x0002|0x0040) // HWND_TOPMOST, senza spostarla
	fg, _, _ := pGetForeground.Call()
	io, _, _ := pGetCurrentThreadId.Call()
	suo, _, _ := pGetWindowThreadProcessId.Call(fg, 0)
	if suo != 0 && suo != io {
		pAttachThreadInput.Call(io, suo, 1)
		defer pAttachThreadInput.Call(io, suo, 0)
	}
	pBringWindowToTop.Call(h)
	pSetForeground.Call(h)
	time.Sleep(900 * time.Millisecond)
}

func eseguiFoto() {
	defer guard("foto")
	defer func() { wv.Dispatch(func() { wv.Terminate() }) }()
	_ = os.MkdirAll(cartellaFoto, 0o755)
	var note []string
	scatta := func(z Zona, nome, descr string) {
		if err := fotografa(z, nome); err != nil {
			note = append(note, nome+": errore "+err.Error())
			return
		}
		note = append(note, nome+": "+descr)
	}
	h := uintptr(wv.Window())
	pSetWindowPos.Call(h, 0, 40, 30, 1440, 900, 0x0040)
	avanti(h)
	time.Sleep(1500 * time.Millisecond)
	scatta(rettangolo(h), "01-avvio-spento.png", "la finestra appena aperto: sempre spento all'avvio")

	js(`document.getElementById('h-acceso').click()`)
	time.Sleep(1500 * time.Millisecond)
	scatta(rettangolo(h), "02-acceso.png", "acceso: pronto a tradurre i giochi")

	js(`document.getElementById('m-voci').classList.add('on')`)
	time.Sleep(900 * time.Millisecond)
	scatta(rettangolo(h), "03-voci.png", "strumento Voci in italiano: interruttore, lingue, scorciatoia, Salva e applica")
	js(`document.getElementById('m-voci').classList.remove('on'); document.getElementById('m-chat').classList.add('on'); document.getElementById('h-chat-it').value="Aspettatemi all'ingresso del castello, arrivo tra due minuti."; document.getElementById('h-chat-vai').click()`)
	time.Sleep(2500 * time.Millisecond)
	scatta(rettangolo(h), "04-chat-tradotta.png", "strumento Chat: frase italiana tradotta in inglese e copiata")
	js(`document.getElementById('m-chat').classList.remove('on'); document.getElementById('m-trad').classList.add('on')`)
	time.Sleep(900 * time.Millisecond)
	scatta(rettangolo(h), "04b-traduzione.png", "strumento Traduzione dello schermo")
	js(`document.getElementById('m-trad').classList.remove('on'); document.querySelector('nav button[data-v=giochi]').click()`)
	time.Sleep(2500 * time.Millisecond)
	scatta(rettangolo(h), "04c-giochi.png", "i miei giochi con le copertine")
	// Traduzione gioco a passi (prova vera: alla fine la fermo, così le impostazioni restano com'erano)
	js(`document.getElementById('apri-wiz').click()`)
	time.Sleep(2500 * time.Millisecond)
	js(`(document.querySelector('#w-lista .gioco')||{click(){}}).click()`)
	time.Sleep(600 * time.Millisecond)
	scatta(rettangolo(h), "04d-passo1-gioco.png", "Traduzione gioco, passo 1: scegli il gioco")
	js(`document.getElementById('w-avanti').click()`)
	time.Sleep(500 * time.Millisecond)
	scatta(rettangolo(h), "04e-passo2-lingua.png", "passo 2: lingua del gioco")
	js(`document.getElementById('w-avanti').click()`)
	time.Sleep(500 * time.Millisecond)
	scatta(rettangolo(h), "04f-passo3-italiano.png", "passo 3: italiano, Applica traduzione")
	js(`document.getElementById('w-avanti').click()`)
	time.Sleep(150 * time.Millisecond)
	scatta(rettangolo(h), "04g-applico.png", "applico: barra di avanzamento con i passi veri")
	time.Sleep(4 * time.Second)
	scatta(rettangolo(h), "04h-applicata.png", "traduzione applicata")
	js(`document.getElementById('w-ferma').click()`)
	time.Sleep(800 * time.Millisecond)
	// gioco con anti-imbroglio: l'avviso e la conferma
	js(`document.getElementById('apri-wiz').click()`)
	time.Sleep(2500 * time.Millisecond)
	js(`(()=>{const g=[...document.querySelectorAll('#w-lista .gioco')].find(x=>x.textContent.includes('anti-imbroglio')); if(g){g.click(); document.getElementById('w-avanti').click(); document.getElementById('w-avanti').click();}})()`)
	time.Sleep(700 * time.Millisecond)
	scatta(rettangolo(h), "04i-anticheat-avviso.png", "gioco con anti-imbroglio: tradotto sullo schermo, piccolo messaggio")
	js(`document.getElementById('m-wiz').classList.remove('on')`)
	time.Sleep(400 * time.Millisecond)
	js(`document.querySelector('nav button[data-v=opz]').click(); document.getElementById('o-analizza').click(); setTimeout(()=>{ window.scrollTo(0,0); document.scrollingElement && (document.scrollingElement.scrollTop=0); document.querySelectorAll('main,section,.vista').forEach(e=>e.scrollTop=0); }, 300)`)
	time.Sleep(6 * time.Second)
	scatta(rettangolo(h), "05-impostazioni.png", "impostazioni: quale traduttore conviene a questo PC e motore")

	js(`document.querySelectorAll('#v-opz details').forEach(d=>d.open=true); document.getElementById('o-tasti').scrollIntoView({block:'center'})`)
	time.Sleep(800 * time.Millisecond)
	scatta(rettangolo(h), "06-tasti.png", "tasti a scelta (anche chat Ctrl+Alt+T e voci Ctrl+Alt+V)")

	// ---- scene di gioco: la finestra di Tasso torna una finestra normale, dietro al gioco
	pSetWindowPos.Call(h, ^uintptr(1), 0, 0, 0, 0, 0x0001|0x0002|0x0010) // HWND_NOTOPMOST
	scene, _ := filepath.Glob(filepath.Join(cartellaScene, "*.png"))
	n := 7
	for _, s := range scene {
		if n > 9 {
			break
		}
		img, err := caricaPNG(s)
		if err != nil {
			continue
		}
		fin := mostraImmagine(img)
		if fin == 0 {
			continue
		}
		avanti(fin)
		zona := rettangolo(fin)
		nome := strings.TrimSuffix(filepath.Base(s), ".png")
		scatta(zona, fmt.Sprintf("%02d-%s-inglese.png", n, nome), "schermata del gioco com'è (inglese)")
		avviaPaginaSu(fin)
		time.Sleep(8 * time.Second)
		scatta(zona, fmt.Sprintf("%02d-%s-tradotto.png", n, nome), "la stessa schermata con Tasso acceso (F6): scritte sostituite al loro posto")
		paginaAttiva.Store(false)
		paginaGiro.Add(1)
		NascondiPagina()
		n++
		if n == 9 && cartellaAudioProva != "" && vociPresenti() {
			// voci: sottotitolo sopra il gioco mentre parla un personaggio
			if err := accendiVoci(true); err == nil {
				avanti(fin)
				go suona(filepath.Join(cartellaAudioProva, "voce1.wav"))
				fatto := false
				for i := 0; i < 120 && !fatto; i++ {
					time.Sleep(100 * time.Millisecond)
					ov.mu.Lock()
					ok := ov.voce && ov.visibile && !strings.HasSuffix(ov.testo, "…")
					ov.mu.Unlock()
					if ok {
						time.Sleep(300 * time.Millisecond)
						scatta(zona, fmt.Sprintf("%02d-voce-sottotitolo.png", n), "un personaggio parla: sottotitolo italiano in basso")
						fatto = true
					}
				}
				if !fatto {
					note = append(note, "sottotitolo della voce non comparso in tempo")
				}
				_ = accendiVoci(false)
				n++
			}
		}
		pShowWindow.Call(fin, 0)
	}
	note = append(note, misuraCambi(scene)...)
	_ = os.WriteFile(filepath.Join(cartellaFoto, "foto.txt"), []byte(strings.Join(note, "\r\n")), 0o644)
}

func caricaPNG(p string) (*image.RGBA, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	im, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	out := image.NewRGBA(im.Bounds())
	draw.Draw(out, out.Bounds(), im, im.Bounds().Min, draw.Src)
	return out, nil
}

var (
	pStretchDIBits = gdi32.NewProc("StretchDIBits")
	pBeginPaintF   = user32.NewProc("BeginPaint")
	pEndPaintF     = user32.NewProc("EndPaint")
	immagineFoto   *image.RGBA
	bgrFoto        []byte
)

// mostraImmagine: una finestra senza bordi che mostra la schermata del gioco (al posto del gioco vero).
func mostraImmagine(img *image.RGBA) uintptr {
	immagineFoto = img
	b := img.Bounds()
	bgrFoto = make([]byte, b.Dx()*b.Dy()*4)
	for i := 0; i < len(img.Pix); i += 4 {
		bgrFoto[i], bgrFoto[i+1], bgrFoto[i+2], bgrFoto[i+3] = img.Pix[i+2], img.Pix[i+1], img.Pix[i], 255
	}
	pronta := make(chan uintptr, 1)
	go func() {
		runtime.LockOSThread()
		classe, _ := windows.UTF16PtrFromString(fmt.Sprintf("TassoFotoGioco%d", time.Now().UnixNano()))
		titolo, _ := windows.UTF16PtrFromString("Gioco di prova")
		wc := wndClassEx{lpfnWndProc: windows.NewCallback(fotoProc), lpszClassName: classe}
		wc.cbSize = uint32(unsafe.Sizeof(wc))
		pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
		w, h := b.Dx(), b.Dy()
		if w > 1900 { // dimensione vera, come il gioco (rimpicciolire rovina le scritte piccole)
			h = h * 1900 / w
			w = 1900
		}
		fin, _, _ := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(classe)), uintptr(unsafe.Pointer(titolo)), 0x80000000|0x10000000, 10, 10, uintptr(w), uintptr(h), 0, 0, 0, 0)
		pronta <- fin
		var m msgW
		for {
			r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if r == 0 || int32(r) == -1 {
				return
			}
			pTranslateMsg.Call(uintptr(unsafe.Pointer(&m)))
			pDispatchMsg.Call(uintptr(unsafe.Pointer(&m)))
		}
	}()
	return <-pronta
}

func fotoProc(hwnd, msg, wp, lp uintptr) uintptr {
	if msg == wmPaint && immagineFoto != nil {
		var ps paintStruct
		hdc, _, _ := pBeginPaintF.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		var r rect
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
		b := immagineFoto.Bounds()
		type bih struct {
			Size                                uint32
			Width, Height                       int32
			Planes, BitCount                    uint16
			Compression, SizeImage              uint32
			XPels, YPels, ClrUsed, ClrImportant int32
		}
		hdr := bih{Size: 40, Width: int32(b.Dx()), Height: -int32(b.Dy()), Planes: 1, BitCount: 32}
		pStretchDIBits.Call(hdc, 0, 0, uintptr(r.Right), uintptr(r.Bottom), 0, 0, uintptr(b.Dx()), uintptr(b.Dy()),
			uintptr(unsafe.Pointer(&bgrFoto[0])), uintptr(unsafe.Pointer(&hdr)), 0, 0x00CC0020)
		pEndPaintF.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, msg, wp, lp)
	return r
}

var pInvalidateRectF = user32.NewProc("InvalidateRect")

// cambiaImmagine: la "schermata del gioco" cambia (come quando apri un altro menu).
func cambiaImmagine(fin uintptr, img *image.RGBA) {
	b := img.Bounds()
	bgr := make([]byte, b.Dx()*b.Dy()*4)
	for i := 0; i < len(img.Pix); i += 4 {
		bgr[i], bgr[i+1], bgr[i+2], bgr[i+3] = img.Pix[i+2], img.Pix[i+1], img.Pix[i], 255
	}
	immagineFoto, bgrFoto = img, bgr
	pInvalidateRectF.Call(fin, 0, 1)
}

// misuraCambi: quanto passa da quando la schermata del gioco cambia a quando la traduzione
// nuova è disegnata. Prima volta (scritte nuove) e volte dopo (scritte già viste).
func misuraCambi(scene []string) []string {
	if len(scene) < 2 {
		return nil
	}
	a, err1 := caricaPNG(scene[0])
	b, err2 := caricaPNG(scene[1])
	if err1 != nil || err2 != nil {
		return []string{"misura cambi: immagini non leggibili"}
	}
	// misura come per l'utente vero: le traduzioni disegnate non si vedono nelle catture
	// (altrimenti Tasso rivede le sue scritte e crede che la schermata sia cambiata)
	pag.mu.Lock()
	hp := pag.hwnd
	pag.mu.Unlock()
	pSetDisplayAffin.Call(hp, 0x11)
	// misura onesta: traduttore senza memoria e libreria vuota (niente scritte già viste prima)
	traduttore = NuovoTraduttore("", leggiImp().Glossario, moz)
	traduttore.QuandoCorretta(correzioneArrivata)
	fin := mostraImmagine(a)
	avanti(fin)
	avviaPaginaSu(fin)
	traduttore.ApriLibreria(filepath.Join(os.TempDir(), fmt.Sprintf("tt-misura-%d.json", time.Now().UnixNano())))
	time.Sleep(6 * time.Second)
	var out []string
	immagini := []*image.RGBA{b, a, b, a, b, a}
	for i, im := range immagini {
		prima := paginaMostrata.Load()
		t0 := time.Now()
		cambiaImmagine(fin, im)
		for j := 0; j < 300 && paginaMostrata.Load() == prima; j++ {
			time.Sleep(10 * time.Millisecond)
		}
		dt := time.Since(t0)
		quale := "scritte nuove"
		if i >= 2 {
			quale = "scritte già viste"
		}
		if tp := tempiPagina.Load(); tp != nil {
			out = append(out, fmt.Sprintf("cambio schermata %d (%s): traduzione disegnata dopo %d ms (visto dopo %d ms, OCR %d ms, traduzione %d ms, colori e sfondi %d ms; %d scritte, %d tradotte ora)", i+1, quale, dt.Milliseconds(), tp.Visto.Sub(t0).Milliseconds(), tp.OCR.Milliseconds(), tp.Traduzione.Milliseconds(), (tp.Totale-tp.OCR-tp.Traduzione).Milliseconds(), tp.Scritte, tp.Nuove))
		}
		time.Sleep(2 * time.Second)
	}
	paginaAttiva.Store(false)
	paginaGiro.Add(1)
	NascondiPagina()
	pShowWindow.Call(fin, 0)
	return out
}
