package main

// Cancellazione sicura per i passaggi di pulizia (Clear-Folder, Remove-Safe e Remove-Old in steps.go).
//
// Il problema: Tasso gira da amministratore, ma pulisce cartelle dove può scrivere anche un programma
// qualsiasi dell'utente (la cache in AppData, i temporanei, la cache di Steam...). Fra il controllo
// "qui non ci sono giunzioni" e la cancellazione vera, quel programma potrebbe sostituire una
// sottocartella con una giunzione verso System32: un PowerShell che cancella per nome la seguirebbe
// e cancellerebbe file di Windows.
//
// La soluzione: qui non si cancella mai partendo dal nome completo. Apro la cartella di partenza (la
// "radice fidata") e la tengo aperta; ogni cartella e ogni file sotto si apre partendo dalla cartella
// già aperta che lo contiene, senza mai seguire giunzioni o collegamenti (FILE_OPEN_REPARSE_POINT).
// Una giunzione trovata dentro si cancella lei (solo il collegamento), mai quello a cui punta.
// Cambiare una cartella dopo il controllo non serve: io lavoro su quella che ho già aperto, non su
// quella che adesso ha quel nome.
//
// Gli script la usano così:
//
//	Tasso.exe --cancella-sicura svuota <radice> <cartella> <ore>   (svuota la cartella, lei resta)
//	Tasso.exe --cancella-sicura togli <elemento> <ore>             (toglie file o cartella)
//
// <ore> = 0: tutto; altrimenti solo quello più vecchio di tante ore. Risposta su una riga:
// "LIBERATI n" (byte liberati) oppure "RIFIUTO motivo" (non ho toccato niente).

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const argCancellaSicura = "--cancella-sicura"

// cancellaSicuraDaRiga: il lavoro chiesto dallo script. Restituisce il codice di uscita (0 fatto, 3 rifiutato).
func cancellaSicuraDaRiga(args []string) (codice int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("RIFIUTO errore interno")
			codice = 3
		}
	}()
	rifiuta := func(m string) int { fmt.Println("RIFIUTO " + m); return 3 }
	if len(args) < 1 {
		return rifiuta("comando mancante")
	}
	c := &cancellatore{fine: scadenzaDaAmbiente()}
	ore := func(s string) bool {
		h, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil || h < 0 || h > 24*365*20 {
			return false
		}
		if h > 0 {
			c.limite = time.Now().Add(-time.Duration(h * float64(time.Hour)))
		}
		return true
	}
	switch args[0] {
	case "svuota":
		if len(args) != 4 || !ore(args[3]) {
			return rifiuta("argomenti sbagliati")
		}
		if err := c.svuotaCartella(args[1], args[2]); err != nil {
			return rifiuta(err.Error())
		}
	case "togli":
		if len(args) != 3 || !ore(args[2]) {
			return rifiuta("argomenti sbagliati")
		}
		if err := c.togliElemento(args[1]); err != nil {
			return rifiuta(err.Error())
		}
	default:
		return rifiuta("comando sconosciuto")
	}
	fmt.Println("LIBERATI " + strconv.FormatInt(c.liberati, 10))
	return 0
}

type cancellatore struct {
	limite   time.Time // zero: si cancella tutto
	liberati int64
	fine     time.Time // zero: nessuna scadenza; passata, mi fermo e dico quanto ho liberato fin lì
}

// scadenzaDaAmbiente: TASSO_FINE (secondi Unix) lo mette lo script del passaggio, così con cartelle
// enormi (milioni di file di cache) mi fermo prima che il passaggio venga chiuso per il tempo.
// Fermarsi prima toglie solo meno roba: non può mai far cancellare qualcosa in più.
func scadenzaDaAmbiente() time.Time {
	v := strings.TrimSpace(os.Getenv("TASSO_FINE"))
	if v == "" {
		return time.Time{}
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return time.Time{}
	}
	return time.Unix(n, 0)
}

// scaduto: è passata la scadenza (controllo leggero, fatto per ogni elemento).
func (c *cancellatore) scaduto() bool {
	return !c.fine.IsZero() && time.Now().After(c.fine)
}

type errCS string

func (e errCS) Error() string { return string(e) }

// percorsoLocale: solo percorsi completi su un disco con la lettera (niente rete, niente \\?\).
func percorsoLocale(p string) (string, bool) {
	p = strings.TrimSpace(p)
	if p == "" || strings.HasPrefix(p, `\\`) || strings.HasPrefix(p, `//`) || strings.ContainsAny(p, "*?\"<>|") {
		return "", false
	}
	p = filepath.Clean(p)
	v := filepath.VolumeName(p)
	if len(v) != 2 || v[1] != ':' || !filepath.IsAbs(p) {
		return "", false
	}
	if strings.Contains(p[2:], ":") { // niente flussi nascosti (file:flusso)
		return "", false
	}
	return p, true
}

// dentroDi: p è base o sta dentro base (senza badare a maiuscole e minuscole).
func dentroDi(p, base string) bool {
	p, base = strings.TrimRight(p, `\`), strings.TrimRight(base, `\`)
	if base == "" || p == "" {
		return false
	}
	if strings.EqualFold(p, base) {
		return true
	}
	return len(p) > len(base) && p[len(base)] == '\\' && strings.EqualFold(p[:len(base)], base)
}

// percorsoFinale: dove si trova davvero la cartella aperta (dopo tutte le giunzioni). "" se non si sa
// o se è in rete.
func percorsoFinale(h windows.Handle) string {
	buf := make([]uint16, 1024)
	for i := 0; i < 2; i++ {
		n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0) // nome normale, con la lettera del disco
		if err != nil || n == 0 {
			return ""
		}
		if int(n) < len(buf) {
			s := windows.UTF16ToString(buf[:n])
			s = strings.TrimPrefix(s, `\\?\`)
			if strings.HasPrefix(strings.ToUpper(s), `UNC\`) || strings.HasPrefix(s, `\\`) {
				return "" // cartella di rete
			}
			return strings.TrimRight(s, `\`)
		}
		buf = make([]uint16, n+1)
	}
	return ""
}

// apriCartellaPerNome: apre una cartella partendo dal suo nome completo (qui le giunzioni SOPRA e
// sulla cartella stessa si seguono: per questo dopo si controlla dove porta davvero).
func apriCartellaPerNome(p string) (windows.Handle, error) {
	u, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return 0, err
	}
	h, err := windows.CreateFile(u, windows.FILE_LIST_DIRECTORY|windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return 0, err
	}
	var fi windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(h, &fi) != nil || fi.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		_ = windows.CloseHandle(h)
		return 0, errCS("non è una cartella")
	}
	return h, nil
}

// finaleDi: dove porta davvero una cartella data per nome ("" se non si apre).
func finaleDi(p string) string {
	if p == "" {
		return ""
	}
	h, err := apriCartellaPerNome(p)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	return percorsoFinale(h)
}

// radiceAccettabile: la radice aperta va bene se è proprio dove dice il suo nome. Se no (una giunzione
// sopra la radice o sulla radice: C:\Users spostato su D:, Temp su un disco in memoria, Steam spostato)
// va bene solo se non porta dentro Windows, Programmi, ProgramData, il profilo di un altro utente o
// un disco intero.
func radiceAccettabile(atteso, finale string) error {
	if finale == "" {
		return errCS("non so dove si trova davvero la cartella (in rete?)")
	}
	if strings.EqualFold(strings.TrimRight(atteso, `\`), finale) {
		return nil
	}
	if len(finale) <= 3 {
		return errCS("porta a un disco intero")
	}
	c := cartelleVere
	for _, d := range []string{c.Win, c.PF, c.PF86, c.PD} {
		if d == "" {
			continue
		}
		if dentroDi(finale, d) || dentroDi(finale, finaleDi(d)) {
			return errCS("porta in una cartella di sistema")
		}
	}
	for _, prof := range []string{c.Profilo, finaleDi(c.Profilo)} {
		if prof == "" {
			continue
		}
		utenti := filepath.Dir(prof)
		if dentroDi(finale, utenti) && !dentroDi(finale, prof) {
			return errCS("porta nella cartella di un altro utente")
		}
	}
	return nil
}

// apriRadice: apre la radice fidata e controlla dove si trova davvero.
func apriRadice(radice string) (windows.Handle, string, error) {
	h, err := apriCartellaPerNome(radice)
	if err != nil {
		return 0, "", err
	}
	fin := percorsoFinale(h)
	if err := radiceAccettabile(radice, fin); err != nil {
		_ = windows.CloseHandle(h)
		return 0, "", err
	}
	return h, fin, nil
}

// apriSotto: apre "nome" dentro la cartella già aperta dir, senza mai seguire giunzioni o collegamenti.
// Con nome "" riapre lo stesso oggetto di dir (con altri permessi).
func apriSotto(dir windows.Handle, nome string, accesso, opzioni uint32) (windows.Handle, error) {
	ns, err := windows.NewNTUnicodeString(nome)
	if err != nil {
		return 0, err
	}
	oa := windows.OBJECT_ATTRIBUTES{RootDirectory: dir, ObjectName: ns, Attributes: windows.OBJ_CASE_INSENSITIVE}
	oa.Length = uint32(unsafe.Sizeof(oa))
	var h windows.Handle
	var iosb windows.IO_STATUS_BLOCK
	st := windows.NtCreateFile(&h, accesso|windows.SYNCHRONIZE, &oa, &iosb, nil, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_OPEN,
		opzioni|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if st != nil {
		return 0, st
	}
	return h, nil
}

func nonTrovato(err error) bool {
	return err == windows.STATUS_OBJECT_NAME_NOT_FOUND || err == windows.STATUS_OBJECT_PATH_NOT_FOUND ||
		err == windows.ERROR_FILE_NOT_FOUND || err == windows.ERROR_PATH_NOT_FOUND
}

// nomeValido: un solo pezzo di percorso, senza trucchi.
func nomeValido(n string) bool {
	return n != "" && n != "." && n != ".." && !strings.ContainsAny(n, `\/:*?"<>|`)
}

// svuotaCartella: svuota cartella (che sta dentro radice o è la radice stessa), che resta.
func (c *cancellatore) svuotaCartella(radice, cartella string) error {
	r, ok1 := percorsoLocale(radice)
	p, ok2 := percorsoLocale(cartella)
	if !ok1 || !ok2 || len(r) <= 3 {
		return errCS("percorso non valido")
	}
	if !dentroDi(p, r) {
		return errCS("la cartella non è dentro la radice")
	}
	hr, fin, err := apriRadice(r)
	if err != nil {
		if nonTrovato(err) {
			return nil // non c'è: niente da fare
		}
		return err
	}
	aperte := []windows.Handle{hr}
	defer func() {
		for _, h := range aperte {
			_ = windows.CloseHandle(h)
		}
	}()
	cur := hr
	if rel := strings.Trim(p[len(r):], `\`); rel != "" {
		// Dalla radice in giù, un pezzo alla volta: nessuna di queste cartelle può essere una giunzione.
		for _, n := range strings.Split(rel, `\`) {
			if !nomeValido(n) {
				return errCS("percorso non valido")
			}
			h, err := apriSotto(cur, n, windows.FILE_LIST_DIRECTORY|windows.FILE_READ_ATTRIBUTES, windows.FILE_DIRECTORY_FILE)
			if err != nil {
				if nonTrovato(err) {
					return nil
				}
				return errCS("non riesco ad aprire " + n)
			}
			aperte = append(aperte, h)
			var fi windows.ByHandleFileInformation
			if windows.GetFileInformationByHandle(h, &fi) != nil {
				return errCS("non riesco a leggere " + n)
			}
			if fi.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
				return errCS("c'è una giunzione o un collegamento: " + n)
			}
			cur = h
		}
	}
	// Controllo in più: la cartella aperta sta davvero sotto la radice.
	f := percorsoFinale(cur)
	if f == "" || !dentroDi(f, fin) {
		return errCS("la cartella non è dove dovrebbe")
	}
	// L'elenco delle cartelle ammesse lo decide Tasso qui, non lo script (cancella_ammessi.go).
	if !percorsoAmmesso(f, elencoAmmessi()) {
		return errCS("non è fra le cartelle che Tasso pulisce: " + f)
	}
	c.svuota(cur, 0)
	return nil
}

// togliElemento: toglie un file o una cartella (con tutto dentro). Una giunzione o un collegamento
// si toglie lui, mai quello a cui punta. La cartella che lo contiene fa da radice.
func (c *cancellatore) togliElemento(elemento string) error {
	p, ok := percorsoLocale(elemento)
	if !ok || len(p) <= 3 {
		return errCS("percorso non valido")
	}
	padre, nome := filepath.Dir(p), filepath.Base(p)
	if !nomeValido(nome) {
		return errCS("percorso non valido")
	}
	hp, finPadre, err := apriRadice(padre)
	if err != nil {
		if nonTrovato(err) {
			return nil
		}
		return err
	}
	defer windows.CloseHandle(hp)
	if dove := finPadre + `\` + nome; !percorsoAmmesso(dove, elencoAmmessi()) {
		return errCS("non è fra le cartelle che Tasso pulisce: " + dove)
	}
	c.togli(hp, nome, 0)
	return nil
}

type voce struct {
	nome string
}

// elenca: i nomi dentro la cartella aperta (letti tutti prima di cancellare).
func elenca(dir windows.Handle) []voce {
	var out []voce
	buf := make([]uint64, 8192) // 64 KB, allineati
	b := unsafe.Slice((*byte)(unsafe.Pointer(&buf[0])), len(buf)*8)
	classe := uint32(windows.FileFullDirectoryRestartInfo)
	for len(out) < 2000000 {
		if err := windows.GetFileInformationByHandleEx(dir, classe, &b[0], uint32(len(b))); err != nil {
			break // ERROR_NO_MORE_FILES o un errore: mi fermo
		}
		classe = windows.FileFullDirectoryInfo
		off := 0
		for off+68 <= len(b) {
			next := int(binary.LittleEndian.Uint32(b[off:]))
			ln := int(binary.LittleEndian.Uint32(b[off+60:]))
			if ln > 0 && off+68+ln <= len(b) {
				u := unsafe.Slice((*uint16)(unsafe.Pointer(&b[off+68])), ln/2)
				n := windows.UTF16ToString(u)
				if n != "." && n != ".." {
					out = append(out, voce{nome: n})
				}
			}
			if next == 0 {
				break
			}
			off += next
		}
	}
	return out
}

// vecchio: il momento t viene prima del limite (o non c'è limite).
func (c *cancellatore) vecchio(t windows.Filetime) bool {
	if c.limite.IsZero() {
		return true
	}
	return time.Unix(0, t.Nanoseconds()).Before(c.limite)
}

// svuota: toglie quello che c'è dentro la cartella aperta (lei resta).
func (c *cancellatore) svuota(dir windows.Handle, prof int) {
	for _, v := range elenca(dir) {
		if c.scaduto() {
			return
		}
		if nomeValido(v.nome) {
			c.togli(dir, v.nome, prof)
		}
	}
}

// togli: toglie "nome" dentro la cartella aperta dir. Le decisioni si prendono su quello che è stato
// aperto davvero (non su quello che diceva l'elenco).
func (c *cancellatore) togli(dir windows.Handle, nome string, prof int) {
	acc := uint32(windows.DELETE | windows.FILE_READ_ATTRIBUTES | windows.FILE_WRITE_ATTRIBUTES)
	h, err := apriSotto(dir, nome, acc, 0)
	if err != nil {
		acc &^= windows.FILE_WRITE_ATTRIBUTES
		if h, err = apriSotto(dir, nome, acc, 0); err != nil {
			return // in uso o senza permesso: resta
		}
	}
	defer windows.CloseHandle(h)
	var fi windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(h, &fi) != nil {
		return
	}
	a := fi.FileAttributes
	switch {
	case a&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0:
		// Giunzione o collegamento: tolgo solo il collegamento.
		if c.vecchio(fi.LastWriteTime) {
			_ = cancellaAperto(h, a, acc)
		}
	case a&windows.FILE_ATTRIBUTE_DIRECTORY != 0:
		if prof > 256 {
			return
		}
		// Riapro la stessa cartella (non per nome: proprio quella già aperta) per leggerne l'elenco.
		const lista = windows.FILE_LIST_DIRECTORY | windows.FILE_READ_ATTRIBUTES
		hl, err := apriSotto(h, "", lista, windows.FILE_DIRECTORY_FILE)
		if err != nil {
			// Se Windows non la riapre così, la riapro per nome dentro la cartella sopra (sempre senza
			// seguire giunzioni) e controllo che sia proprio la stessa cartella di prima.
			if h2, err2 := apriSotto(dir, nome, lista, windows.FILE_DIRECTORY_FILE); err2 == nil {
				var f2 windows.ByHandleFileInformation
				if windows.GetFileInformationByHandle(h2, &f2) == nil && stessoFile(&fi, &f2) && f2.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT == 0 {
					hl, err = h2, nil
				} else {
					_ = windows.CloseHandle(h2)
				}
			}
		}
		if err == nil {
			c.svuota(hl, prof+1)
			_ = windows.CloseHandle(hl)
		}
		// Solo se vuota (Windows rifiuta di cancellare una cartella piena) e creata da tempo.
		if c.vecchio(fi.CreationTime) {
			_ = cancellaAperto(h, a, acc)
		}
	default:
		// Anche la data di creazione: un file appena estratto da uno zip può avere una data di modifica vecchia.
		if c.vecchio(fi.LastWriteTime) && c.vecchio(fi.CreationTime) {
			sz := int64(fi.FileSizeHigh)<<32 | int64(fi.FileSizeLow)
			if cancellaAperto(h, a, acc) && fi.NumberOfLinks <= 1 {
				c.liberati += sz
			}
		}
	}
}

// stessoFile: le due maniglie aprono lo stesso file (stesso disco e stesso numero del file).
func stessoFile(a, b *windows.ByHandleFileInformation) bool {
	return a.VolumeSerialNumber == b.VolumeSerialNumber && a.FileIndexHigh == b.FileIndexHigh && a.FileIndexLow == b.FileIndexLow
}

// infoBase: FILE_BASIC_INFO di Windows (per togliere "sola lettura").
type infoBase struct {
	CreationTime, LastAccessTime, LastWriteTime, ChangeTime int64
	FileAttributes                                          uint32
	_                                                       uint32
}

// attrImpostabili: gli attributi che si possono riscrivere (non "cartella" o "collegamento").
const attrImpostabili = windows.FILE_ATTRIBUTE_READONLY | windows.FILE_ATTRIBUTE_HIDDEN | windows.FILE_ATTRIBUTE_SYSTEM |
	windows.FILE_ATTRIBUTE_ARCHIVE | windows.FILE_ATTRIBUTE_TEMPORARY | windows.FILE_ATTRIBUTE_OFFLINE |
	windows.FILE_ATTRIBUTE_NOT_CONTENT_INDEXED

// cancellaAperto: segna da cancellare il file o la cartella aperta (sparisce subito).
func cancellaAperto(h windows.Handle, attr, acc uint32) bool {
	fl := uint32(windows.FILE_DISPOSITION_DELETE | windows.FILE_DISPOSITION_POSIX_SEMANTICS | windows.FILE_DISPOSITION_IGNORE_READONLY_ATTRIBUTE)
	if windows.SetFileInformationByHandle(h, windows.FileDispositionInfoEx, (*byte)(unsafe.Pointer(&fl)), 4) == nil {
		return true
	}
	// Windows più vecchio di 10 1809: tolgo "sola lettura" e uso il modo classico.
	tolto := false
	if attr&windows.FILE_ATTRIBUTE_READONLY != 0 && acc&windows.FILE_WRITE_ATTRIBUTES != 0 {
		ib := infoBase{FileAttributes: attr & attrImpostabili &^ windows.FILE_ATTRIBUTE_READONLY}
		if ib.FileAttributes == 0 {
			ib.FileAttributes = windows.FILE_ATTRIBUTE_NORMAL
		}
		tolto = windows.SetFileInformationByHandle(h, windows.FileBasicInfo, (*byte)(unsafe.Pointer(&ib)), uint32(unsafe.Sizeof(ib))) == nil
	}
	del := byte(1)
	if windows.SetFileInformationByHandle(h, windows.FileDispositionInfo, &del, 1) == nil {
		return true
	}
	if tolto { // non cancellato: rimetto "sola lettura"
		ib := infoBase{FileAttributes: attr & attrImpostabili}
		_ = windows.SetFileInformationByHandle(h, windows.FileBasicInfo, (*byte)(unsafe.Pointer(&ib)), uint32(unsafe.Sizeof(ib)))
	}
	return false
}

// tassoExe: il Tasso.exe che sta girando (tenuto bloccato da lockSelfExe), per --cancella-sicura.
func tassoExe() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	return p
}

// profiloVero: la cartella dell'utente scritta da Windows in HKLM (ProfileList), che un programma
// senza permessi non può cambiare. Le cartelle "note" (AppData, Documenti...) invece si possono
// redirigere da HKCU.
func profiloVero() string {
	tok := windows.GetCurrentProcessToken()
	u, err := tok.GetTokenUser()
	if err != nil || u == nil || u.User.Sid == nil {
		return ""
	}
	sid := u.User.Sid.String()
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList\`+sid, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue("ProfileImagePath")
	if err != nil {
		return ""
	}
	if e, err := registry.ExpandString(v); err == nil {
		v = e
	}
	if q, ok := percorsoLocale(v); ok && len(q) > 3 {
		return q
	}
	return ""
}

// steamVero: la cartella di Steam scritta dall'installatore in HKLM (con steam.exe dentro), "" se no.
func steamVero() string {
	for _, p := range []string{`SOFTWARE\WOW6432Node\Valve\Steam`, `SOFTWARE\Valve\Steam`} {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, p, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		v, _, err := k.GetStringValue("InstallPath")
		k.Close()
		if err != nil {
			continue
		}
		if q, ok := percorsoLocale(strings.ReplaceAll(v, "/", `\`)); ok && len(q) > 3 && fileExists(filepath.Join(q, "steam.exe")) {
			return q
		}
	}
	return ""
}

// elencoAmmessi: dove la cancellazione sicura può lavorare (vedi cancella_ammessi.go). Ogni cartella
// vale anche con il suo percorso vero (C:\Users spostato su D:, Steam su un altro disco).
func elencoAmmessi() cartelleAmmesse {
	c := cartelleVere
	var a cartelleAmmesse
	add := func(lista *[]string, p string) {
		if p == "" {
			return
		}
		*lista = append(*lista, p)
		if f := finaleDi(p); f != "" && !strings.EqualFold(f, strings.TrimRight(p, `\`)) {
			*lista = append(*lista, f)
		}
	}
	// AppData del profilo VERO. Quelle "note" solo se stanno davvero lì dentro.
	if prof := profiloVero(); prof != "" {
		local, roaming := filepath.Join(prof, "AppData", "Local"), filepath.Join(prof, "AppData", "Roaming")
		add(&a.dentro, local)
		add(&a.dentro, roaming)
		add(&a.intere, filepath.Join(local, "Temp"))
		for _, k := range []string{c.Local, c.Roaming} {
			if k != "" && dentroDi(k, prof) {
				add(&a.dentro, k)
			}
		}
	}
	if c.TempOriginale != "" { // già controllata da tempOriginale (nome temp/tmp, niente cartelle personali)
		add(&a.intere, c.TempOriginale)
	}
	if w := c.Win; w != "" {
		for _, s := range []string{"Temp", `SoftwareDistribution\Download`, "Minidump", "LiveKernelReports",
			`ServiceProfiles\LocalService\AppData\Local\Temp`, `ServiceProfiles\NetworkService\AppData\Local\Temp`} {
			add(&a.intere, filepath.Join(w, s))
		}
		if sd := filepath.VolumeName(w); sd != "" {
			for _, s := range []string{"AMD", "NVIDIA", `Intel\Logs`, `Intel\ExtremeGraphics`, "EFSTMPWP"} {
				add(&a.intere, filepath.Join(sd+`\`, s))
			}
		}
	}
	if pd := c.PD; pd != "" {
		for _, s := range []string{`Microsoft\Windows\WER`, `NVIDIA Corporation\NV_Cache`, `NVIDIA Corporation\Downloader`, `GOG.com\Galaxy\webcache`} {
			add(&a.intere, filepath.Join(pd, s))
		}
	}
	for _, pf := range []string{c.PF, c.PF86} {
		if pf != "" {
			add(&a.intere, filepath.Join(pf, `Ubisoft\Ubisoft Game Launcher\cache`))
		}
	}
	if st := steamVero(); st != "" {
		for _, s := range []string{"dumps", "logs", `appcache\httpcache`} {
			add(&a.intere, filepath.Join(st, s))
		}
	}
	// I temporanei di Tasso (Programmi\Tasso\lavoro): le esportazioni Wi-Fi e simili.
	add(&a.intere, filepath.Join(cartellaTasso(), "lavoro"))
	add(&a.mai, filepath.Join(cartellaTasso(), "lavoro", "copie-giochi")) // traduce/copie_protette.go
	add(&a.mai, filepath.Join(cartellaTasso(), "lavoro", "mod"))
	if c.TempTasso != "" {
		add(&a.intere, c.TempTasso)
	}
	return a
}
