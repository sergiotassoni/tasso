//go:build windows

// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

// sistema_windows.go: le parti di Windows (finestre "Apri" e "Salva con nome", browser per i
// collegamenti, DPAPI per la firma salvata, lingua, foto HEIC con i componenti di Windows).

import (
	"errors"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var hwndPrincipale uintptr

var (
	comdlg32           = windows.NewLazySystemDLL("comdlg32.dll")
	pGetOpenFileName   = comdlg32.NewProc("GetOpenFileNameW")
	pGetSaveFileName   = comdlg32.NewProc("GetSaveFileNameW")
	pCommDlgExtError   = comdlg32.NewProc("CommDlgExtendedError")
	shell32            = windows.NewLazySystemDLL("shell32.dll")
	pShellExecute      = shell32.NewProc("ShellExecuteW")
	kernel32           = windows.NewLazySystemDLL("kernel32.dll")
	pGetUserUILanguage = kernel32.NewProc("GetUserDefaultUILanguage")
	user32             = windows.NewLazySystemDLL("user32.dll")
	pMessageBox        = user32.NewProc("MessageBoxW")
	shlwapi            = windows.NewLazySystemDLL("shlwapi.dll")
	pSHCreateMemStream = shlwapi.NewProc("SHCreateMemStream")
	ole32              = windows.NewLazySystemDLL("ole32.dll")
	pCoCreateInstance  = ole32.NewProc("CoCreateInstance")
)

type openFileName struct {
	lStructSize       uint32
	hwndOwner         uintptr
	hInstance         uintptr
	lpstrFilter       *uint16
	lpstrCustomFilter *uint16
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         *uint16
	nMaxFile          uint32
	lpstrFileTitle    *uint16
	nMaxFileTitle     uint32
	lpstrInitialDir   *uint16
	lpstrTitle        *uint16
	flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        uintptr
	dwReserved        uint32
	flagsEx           uint32
}

const (
	ofnOverwritePrompt = 0x00000002
	ofnHideReadOnly    = 0x00000004
	ofnNoChangeDir     = 0x00000008
	ofnAllowMulti      = 0x00000200
	ofnPathMustExist   = 0x00000800
	ofnFileMustExist   = 0x00001000
	ofnExplorer        = 0x00080000
	ofnNoReadOnlyRet   = 0x00008000
)

// filtro: il testo dei filtri della finestra, con gli zeri di separazione.
func filtro(s string) *uint16 {
	u := utf16.Encode([]rune(s))
	return &u[0]
}

func cartellaDocumenti() string {
	p, err := windows.KnownFolderPath(windows.FOLDERID_Documents, 0)
	if err != nil {
		return ""
	}
	return p
}

func scegliFileWin(multi, soloPDF bool, titolo string) ([]string, error) {
	buf := make([]uint16, 1<<16)
	f := "PDF (*.pdf)\x00*.pdf\x00\x00"
	if !soloPDF {
		f = "PDF, JPG, PNG, HEIC\x00*.pdf;*.jpg;*.jpeg;*.png;*.heic;*.heif\x00PDF (*.pdf)\x00*.pdf\x00JPG, PNG, HEIC\x00*.jpg;*.jpeg;*.png;*.heic;*.heif\x00\x00"
	}
	t, _ := windows.UTF16PtrFromString(titolo)
	ofn := openFileName{
		hwndOwner:   hwndPrincipale,
		lpstrFilter: filtro(f),
		lpstrFile:   &buf[0],
		nMaxFile:    uint32(len(buf)),
		lpstrTitle:  t,
		flags:       ofnFileMustExist | ofnPathMustExist | ofnNoChangeDir | ofnExplorer | ofnHideReadOnly,
	}
	if multi {
		ofn.flags |= ofnAllowMulti
	}
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	r, _, _ := pGetOpenFileName.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		if e, _, _ := pCommDlgExtError.Call(); e != 0 {
			return nil, errors.New("finestra Apri: errore di Windows")
		}
		return nil, nil // annullato
	}
	// Un file: "C:\cartella\file". Più file: "C:\cartella\0file1\0file2\0\0".
	var parti []string
	inizio := 0
	for i, c := range buf {
		if c == 0 {
			if i == inizio {
				break
			}
			parti = append(parti, windows.UTF16ToString(buf[inizio:i]))
			inizio = i + 1
		}
	}
	if len(parti) <= 1 {
		return parti, nil
	}
	var out []string
	for _, n := range parti[1:] {
		out = append(out, filepath.Join(parti[0], n))
	}
	return out, nil
}

// filtriSalva: il filtro della finestra "Salva con nome" per ogni tipo di file.
var filtriSalva = map[string]string{
	"pdf": "PDF (*.pdf)\x00*.pdf\x00\x00",
	"txt": "Testo (*.txt)\x00*.txt\x00\x00",
	"png": "PNG (*.png)\x00*.png\x00\x00",
	"jpg": "JPG (*.jpg)\x00*.jpg\x00\x00",
}

func salvaFileWin(nome, cartella, titolo, tipo string) (string, error) {
	if _, ok := filtriSalva[tipo]; !ok {
		tipo = "pdf"
	}
	buf := make([]uint16, 32768)
	copy(buf, utf16.Encode([]rune(nome)))
	t, _ := windows.UTF16PtrFromString(titolo)
	if cartella == "" {
		cartella = cartellaDocumenti()
	}
	dir, _ := windows.UTF16PtrFromString(cartella)
	ext, _ := windows.UTF16PtrFromString(tipo)
	ofn := openFileName{
		hwndOwner:       hwndPrincipale,
		lpstrFilter:     filtro(filtriSalva[tipo]),
		lpstrFile:       &buf[0],
		nMaxFile:        uint32(len(buf)),
		lpstrTitle:      t,
		lpstrInitialDir: dir,
		lpstrDefExt:     ext,
		// OVERWRITEPROMPT: se il file esiste già, Windows chiede prima di sostituirlo.
		flags: ofnOverwritePrompt | ofnPathMustExist | ofnNoChangeDir | ofnExplorer | ofnHideReadOnly | ofnNoReadOnlyRet,
	}
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	r, _, _ := pGetSaveFileName.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		if e, _, _ := pCommDlgExtError.Call(); e != 0 {
			return "", errors.New("finestra Salva: errore di Windows")
		}
		return "", nil
	}
	return windows.UTF16ToString(buf), nil
}

func shellApri(verbo, file, param string) error {
	v, _ := windows.UTF16PtrFromString(verbo)
	f, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return err
	}
	var p *uint16
	if param != "" {
		p, _ = windows.UTF16PtrFromString(param)
	}
	r, _, _ := pShellExecute.Call(hwndPrincipale, uintptr(unsafe.Pointer(v)), uintptr(unsafe.Pointer(f)), uintptr(unsafe.Pointer(p)), 0, 1)
	if r <= 32 {
		return errors.New("impossibile aprire")
	}
	return nil
}

func apriLinkWin(u string) error { return shellApri("open", u, "") }

func mostraFileWin(p string) error {
	if _, err := os.Stat(p); err != nil {
		return err
	}
	win := os.Getenv("SystemRoot")
	if win == "" {
		win = `C:\Windows`
	}
	return shellApri("open", filepath.Join(win, "explorer.exe"), `/select,"`+p+`"`)
}

func blob(b []byte) *windows.DataBlob {
	if len(b) == 0 {
		return &windows.DataBlob{}
	}
	return &windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
}

var entropiaFirma = []byte("Tasso PDF - firma salvata")

func proteggiWin(b []byte) ([]byte, error) {
	var out windows.DataBlob
	nome, _ := windows.UTF16PtrFromString("Tasso PDF")
	const uiForbidden = 0x1
	if err := windows.CryptProtectData(blob(b), nome, blob(entropiaFirma), 0, nil, uiForbidden, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

func svelaWin(b []byte) ([]byte, error) {
	var out windows.DataBlob
	const uiForbidden = 0x1
	if err := windows.CryptUnprotectData(blob(b), nil, blob(entropiaFirma), 0, nil, uiForbidden, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

// cartellaDatiWin: %LOCALAPPDATA%\Tasso PDF
func cartellaDatiWin() string {
	p, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil || p == "" {
		p = os.Getenv("LOCALAPPDATA")
	}
	if p == "" {
		return ""
	}
	return filepath.Join(p, "Tasso PDF")
}

func linguaTassoWin() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Tasso`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	s, _, err := k.GetStringValue("Lingua")
	if err != nil {
		return ""
	}
	return s
}

func linguaSistemaWin() string {
	r, _, _ := pGetUserUILanguage.Call()
	switch uint16(r) & 0x3FF { // lingua principale
	case 0x10:
		return "it"
	case 0x09:
		return "en"
	case 0x0A:
		return "es"
	case 0x0C:
		return "fr"
	case 0x07:
		return "de"
	case 0x16:
		return "pt"
	case 0x15:
		return "pl"
	}
	return ""
}

func messaggio(testo, titolo string) {
	t, _ := windows.UTF16PtrFromString(testo)
	ti, _ := windows.UTF16PtrFromString(titolo)
	_, _, _ = pMessageBox.Call(hwndPrincipale, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(ti)), 0x40)
}

// ---------------------------------------------------------------- HEIC con WIC

var (
	clsidWICImagingFactory = windows.GUID{Data1: 0xcacaf262, Data2: 0x9370, Data3: 0x4615, Data4: [8]byte{0xa1, 0x3b, 0x9f, 0x55, 0x39, 0xda, 0x4c, 0x0a}}
	iidIWICImagingFactory  = windows.GUID{Data1: 0xec5ec8a9, Data2: 0xc395, Data3: 0x4314, Data4: [8]byte{0x9c, 0x77, 0x54, 0xd7, 0xa9, 0x35, 0xff, 0x70}}
	guidPixel32bppBGRA     = windows.GUID{Data1: 0x6fddc324, Data2: 0x4e03, Data3: 0x4bfe, Data4: [8]byte{0xb1, 0x85, 0x3d, 0x77, 0x76, 0x8d, 0xc9, 0x0f}}
)

// comObj: un oggetto COM (il primo campo è la tabella dei metodi).
type comObj struct{ vtbl *[64]uintptr }

func (o *comObj) chiama(i int, args ...uintptr) uintptr {
	a := append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)
	r, _, _ := syscall.SyscallN(o.vtbl[i], a...)
	return r
}

func (o *comObj) rilascia() {
	if o != nil {
		o.chiama(2)
	}
}

func fallito(hr uintptr) bool { return int32(hr) < 0 }

// decodificaHEICWin: legge una foto HEIC con i componenti di Windows (serve l'estensione
// "Immagini HEIF" e, per molte foto, "Estensioni video HEVC" dal Microsoft Store).
func decodificaHEICWin(data []byte) (img image.Image, err error) {
	if len(data) == 0 {
		return nil, errors.New("vuoto")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if hr := windows.CoInitializeEx(0, windows.COINIT_MULTITHREADED); hr == nil {
		defer windows.CoUninitialize()
	}
	defer func() {
		if r := recover(); r != nil {
			img, err = nil, errors.New("errore nel leggere la foto HEIC")
		}
	}()
	var fact *comObj
	const clsctxInproc = 0x1
	hr, _, _ := pCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidWICImagingFactory)), 0, clsctxInproc,
		uintptr(unsafe.Pointer(&iidIWICImagingFactory)), uintptr(unsafe.Pointer(&fact)))
	if fallito(hr) || fact == nil {
		return nil, errors.New("WIC non disponibile")
	}
	defer fact.rilascia()
	st, _, _ := pSHCreateMemStream.Call(uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)))
	if st == 0 {
		return nil, errors.New("memoria")
	}
	stream := *(**comObj)(unsafe.Pointer(&st))
	defer stream.rilascia()
	var dec *comObj
	// IWICImagingFactory::CreateDecoderFromStream (4)
	if hr := fact.chiama(4, st, 0, 0, uintptr(unsafe.Pointer(&dec))); fallito(hr) || dec == nil {
		return nil, errors.New("questo PC non sa leggere le foto HEIC")
	}
	defer dec.rilascia()
	var frame *comObj
	// IWICBitmapDecoder::GetFrame (13)
	if hr := dec.chiama(13, 0, uintptr(unsafe.Pointer(&frame))); fallito(hr) || frame == nil {
		return nil, errors.New("foto HEIC senza immagine")
	}
	defer frame.rilascia()
	var conv *comObj
	// IWICImagingFactory::CreateFormatConverter (10)
	if hr := fact.chiama(10, uintptr(unsafe.Pointer(&conv))); fallito(hr) || conv == nil {
		return nil, errors.New("conversione non disponibile")
	}
	defer conv.rilascia()
	// IWICFormatConverter::Initialize (8): sorgente, formato, dither none, palette nil, soglia 0, palette custom
	if hr := conv.chiama(8, uintptr(unsafe.Pointer(frame)), uintptr(unsafe.Pointer(&guidPixel32bppBGRA)), 0, 0, 0, 0); fallito(hr) {
		return nil, errors.New("conversione della foto HEIC non riuscita")
	}
	var w, h uint32
	// IWICBitmapSource::GetSize (3)
	if hr := conv.chiama(3, uintptr(unsafe.Pointer(&w)), uintptr(unsafe.Pointer(&h))); fallito(hr) || w == 0 || h == 0 || w > 20000 || h > 20000 {
		return nil, errors.New("dimensioni della foto HEIC non valide")
	}
	rgba := image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
	stride := uint32(w * 4)
	// IWICBitmapSource::CopyPixels (7)
	if hr := conv.chiama(7, 0, uintptr(stride), uintptr(len(rgba.Pix)), uintptr(unsafe.Pointer(&rgba.Pix[0]))); fallito(hr) {
		return nil, errors.New("lettura della foto HEIC non riuscita")
	}
	// BGRA -> RGBA
	p := rgba.Pix
	for i := 0; i+3 < len(p); i += 4 {
		p[i], p[i+2] = p[i+2], p[i]
	}
	return rgba, nil
}

func init() {
	piattaforma.scegliFile = scegliFileWin
	piattaforma.salvaFile = salvaFileWin
	piattaforma.apriLink = apriLinkWin
	piattaforma.mostraFile = mostraFileWin
	piattaforma.proteggi = proteggiWin
	piattaforma.svela = svelaWin
	piattaforma.cartellaDati = cartellaDatiWin
	piattaforma.linguaTasso = linguaTassoWin
	piattaforma.linguaSistema = linguaSistemaWin
	decodificaHEIC = decodificaHEICWin
}
