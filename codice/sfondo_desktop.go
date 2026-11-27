package main

// 3.2: sfondo del desktop abbinato al tema. Mai automatico: Tasso lo propone una volta (finestrella
// in Impostazioni → Aspetto) e lo mette solo col "Sì". Prima salva lo sfondo dell'utente (percorso,
// stile e una copia dell'immagine) per poterlo rimettere con "Rimetti il mio".
//
// Tutto in HKCU: niente che chieda l'amministratore. Gli sfondi 4K li mette l'installatore in
// Programmi\Tasso\sfondi\ (cartella protetta) e Windows li legge direttamente da lì; se non ci sono
// (versione zip, cartella cancellata) la funzione non si mostra. La logica pura è in sfondi/.
//
// Sicurezza: Tasso gira da amministratore, mentre HKCU (cartelle note come AppData) e settings.json
// li può cambiare qualunque programma dell'utente. Le cartelle da cui Tasso accetta di leggere
// un'immagine dell'utente vengono quindi SOLO da profiloVero() (scritto da Windows in HKLM).

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
	nomeCopiaMio        = "mio-sfondo-prima"
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

// cartelleUtente: profilo, AppData\Roaming e AppData\Local\Tasso dell'utente, costruite solo dal
// profilo scritto in HKLM (mai dalle cartelle note di HKCU). "" se il profilo non si sa.
func cartelleUtente() (profilo, roaming, datiTasso string) {
	profilo = profiloVero()
	if !sfondi.RadiceOk(profilo) {
		return "", "", ""
	}
	return profilo, filepath.Join(profilo, "AppData", "Roaming"), filepath.Join(profilo, "AppData", "Local", "Tasso")
}

// cartellaCopie: dove Tasso tiene la copia dello sfondo dell'utente: appDir\sfondi, solo se appDir
// è davvero AppData\Local\Tasso del profilo vero. "" altrimenti (niente copia, solo il percorso).
func cartellaCopie() string {
	_, _, dati := cartelleUtente()
	if dati == "" || appDir == "" || !sfondi.Sotto(appDir, dati) || !sfondi.Sotto(dati, appDir) {
		return ""
	}
	return filepath.Join(appDir, "sfondi")
}

// sfondoAttuale: il percorso dello sfondo di Windows adesso ("" = nessuna immagine).
// ok=false se Windows non ha risposto: allora non si sa, e non va scambiato per "nessuna immagine".
func sfondoAttuale() (string, bool) {
	buf := make([]uint16, 1024)
	r, _, _ := pSysParamInfo.Call(spiGetDeskWallpaper, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])), 0)
	if r == 0 {
		return "", false
	}
	return windows.UTF16ToString(buf), true
}

// impostaSfondo: mette p come sfondo ("" = nessuna immagine) e lo dice alle finestre aperte.
func impostaSfondo(p string) error {
	u, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return err
	}
	if r, _, e := pSysParamInfo.Call(spiSetDeskWallpaper, 0, uintptr(unsafe.Pointer(u)), spifUpdateIniFile); r == 0 {
		if e == nil || e == windows.ERROR_SUCCESS {
			e = errors.New("SystemParametersInfo non riuscito")
		}
		return e
	}
	// L'avviso alle finestre lo mando io, con un tempo massimo per finestra: una finestra bloccata
	// non blocca Tasso.
	area, _ := windows.UTF16PtrFromString("Control Panel\\Desktop")
	var res uintptr
	_, _, _ = pSendMsgTimeout.Call(0xFFFF, wmSettingChange, spiSetDeskWallpaper, uintptr(unsafe.Pointer(area)), smtoAbortIfHung, 2000, uintptr(unsafe.Pointer(&res)))
	return nil
}

// stileSfondo: WallpaperStyle e TileWallpaper di HKCU\Control Panel\Desktop, con "c'era o no".
type stileSfondo struct {
	Stile, Tile   string
	StileC, TileC bool
}

func leggiStile() stileSfondo {
	var s stileSfondo
	k, err := registry.OpenKey(registry.CURRENT_USER, `Control Panel\Desktop`, registry.QUERY_VALUE)
	if err != nil {
		return s
	}
	defer k.Close()
	if v, _, err := k.GetStringValue("WallpaperStyle"); err == nil {
		s.Stile, s.StileC = v, true
	}
	if v, _, err := k.GetStringValue("TileWallpaper"); err == nil {
		s.Tile, s.TileC = v, true
	}
	return s
}

// scriviStile: rimette i valori com'erano (un valore che non c'era si cancella).
func scriviStile(s stileSfondo) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Control Panel\Desktop`, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	metti := func(nome, v string, c bool) {
		if c {
			_ = k.SetStringValue(nome, v)
		} else {
			_ = k.DeleteValue(nome)
		}
	}
	metti("WallpaperStyle", s.Stile, s.StileC)
	metti("TileWallpaper", s.Tile, s.TileC)
}

// cambiaSfondo: mette stile e immagine insieme; se Windows rifiuta l'immagine, lo stile torna com'era.
func cambiaSfondo(p string, stile stileSfondo) error {
	prima := leggiStile()
	scriviStile(stile)
	if err := impostaSfondo(p); err != nil {
		scriviStile(prima)
		return err
	}
	return nil
}

// radiciSfondo: le cartelle da cui Tasso accetta di leggere un'immagine dell'utente: il suo
// profilo (con dentro AppData) e gli sfondi di Windows. Tutte da HKLM/Windows, mai da HKCU.
func radiciSfondo() []string {
	var r []string
	if profilo, _, _ := cartelleUtente(); profilo != "" {
		r = append(r, profilo)
	}
	if w := winDir(); w != "" {
		r = append(r, filepath.Join(w, "Web"))
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
		if sfondi.Sotto(vero, r) {
			ok = true
			break
		}
	}
	if !ok {
		return nil, errors.New("fuori dalle cartelle permesse")
	}
	return io.ReadAll(io.LimitReader(f, maxSfondo))
}

// usabileSfondo: per "Rimetti il mio", il file c'è, si legge in sicurezza ed è un'immagine.
func usabileSfondo(p string) bool {
	b, err := leggiImmagineUtente(p)
	return err == nil && sfondi.EstensioneDa(b) != ""
}

// preparaCartella: crea la cartella delle copie senza passare da collegamenti.
func preparaCartella(c string) error {
	if c == "" {
		return errors.New("cartella dati sconosciuta")
	}
	if err := controllaPercorsoApp(c); err != nil {
		return err
	}
	if err := os.MkdirAll(c, 0o755); err != nil {
		return err
	}
	return controllaPercorsoApp(c)
}

// scriviCopia: scrive su un file nuovo e poi lo rinomina (un collegamento o un hard link messo
// al posto del file finale viene sostituito, non seguito).
func scriviCopia(dir, nome string, b []byte) error {
	f, err := os.CreateTemp(dir, ".sfondo-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(b)
	if e := f.Close(); err == nil {
		err = e
	}
	if err == nil {
		err = rinominaApp(tmp, filepath.Join(dir, nome))
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}

// togliCopie: via le copie vecchie dello sfondo dell'utente (tutte le estensioni).
func togliCopie(dir string) {
	for _, e := range sfondi.EstensioniCopia() {
		_ = os.Remove(filepath.Join(dir, nomeCopiaMio+e))
	}
}

// cartelleSfondiTasso: dove stanno gli sfondi messi da Tasso (Programmi e, per le prove della 3.2,
// la vecchia copia nella cartella dati).
func cartelleSfondiTasso() []string {
	r := []string{cartellaSfondiProg()}
	if appDir != "" {
		r = append(r, filepath.Join(appDir, "sfondi"))
	}
	return r
}

// salvaSfondoMio: tiene da parte lo sfondo dell'utente (se quello attuale non è già di Tasso).
func salvaSfondoMio() {
	attuale, ok := sfondoAttuale()
	if !ok {
		// Non so che sfondo c'è: non registro "nessuna immagine" e non propongo "Rimetti il mio".
		updateSettings(func(s *Settings) { s.SfondoMioOk = false })
		return
	}
	if !sfondi.DaSalvare(attuale, cartelleSfondiTasso()...) {
		return
	}
	stile := leggiStile()
	// Copia dell'immagine com'è sul desktop (TranscodedWallpaper), nel caso l'originale sparisca.
	if dir := cartellaCopie(); dir != "" && preparaCartella(dir) == nil {
		togliCopie(dir) // una copia vecchia non deve passare per quella dello sfondo di adesso
		if _, roaming, _ := cartelleUtente(); attuale != "" && roaming != "" {
			if b, err := leggiImmagineUtente(filepath.Join(roaming, `Microsoft\Windows\Themes\TranscodedWallpaper`)); err == nil {
				if est := sfondi.EstensioneDa(b); est != "" {
					_ = scriviCopia(dir, nomeCopiaMio+est, b)
				}
			}
		}
	}
	updateSettings(func(s *Settings) {
		s.SfondoMio, s.SfondoMioOk = attuale, true
		s.SfondoMioStile, s.SfondoMioTile, s.SfondoMioStileC, s.SfondoMioTileC = stile.Stile, stile.Tile, stile.StileC, stile.TileC
	})
}

// sfondoMetti: mette lo sfondo di Tasso, direttamente dalla cartella protetta in Programmi.
// "ok", "manca" (sfondo non installato) o "errore".
func sfondoMetti(id string) string {
	src := fileSfondoProg(id)
	if src == "" {
		return "manca"
	}
	salvaSfondoMio()
	// Riempi: l'immagine copre tutto lo schermo senza deformarsi.
	if cambiaSfondo(src, stileSfondo{Stile: "10", Tile: "0", StileC: true, TileC: true}) != nil {
		return "errore"
	}
	updateSettings(func(s *Settings) { s.SfondoTasso = id })
	return "ok"
}

// copiaMia: la copia dello sfondo dell'utente tenuta da Tasso ("" se non c'è).
func copiaMia() string {
	dir := cartellaCopie()
	if dir == "" {
		return ""
	}
	for _, e := range sfondi.EstensioniCopia() {
		p := filepath.Join(dir, nomeCopiaMio+e)
		if fi, err := os.Lstat(p); err == nil && fi.Mode().IsRegular() {
			return p
		}
	}
	return ""
}

// sfondoRimetti: rimette lo sfondo dell'utente salvato. "ok", "niente" (niente di salvato o di
// sicuro da rimettere) o "errore".
func sfondoRimetti() string {
	setMu.Lock()
	salvato, ok := settings.SfondoMio, settings.SfondoMioOk
	stile := stileSfondo{Stile: settings.SfondoMioStile, Tile: settings.SfondoMioTile, StileC: settings.SfondoMioStileC, TileC: settings.SfondoMioTileC}
	setMu.Unlock()
	if !ok {
		return "niente"
	}
	p, ok := sfondi.Ripristino(salvato, copiaMia(), usabileSfondo)
	if !ok {
		return "niente"
	}
	if cambiaSfondo(p, stile) != nil {
		return "errore"
	}
	updateSettings(func(s *Settings) { s.SfondoTasso = "" })
	return "ok"
}

// sfondoMessoOra: lo sfondo di Tasso che Windows mostra davvero adesso ("" se l'utente l'ha
// cambiato da Windows, o se non lo so).
func sfondoMessoOra(registrato string) string {
	if registrato == "" {
		return ""
	}
	attuale, ok := sfondoAttuale()
	if !ok {
		return ""
	}
	return sfondi.Messo(registrato, attuale, cartelleSfondiTasso()...)
}

// sfondoInfo: per l'interfaccia. ok=false: la funzione non c'è su questo PC (si nasconde).
func sfondoInfo(tema string, ufficio bool) map[string]interface{} {
	id := sfondi.PerTema(tema, ufficio)
	setMu.Lock()
	registrato, mai, chiesti, mio := settings.SfondoTasso, settings.SfondoNoChiedere, append([]string(nil), settings.SfondoChiesti...), settings.SfondoMioOk
	setMu.Unlock()
	messo := sfondoMessoOra(registrato)
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

// sfondoMai: la casella "Non chiedermelo più" (salvata subito, così vale anche chiudendo con Esc).
func sfondoMai(on bool) string {
	updateSettings(func(s *Settings) { s.SfondoNoChiedere = on })
	return "ok"
}

// sfondoRisposta: la risposta alla finestrella. Ricorda "Non chiedermelo più", con il sì mette lo
// sfondo e segna lo sfondo come già proposto (un sì non riuscito non conta: si potrà richiedere).
func sfondoRisposta(id string, si, maiPiu bool) string {
	if !sfondi.Valido(id) {
		return "errore"
	}
	r := "ok"
	if si {
		r = sfondoMetti(id)
	}
	updateSettings(func(s *Settings) {
		if maiPiu {
			s.SfondoNoChiedere = true
		}
		if r != "ok" {
			return
		}
		for _, c := range s.SfondoChiesti {
			if c == id {
				return
			}
		}
		s.SfondoChiesti = append(s.SfondoChiesti, id)
	})
	return r
}
