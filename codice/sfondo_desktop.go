package main

// 3.2: sfondo del desktop abbinato al tema. Mai automatico: Tasso lo propone una volta (finestrella
// in Impostazioni → Aspetto) e lo mette solo col "Sì". Prima salva lo sfondo dell'utente (percorso,
// stile e una copia dell'immagine) per poterlo rimettere con "Rimetti il mio".
//
// Tutto in HKCU e nella cartella dati dell'utente: niente che chieda l'amministratore. Gli sfondi 4K
// li mette l'installatore in Programmi\Tasso\sfondi\ (cartella protetta); se non ci sono (versione
// zip, cartella cancellata) la funzione non si mostra. La logica pura è in sfondi/ (con i test).

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"tasso/sfondi"
)

const (
	spiGetDeskWallpaper = 0x0073
	spiSetDeskWallpaper = 0x0014
	spifUpdateIniFile   = 0x01
	wmSettingChange     = 0x001A
	smtoAbortIfHung     = 0x0002
	maxSfondo           = 64 << 20 // un'immagine più grande non la leggo
	nomeCopiaMio        = "mio-sfondo-prima.jpg"
)

var pSendMsgTimeout = user32.NewProc("SendMessageTimeoutW")

// cartellaSfondiProg: Programmi\Tasso\sfondi, solo se Tasso gira da Programmi (cartella che un
// programma senza permessi non può cambiare). "" altrimenti (es. Tasso dallo zip).
func cartellaSfondiProg() string {
	exe := tassoExe()
	if exe == "" {
		return ""
	}
	dir := filepath.Dir(exe)
	if !sfondi.Sotto(dir, programFilesDir()) {
		return ""
	}
	return filepath.Join(dir, "sfondi")
}

// fileSfondoProg: il file 4K dello sfondo installato, "" se non c'è.
func fileSfondoProg(id string) string {
	c, n := cartellaSfondiProg(), sfondi.File(id)
	if c == "" || n == "" {
		return ""
	}
	p := filepath.Join(c, n)
	if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() || fi.Size() == 0 || fi.Size() > maxSfondo {
		return ""
	}
	return p
}

// cartellaSfondiApp: dove Tasso copia gli sfondi che mette (Windows li legge da lì).
func cartellaSfondiApp() string {
	if appDir == "" {
		return ""
	}
	return filepath.Join(appDir, "sfondi")
}

// sfondoAttuale: il percorso dello sfondo di Windows adesso ("" = nessuna immagine).
func sfondoAttuale() string {
	buf := make([]uint16, 1024)
	r, _, _ := pSysParamInfo.Call(spiGetDeskWallpaper, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])), 0)
	if r == 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

// impostaSfondo: mette p come sfondo ("" = nessuna immagine) e lo dice alle finestre aperte.
func impostaSfondo(p string) error {
	u, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return err
	}
	if r, _, e := pSysParamInfo.Call(spiSetDeskWallpaper, 0, uintptr(unsafe.Pointer(u)), spifUpdateIniFile); r == 0 {
		return e
	}
	// L'avviso alle finestre lo mando io, con un tempo massimo: una finestra bloccata non blocca Tasso.
	area, _ := windows.UTF16PtrFromString("Control Panel\\Desktop")
	var res uintptr
	_, _, _ = pSendMsgTimeout.Call(0xFFFF, wmSettingChange, spiSetDeskWallpaper, uintptr(unsafe.Pointer(area)), smtoAbortIfHung, 2000, uintptr(unsafe.Pointer(&res)))
	return nil
}

// stileDesktop: WallpaperStyle e TileWallpaper di HKCU\Control Panel\Desktop.
func stileDesktop() (stile, tile string) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Control Panel\Desktop`, registry.QUERY_VALUE)
	if err != nil {
		return "", ""
	}
	defer k.Close()
	stile, _, _ = k.GetStringValue("WallpaperStyle")
	tile, _, _ = k.GetStringValue("TileWallpaper")
	return stile, tile
}

func scriviStileDesktop(stile, tile string) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Control Panel\Desktop`, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	if stile != "" {
		_ = k.SetStringValue("WallpaperStyle", stile)
	}
	if tile != "" {
		_ = k.SetStringValue("TileWallpaper", tile)
	}
}

// radiciSfondo: le cartelle da cui Tasso (che gira come amministratore) accetta di leggere
// un'immagine dell'utente: il suo profilo, AppData\Roaming e gli sfondi di Windows.
func radiciSfondo() []string {
	r := []string{profiloVero(), cartelleVere.Roaming}
	if w := winDir(); w != "" {
		r = append(r, filepath.Join(w, "Web"))
	}
	if appDir != "" {
		r = append(r, appDir)
	}
	return r
}

// leggiImmagineUtente: legge un'immagine solo se, seguiti collegamenti e giunzioni, il file vero
// sta in una delle radiciSfondo (così settings.json o un collegamento non fanno leggere a Tasso
// file di altri).
func leggiImmagineUtente(p string) ([]byte, error) {
	if !sfondi.Locale(p) {
		return nil, errors.New("percorso non locale")
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() || fi.Size() == 0 || fi.Size() > maxSfondo {
		return nil, errors.New("non è un'immagine usabile")
	}
	vero := percorsoFinale(windows.Handle(f.Fd()))
	ok := false
	for _, r := range radiciSfondo() {
		if r != "" && sfondi.Sotto(vero, r) {
			ok = true
			break
		}
	}
	if !ok {
		return nil, errors.New("fuori dalle cartelle permesse")
	}
	return io.ReadAll(io.LimitReader(f, maxSfondo))
}

// usabileSfondo: per "Rimetti il mio", il file c'è e si può leggere in sicurezza.
func usabileSfondo(p string) bool {
	_, err := leggiImmagineUtente(p)
	return err == nil
}

// preparaCartellaApp: crea AppData\Local\Tasso\sfondi senza passare da collegamenti.
func preparaCartellaApp() (string, error) {
	c := cartellaSfondiApp()
	if c == "" {
		return "", errors.New("cartella dati sconosciuta")
	}
	if err := controllaPercorsoApp(c); err != nil {
		return "", err
	}
	if err := os.MkdirAll(c, 0o755); err != nil {
		return "", err
	}
	return c, controllaPercorsoApp(c)
}

// salvaSfondoMio: tiene da parte lo sfondo dell'utente (se quello attuale non è già di Tasso).
func salvaSfondoMio(cartApp string) {
	attuale := sfondoAttuale()
	if !sfondi.DaSalvare(attuale, cartApp, cartellaSfondiProg()) {
		return
	}
	stile, tile := stileDesktop()
	// Copia dell'immagine com'è sul desktop (TranscodedWallpaper), nel caso l'originale sparisca.
	copia := filepath.Join(cartApp, nomeCopiaMio)
	ok := false
	if attuale != "" && cartelleVere.Roaming != "" {
		if b, err := leggiImmagineUtente(filepath.Join(cartelleVere.Roaming, `Microsoft\Windows\Themes\TranscodedWallpaper`)); err == nil {
			ok = scriviFileApp(copia, b, 0o644) == nil
		}
	}
	if !ok {
		_ = os.Remove(copia) // una copia vecchia non deve passare per quella dello sfondo di adesso
	}
	updateSettings(func(s *Settings) {
		s.SfondoMio, s.SfondoMioStile, s.SfondoMioTile, s.SfondoMioOk = attuale, stile, tile, true
	})
}

// sfondoMetti: mette lo sfondo di Tasso. "ok", "manca" (sfondo non installato) o "errore".
func sfondoMetti(id string) string {
	src := fileSfondoProg(id)
	if src == "" {
		return "manca"
	}
	cartApp, err := preparaCartellaApp()
	if err != nil {
		return "errore"
	}
	b, err := os.ReadFile(src)
	if err != nil || len(b) == 0 {
		return "errore"
	}
	salvaSfondoMio(cartApp)
	dst := filepath.Join(cartApp, sfondi.File(id))
	if err := scriviFileApp(dst, b, 0o644); err != nil {
		return "errore"
	}
	scriviStileDesktop("10", "0") // Riempi: l'immagine copre tutto lo schermo senza deformarsi
	if impostaSfondo(dst) != nil {
		return "errore"
	}
	updateSettings(func(s *Settings) { s.SfondoTasso = id })
	return "ok"
}

// sfondoRimetti: rimette lo sfondo dell'utente salvato. "ok", "niente" (niente di salvato o di
// sicuro da rimettere) o "errore".
func sfondoRimetti() string {
	setMu.Lock()
	salvato, stile, tile, ok := settings.SfondoMio, settings.SfondoMioStile, settings.SfondoMioTile, settings.SfondoMioOk
	setMu.Unlock()
	if !ok {
		return "niente"
	}
	copia := ""
	if c := cartellaSfondiApp(); c != "" {
		copia = filepath.Join(c, nomeCopiaMio)
	}
	p, ok := sfondi.Ripristino(salvato, copia, usabileSfondo)
	if !ok {
		return "niente"
	}
	scriviStileDesktop(stile, tile)
	if impostaSfondo(p) != nil {
		return "errore"
	}
	updateSettings(func(s *Settings) { s.SfondoTasso = "" })
	return "ok"
}

// sfondoInfo: per l'interfaccia. ok=false: la funzione non c'è su questo PC (si nasconde).
func sfondoInfo(tema string, ufficio bool) map[string]interface{} {
	id := sfondi.PerTema(tema, ufficio)
	setMu.Lock()
	messo, mai, chiesti, mio := settings.SfondoTasso, settings.SfondoNoChiedere, append([]string(nil), settings.SfondoChiesti...), settings.SfondoMioOk
	setMu.Unlock()
	ok := false
	for _, x := range sfondi.Tutti() {
		if fileSfondoProg(x) != "" {
			ok = true
			break
		}
	}
	if id != "" && fileSfondoProg(id) == "" {
		id = ""
	}
	m := map[string]interface{}{"ok": ok, "id": id, "messo": messo, "mio": mio,
		"chiedi": ok && sfondi.DaChiedere(id, messo, mai, chiesti)}
	if id != "" {
		m["mini"] = sfondi.MiniDataURL(id)
	}
	return m
}

// sfondoRisposta: la risposta alla finestrella. Segna lo sfondo come già proposto, ricorda
// "Non chiedermelo più" e con il sì lo mette.
func sfondoRisposta(id string, si, maiPiu bool) string {
	if !sfondi.Valido(id) {
		return "errore"
	}
	updateSettings(func(s *Settings) {
		if maiPiu {
			s.SfondoNoChiedere = true
		}
		for _, c := range s.SfondoChiesti {
			if c == id {
				return
			}
		}
		s.SfondoChiesti = append(s.SfondoChiesti, id)
	})
	if !si {
		return "ok"
	}
	return sfondoMetti(id)
}
