//go:build windows

package traduce

// Chat al contrario (idea presa da LagoFast): scrivi in italiano nella chat del gioco, la tagli
// (Ctrl+A, Ctrl+X), premi Ctrl+Alt+T: Tasso traduce in inglese SUL PC e lo rimette negli
// appunti; Ctrl+V e invii. Non tocca il gioco e non manda niente fuori dal PC.
// Motore: Mozilla Bergamot italiano→inglese (MPL-2.0), scaricato solo dopo il sì dell'utente.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Impronte dal registro ufficiale di Bergamot (bergamot.s3.amazonaws.com/models/index.json, 22/11/2026).
// Modello italiano→inglese 2.0 di Mozilla (Firefox, MPL-2.0), impronte dal registro di Firefox.
var fileMozillaITEN = []FileDaScaricare{
	{URL: "https://firefox-settings-attachments.cdn.mozilla.net/main-workspace/translations-models/a6e3de84-6612-4546-a4ef-b658ee23ca08.bin", Nome: "model.iten.intgemm.alphas.bin",
		SHA256: "21b70978ce2f3b4da7a06b5de86a09abe3acd30b9eee1b2ebb3582b9bad790bf", Byte: 31561787},
	{URL: "https://firefox-settings-attachments.cdn.mozilla.net/main-workspace/translations-models/122ac310-0f7b-40d5-b49d-bcee81077c85.bin", Nome: "lex.50.50.iten.s2t.bin",
		SHA256: "0700d6d70b30490e68ff9deca0b45b80310745b491855bde3a13e1692d0cbce1", Byte: 4713616},
	{URL: "https://firefox-settings-attachments.cdn.mozilla.net/main-workspace/translations-models/51b5a437-9daa-481e-a9bc-311f2b3f490c.spm", Nome: "vocab.iten.spm",
		SHA256: "3ef0211d4ae6db21440892f180f2019fe2bfc110a330ffa9d2eca9665e4f2bc5", Byte: 812724},
}

func dirMozillaITEN() string { return filepath.Join(dirApp, "modelli", "mozilla-iten-2") }

func chatPresente() bool {
	for _, f := range fileMozillaITEN {
		if _, err := os.Stat(filepath.Join(dirMozillaITEN(), f.Nome)); err != nil {
			return false
		}
	}
	return true
}

var scaricandoChat atomic.Bool

func scaricaChat() {
	defer guard("scarica-chat")
	if !scaricandoChat.CompareAndSwap(false, true) {
		return
	}
	defer scaricandoChat.Store(false)
	err := scaricaTutti(context.Background(), dirMozillaITEN(), fileMozillaITEN, func(f, t int64) {
		emetti("avanzamento", map[string]interface{}{"cosa": "chat", "fatti": f, "totale": t})
	})
	if err != nil {
		emetti("erroreScarica", map[string]string{"cosa": "chat", "errore": err.Error()})
		return
	}
	emetti("scaricato", map[string]string{"cosa": "chat"})
	emetti("aggiorna", stato())
}

// traduciChat: italiano → inglese con Mozilla dentro WebView2.
func traduciChat(ctx context.Context, testo string) (string, error) {
	testo = strings.TrimSpace(testo)
	if testo == "" {
		return "", errors.New("non c'è niente da tradurre")
	}
	if len([]rune(testo)) > 2000 {
		return "", errors.New("testo troppo lungo per la chat")
	}
	if !chatPresente() {
		return "", errors.New("manca il traduttore italiano→inglese: scaricalo nella finestra di Tasso Traduce (37 MB)")
	}
	return moz.traduciCoppia(ctx, testo, "it", "en")
}

// suTastoChat: appunti italiani → appunti inglesi.
func suTastoChat() {
	defer guard("chat")
	it, err := leggiAppunti()
	if err != nil || strings.TrimSpace(it) == "" {
		avvisoBreve("Chat: scrivi in italiano nella chat del gioco, poi Ctrl+A e Ctrl+X, poi " + etichettaTasto(tastiCompleti(leggiImp().Tasti)["chat"]) + ".")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	en, err := traduciChat(ctx, it)
	if err != nil {
		avvisoBreve("Chat: " + err.Error())
		return
	}
	if err := scriviAppunti(en); err != nil {
		avvisoBreve("Chat: non riesco a scrivere negli appunti")
		return
	}
	avvisoBreve("Inglese pronto, premi Ctrl+V: " + en)
	emetti("chat", map[string]string{"it": it, "en": en})
}

// avvisoBreve: una riga in alto nel gioco per 4 secondi (non si clicca) e nella finestra.
func avvisoBreve(testo string) {
	emetti("info", map[string]string{"testo": testo})
	fg, _, _ := pGetForeground.Call()
	area, ok := areaFinestra(fg)
	if !ok || paginaAttiva.Load() {
		return
	}
	em := 20.0
	w := min(area.W-40, int(float64(len([]rune(testo)))*em*0.56)+24)
	if w <= 0 {
		return
	}
	b := Blocco{X: (area.W - w) / 2, Y: 24, W: w, H: 34, Righe: 1, Em: em, LargMax: w,
		Sfondo: coloreFascia, Inchiostro: coloreTestoFascia, Tradotto: testo}
	MostraPagina(area, []Blocco{b})
	go func() {
		defer guard("avviso-chat")
		time.Sleep(4 * time.Second)
		if !paginaAttiva.Load() {
			NascondiPagina()
		}
	}()
}

// ---------------------------------------------------------------- appunti di Windows

var (
	pOpenClipboard    = user32.NewProc("OpenClipboard")
	pCloseClipboard   = user32.NewProc("CloseClipboard")
	pEmptyClipboard   = user32.NewProc("EmptyClipboard")
	pGetClipboardData = user32.NewProc("GetClipboardData")
	pSetClipboardData = user32.NewProc("SetClipboardData")
	kernel32Appunti   = windows.NewLazySystemDLL("kernel32.dll")
	pGlobalAlloc      = kernel32Appunti.NewProc("GlobalAlloc")
	pGlobalFree       = kernel32Appunti.NewProc("GlobalFree")
	pGlobalLock       = kernel32Appunti.NewProc("GlobalLock")
	pGlobalUnlock     = kernel32Appunti.NewProc("GlobalUnlock")
	pLstrlenW         = kernel32Appunti.NewProc("lstrlenW")
	pRtlMoveMemory    = kernel32Appunti.NewProc("RtlMoveMemory")
)

const cfUnicodeText = 13

func apriAppunti() error {
	for i := 0; i < 10; i++ {
		if r, _, _ := pOpenClipboard.Call(0); r != 0 {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return errors.New("appunti occupati")
}

func leggiAppunti() (string, error) {
	runtime.LockOSThread() // OpenClipboard e CloseClipboard devono stare sullo stesso filo
	defer runtime.UnlockOSThread()
	if err := apriAppunti(); err != nil {
		return "", err
	}
	defer pCloseClipboard.Call()
	h, _, _ := pGetClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return "", errors.New("negli appunti non c'è testo")
	}
	p, _, _ := pGlobalLock.Call(h)
	if p == 0 {
		return "", errors.New("appunti illeggibili")
	}
	defer pGlobalUnlock.Call(h)
	n, _, _ := pLstrlenW.Call(p)
	if n == 0 {
		return "", nil
	}
	n = min(n, 1<<16)
	u := make([]uint16, n)
	pRtlMoveMemory.Call(uintptr(unsafe.Pointer(&u[0])), p, n*2)
	return string(utf16.Decode(u)), nil
}

func scriviAppunti(s string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	u := utf16.Encode([]rune(s + "\x00"))
	h, _, _ := pGlobalAlloc.Call(0x0002, uintptr(len(u)*2)) // GMEM_MOVEABLE
	if h == 0 {
		return errors.New("memoria")
	}
	p, _, _ := pGlobalLock.Call(h)
	if p == 0 {
		pGlobalFree.Call(h)
		return errors.New("memoria")
	}
	pRtlMoveMemory.Call(p, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)*2))
	pGlobalUnlock.Call(h)
	if err := apriAppunti(); err != nil {
		pGlobalFree.Call(h)
		return err
	}
	defer pCloseClipboard.Call()
	pEmptyClipboard.Call()
	if r, _, _ := pSetClipboardData.Call(cfUnicodeText, h); r == 0 {
		pGlobalFree.Call(h)
		return errors.New("appunti")
	}
	return nil
}
