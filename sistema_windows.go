package main

// Le cose di Windows: registro vero, collegamenti (.lnk con COM), cartelle note,
// programmi aperti, scelta della cartella, spazio libero, togliere il disinstallatore aperto.

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"tassosetup/installa"
)

// ---------------- registro ----------------

// registroWin: il registro di Windows. hkcu = la radice da usare per HKCU (0 = quello di chi fa
// girare l'installatore; HKEY_USERS\<SID> per l'utente della sessione, vedi utenteSessione).
type registroWin struct{ hkcu registry.Key }

func (g registroWin) radice(r installa.Radice) registry.Key {
	if r == installa.HKCU {
		if g.hkcu != 0 {
			return g.hkcu
		}
		return registry.CURRENT_USER
	}
	return registry.LOCAL_MACHINE
}

func (g registroWin) ScriviTesto(r installa.Radice, chiave, nome, valore string) error {
	k, _, err := registry.CreateKey(g.radice(r), chiave, registry.SET_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(nome, valore)
}

func (g registroWin) ScriviNumero(r installa.Radice, chiave, nome string, valore uint32) error {
	k, _, err := registry.CreateKey(g.radice(r), chiave, registry.SET_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetDWordValue(nome, valore)
}

func (g registroWin) LeggiTesto(r installa.Radice, chiave, nome string) (string, bool) {
	k, err := registry.OpenKey(g.radice(r), chiave, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return "", false
	}
	defer k.Close()
	s, _, err := k.GetStringValue(nome)
	return s, err == nil
}

func (g registroWin) TogliValore(r installa.Radice, chiave, nome string) error {
	k, err := registry.OpenKey(g.radice(r), chiave, registry.SET_VALUE|registry.WOW64_64KEY)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(nome); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

func (g registroWin) TogliChiave(r installa.Radice, chiave string) error {
	err := registry.DeleteKey(g.radice(r), chiave)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}

func (g registroWin) TogliAlbero(r installa.Radice, chiave string) error {
	k, err := registry.OpenKey(g.radice(r), chiave, registry.ENUMERATE_SUB_KEYS|registry.WOW64_64KEY)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	figli, _ := k.ReadSubKeyNames(-1)
	k.Close()
	for _, f := range figli {
		if err := g.TogliAlbero(r, chiave+`\`+f); err != nil {
			return err
		}
	}
	return g.TogliChiave(r, chiave)
}

func (g registroWin) Vuota(r installa.Radice, chiave string) bool {
	k, err := registry.OpenKey(g.radice(r), chiave, registry.QUERY_VALUE|registry.ENUMERATE_SUB_KEYS|registry.WOW64_64KEY)
	if err != nil {
		return true
	}
	defer k.Close()
	st, err := k.Stat()
	return err == nil && st.SubKeyCount == 0 && st.ValueCount == 0
}

// ---------------- cartelle di Windows ----------------

// datiLocaliUtente: %LOCALAPPDATA% dell'utente (dalla cartella nota, non dalla variabile).
func datiLocaliUtente() string { return cartellaNota(windows.FOLDERID_LocalAppData, "") }

func cartellaNota(id *windows.KNOWNFOLDERID, riserva string) string {
	if p, err := windows.KnownFolderPath(id, 0); err == nil && p != "" {
		return p
	}
	return riserva
}

// Mai le variabili d'ambiente: l'installatore gira da amministratore e un programma
// qualsiasi dell'utente può cambiarle.
func programFiles() string    { return cartellaNota(windows.FOLDERID_ProgramFiles, `C:\Program Files`) }
func cartellaWindows() string { return cartellaNota(windows.FOLDERID_Windows, `C:\Windows`) }
func menuStart() string       { return cartellaNota(windows.FOLDERID_CommonPrograms, "") }
func desktopPubblico() string { return cartellaNota(windows.FOLDERID_PublicDesktop, "") }
func sistema32() string {
	if p, err := windows.GetSystemDirectory(); err == nil && p != "" {
		return p
	}
	return filepath.Join(cartellaWindows(), "System32")
}

var (
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procGetUserDefaultUILang = kernel32.NewProc("GetUserDefaultUILanguage")
	shell32                  = windows.NewLazySystemDLL("shell32.dll")
	procSHBrowseForFolder    = shell32.NewProc("SHBrowseForFolderW")
	procSHGetPathFromIDList  = shell32.NewProc("SHGetPathFromIDListW")
	procSHChangeNotify       = shell32.NewProc("SHChangeNotify")
	ole32                    = windows.NewLazySystemDLL("ole32.dll")
	procCoCreateInstance     = ole32.NewProc("CoCreateInstance")
	procCoTaskMemFree        = ole32.NewProc("CoTaskMemFree")
	user32                   = windows.NewLazySystemDLL("user32.dll")
	procSendMessageW         = user32.NewProc("SendMessageW")
	procSendMessageTimeoutW  = user32.NewProc("SendMessageTimeoutW")
	procGetDpiForSystem      = user32.NewProc("GetDpiForSystem")
	procSetForegroundWindow  = user32.NewProc("SetForegroundWindow")
	procFindWindowW          = user32.NewProc("FindWindowW")
)

func linguaWindows() uint16 {
	r, _, _ := procGetUserDefaultUILang.Call()
	return uint16(r)
}

// spazioLibero: byte liberi sul disco della cartella (che può ancora non esistere).
func spazioLibero(p string) int64 {
	for d := p; d != ""; {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			ptr, err := windows.UTF16PtrFromString(d)
			if err != nil {
				return 0
			}
			var libero, tot, totLibero uint64
			if windows.GetDiskFreeSpaceEx(ptr, &libero, &tot, &totLibero) != nil {
				return 0
			}
			return int64(libero)
		}
		su := filepath.Dir(d)
		if su == d {
			break
		}
		d = su
	}
	return 0
}

// ---------------- scelta della cartella ----------------

type browseInfo struct {
	hwndOwner      uintptr
	pidlRoot       uintptr
	pszDisplayName *uint16
	lpszTitle      *uint16
	ulFlags        uint32
	lpfn           uintptr
	lParam         uintptr
	iImage         int32
}

var callbackSfoglia = windows.NewCallback(func(hwnd, msg, lp, data uintptr) uintptr {
	const bffmInitialized, bffmSetSelectionW = 1, 0x0400 + 103
	if msg == bffmInitialized && data != 0 {
		procSendMessageW.Call(hwnd, bffmSetSelectionW, 1, data)
	}
	return 0
})

// scegliCartella mostra la finestra di Windows per scegliere una cartella.
// Se la cartella scelta non si chiama "Tasso", aggiunge \Tasso (come fanno gli installatori).
func scegliCartella(owner uintptr, attuale, titolo string) string {
	nome := make([]uint16, windows.MAX_PATH)
	tit, _ := windows.UTF16PtrFromString(titolo)
	var iniz *uint16
	for d := attuale; d != ""; d = filepath.Dir(d) {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			iniz, _ = windows.UTF16PtrFromString(d)
			break
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	bi := browseInfo{hwndOwner: owner, pszDisplayName: &nome[0], lpszTitle: tit,
		ulFlags: 0x0001 | 0x0040 | 0x0010, // BIF_RETURNONLYFSDIRS | BIF_NEWDIALOGSTYLE | BIF_EDITBOX
		lpfn:    callbackSfoglia, lParam: uintptr(unsafe.Pointer(iniz))}
	pidl, _, _ := procSHBrowseForFolder.Call(uintptr(unsafe.Pointer(&bi)))
	runtime.KeepAlive(iniz)
	if pidl == 0 {
		return ""
	}
	defer procCoTaskMemFree.Call(pidl)
	buf := make([]uint16, 1024)
	if r, _, _ := procSHGetPathFromIDList.Call(pidl, uintptr(unsafe.Pointer(&buf[0]))); r == 0 {
		return ""
	}
	p := windows.UTF16ToString(buf)
	if !strings.EqualFold(filepath.Base(p), installa.CartellaTassoStd) {
		p = filepath.Join(p, installa.CartellaTassoStd)
	}
	return p
}

// ---------------- collegamenti (.lnk) ----------------

var (
	clsidShellLink  = windows.GUID{Data1: 0x00021401, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIShellLinkW  = windows.GUID{Data1: 0x000214F9, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIPersistFile = windows.GUID{Data1: 0x0000010B, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
)

type comObj struct{ vtbl *[32]uintptr }

func chiama(o *comObj, i int, a ...uintptr) error {
	r, _, _ := syscall.SyscallN(o.vtbl[i], append([]uintptr{uintptr(unsafe.Pointer(o))}, a...)...)
	if int32(r) < 0 {
		return fmt.Errorf("COM errore 0x%08X", uint32(r))
	}
	return nil
}

func u16(s string) uintptr {
	p, _ := windows.UTF16PtrFromString(s)
	return uintptr(unsafe.Pointer(p))
}

// creaCollegamento scrive un .lnk con IShellLinkW + IPersistFile (come fa Esplora file).
func creaCollegamento(c installa.Collegamento) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err == nil {
		defer windows.CoUninitialize()
	}
	if err := os.MkdirAll(filepath.Dir(c.Percorso), 0o755); err != nil {
		return err
	}
	var sl *comObj
	r, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidShellLink)), 0, 1, /*CLSCTX_INPROC_SERVER*/
		uintptr(unsafe.Pointer(&iidIShellLinkW)), uintptr(unsafe.Pointer(&sl)))
	if int32(r) < 0 || sl == nil {
		return fmt.Errorf("ShellLink non disponibile (0x%08X)", uint32(r))
	}
	defer chiama(sl, 2) // Release
	// vtable di IShellLinkW: 7 SetDescription, 9 SetWorkingDirectory, 17 SetIconLocation, 20 SetPath
	if err := chiama(sl, 20, u16(c.Destinazione)); err != nil {
		return err
	}
	_ = chiama(sl, 9, u16(c.Cartella))
	_ = chiama(sl, 7, u16(c.Descrizione))
	_ = chiama(sl, 17, u16(c.Destinazione), 0)
	var pf *comObj
	if err := chiama(sl, 0, uintptr(unsafe.Pointer(&iidIPersistFile)), uintptr(unsafe.Pointer(&pf))); err != nil || pf == nil {
		return fmt.Errorf("IPersistFile: %v", err)
	}
	defer chiama(pf, 2)
	return chiama(pf, 6, u16(c.Percorso), 1) // Save(percorso, TRUE)
}

// aggiornaIcone dice a Esplora file di rileggere icone e collegamenti.
func aggiornaIcone() {
	procSHChangeNotify.Call(0x08000000 /*SHCNE_ASSOCCHANGED*/, 0, 0, 0)
}

// ---------------- programmi aperti ----------------

type aperto struct {
	pid  uint32
	nome string
}

// exeDaControllare: gli exe che l'installazione sostituirebbe, con il nome da mostrare.
func exeDaControllare(base string, cc []installa.Componente) map[string]string {
	out := map[string]string{}
	for _, p := range installa.ProgrammiPer(cc) {
		switch p {
		case installa.ProgTasso:
			out[strings.ToLower(filepath.Join(base, "Tasso.exe"))] = "Tasso"
		case installa.ProgZip:
			out[strings.ToLower(filepath.Join(installa.CartellaDi(base, p), "TassoZip.exe"))] = "Tasso Zip"
		case installa.ProgPDF:
			out[strings.ToLower(filepath.Join(installa.CartellaDi(base, p), "TassoPDF.exe"))] = "Tasso PDF"
		}
	}
	return out
}

func programmiAperti(base string, cc []installa.Componente) []aperto {
	cerca := exeDaControllare(base, cc)
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	var out []aperto
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		n := strings.ToLower(windows.UTF16ToString(pe.ExeFile[:]))
		if n != "tasso.exe" && n != "tassozip.exe" && n != "tassopdf.exe" {
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pe.ProcessID)
		if err != nil {
			continue
		}
		if exe, ok := percorsoExe(h); ok {
			if nome, ok := cerca[strings.ToLower(exe)]; ok {
				out = append(out, aperto{pe.ProcessID, nome})
			}
		}
		windows.CloseHandle(h)
	}
	return out
}

// Esito di "Tasso.exe --chiudi".
const (
	tassoChiuso    = iota // si è chiuso bene (ha salvato)
	tassoRifiuta          // sta lavorando (pulizia, aggiornamento): non va chiuso a forza
	tassoNonCapito        // Tasso vecchio, senza --chiudi, o non partito: si chiude come gli altri
)

// chiudiTassoBene chiede al Tasso aperto di chiudersi da solo: "Tasso.exe --chiudi" esce con 0 se
// Tasso si è chiuso (dopo aver salvato), con 1 se rifiuta perché sta lavorando o non si chiude
// entro 20 secondi. Un Tasso vecchio non conosce --chiudi: porta davanti la sua finestra ed esce
// con 0 senza chiudere niente; allora resta aperto e lo chiudo come gli altri programmi.
func chiudiTassoBene(exe string, pid uint32) int {
	ctx, annulla := context.WithTimeout(context.Background(), 30*time.Second)
	defer annulla()
	cmd := exec.CommandContext(ctx, exe, "--chiudi")
	cmd.Dir = filepath.Dir(exe)
	err := cmd.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &ee) && ee.ExitCode() == 1 && ctx.Err() == nil:
		if ancoraAperto(pid) {
			return tassoRifiuta
		}
		return tassoChiuso
	default:
		return tassoNonCapito
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return tassoChiuso // non c'è più
	}
	defer windows.CloseHandle(h)
	if ev, _ := windows.WaitForSingleObject(h, 5000); ev == windows.WAIT_OBJECT_0 {
		return tassoChiuso
	}
	return tassoNonCapito
}

// ancoraAperto: il processo pid gira ancora.
func ancoraAperto(pid uint32) bool {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	ev, _ := windows.WaitForSingleObject(h, 0)
	return ev == uint32(windows.WAIT_TIMEOUT)
}

// chiudiProgrammi: Tasso prima con "Tasso.exe --chiudi" (si chiude bene da solo, e se sta facendo
// una pulizia o un aggiornamento rifiuta: allora non lo chiudo a forza e restituisco false).
// Agli altri (e a un Tasso vecchio) prima dice che "Windows si sta spegnendo"
// (WM_QUERYENDSESSION/WM_ENDSESSION: Tasso salva impostazioni e memoria), poi li chiude.
//
// Dopo, chiude anche quello che Tasso aveva avviato e che tiene occupati i file (la sua WebView2,
// llama-server, whisper-server, PresentMon): vedi chiudiAiutanti.
func chiudiProgrammi(base string, cc []installa.Componente) bool {
	aperti := programmiAperti(base, cc)
	// la fotografia PRIMA di chiudere: dopo, i figli di Tasso non si riconoscono più dal padre
	prima := fotoProcessi()
	var radici []uint32
	for _, a := range aperti {
		radici = append(radici, a.pid)
	}
	installa.Diario("chiudo i programmi aperti: %v", aperti)
	var lista []aperto
	for _, a := range aperti {
		if a.nome == "Tasso" {
			switch esito := chiudiTassoBene(filepath.Join(base, "Tasso.exe"), a.pid); esito {
			case tassoChiuso:
				installa.Diario("Tasso (pid %d) si è chiuso da solo con --chiudi", a.pid)
				continue
			case tassoRifiuta:
				installa.Diario("Tasso (pid %d) rifiuta di chiudersi: sta lavorando", a.pid)
				return false
			default:
				installa.Diario("Tasso (pid %d) non ha capito --chiudi: lo chiudo come gli altri", a.pid)
			}
		}
		lista = append(lista, a)
	}
	for _, a := range lista {
		pid := a.pid
		cb := windows.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
			var p uint32
			_, _ = windows.GetWindowThreadProcessId(windows.HWND(hwnd), &p)
			if p == pid {
				var res uintptr
				procSendMessageTimeoutW.Call(hwnd, 0x0011, 0, 0x80000000 /*ENDSESSION_LOGOFF*/, 0x0002 /*SMTO_ABORTIFHUNG*/, 2000, uintptr(unsafe.Pointer(&res)))
				procSendMessageTimeoutW.Call(hwnd, 0x0016, 1, 0x80000000, 0x0002, 3000, uintptr(unsafe.Pointer(&res)))
			}
			return 1
		})
		_ = windows.EnumWindows(cb, nil)
	}
	ok := true
	for _, a := range lista {
		h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, a.pid)
		if err != nil {
			continue // già chiuso
		}
		if ev, _ := windows.WaitForSingleObject(h, 1500); ev != windows.WAIT_OBJECT_0 {
			_ = windows.TerminateProcess(h, 0)
			if ev, _ := windows.WaitForSingleObject(h, 8000); ev != windows.WAIT_OBJECT_0 {
				ok = false
			}
		}
		windows.CloseHandle(h)
	}
	chiudiAiutanti(base, cc, radici, prima)
	time.Sleep(300 * time.Millisecond) // Windows rilascia i file un attimo dopo
	resta := programmiAperti(base, cc)
	if !ok || len(resta) > 0 {
		installa.Diario("non si sono chiusi: %v", resta)
	}
	return ok && len(resta) == 0
}

// ---------------- processi avviati da Tasso ----------------

// fotoProcessi: tutti i processi, con padre, percorso dell'exe e ora di avvio.
func fotoProcessi() []installa.Processo {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		installa.Diario("fotografia dei processi non riuscita: %v", err)
		return nil
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	var out []installa.Processo
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		p := installa.Processo{PID: pe.ProcessID, Padre: pe.ParentProcessID, Nome: windows.UTF16ToString(pe.ExeFile[:])}
		if h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pe.ProcessID); err == nil {
			if exe, ok := percorsoExe(h); ok {
				p.Percorso = exe
			}
			p.Nato = natoIl(h)
			windows.CloseHandle(h)
		}
		out = append(out, p)
	}
	return out
}

// percorsoExe: il percorso completo dell'exe del processo h.
func percorsoExe(h windows.Handle) (string, bool) {
	buf := make([]uint16, 1024)
	sz := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &sz) != nil {
		return "", false
	}
	return windows.UTF16ToString(buf[:sz]), true
}

// natoIl: ora di avvio del processo (0 se non si legge).
func natoIl(h windows.Handle) int64 {
	var c, e, k, u windows.Filetime
	if windows.GetProcessTimes(h, &c, &e, &k, &u) != nil {
		return 0
	}
	return c.Nanoseconds()
}

// criteriChiusura: i file dei programmi che sto per sostituire (elenco installato e nuovo) e le
// cartelle solo di Tasso (installa.CriteriPer). Mai tutto quello che parte dalla cartella.
func criteriChiusura(base string, cc []installa.Componente, radici []uint32) installa.CriteriChiusura {
	st, _ := installa.LeggiStato(base)
	aiutanti := ""
	if d := datiLocaliUtente(); d != "" {
		aiutanti = filepath.Join(d, "TassoTraduce")
	}
	return installa.CriteriPer(base, cc, st, elenco, radici, uint32(os.Getpid()), aiutanti)
}

// chiudiAiutanti chiude i processi che tengono occupati i file da sostituire: i figli dei
// programmi appena chiusi (radici, dalla fotografia fatta prima di chiuderli) e
// qualsiasi processo partito dalle cartelle da sostituire. Prima aspetta un po' che escano da
// soli (la WebView2 di Tasso esce subito dopo Tasso), poi li chiude a forza.
// Mai un processo che non c'entra (vedi installa.DaChiudere). Se uno non si chiude non fermo
// niente: copiando, l'installatore riprova e al limite sposta il file accanto.
func chiudiAiutanti(base string, cc []installa.Componente, radici []uint32, prima []installa.Processo) {
	cr := criteriChiusura(base, cc, radici)
	visti := map[uint32]bool{}
	var tutti []installa.Processo
	if len(radici) > 0 {
		for _, p := range installa.DaChiudere(prima, cr) {
			visti[p.PID] = true
			tutti = append(tutti, p)
		}
	}
	cr.Radici = nil
	for _, p := range installa.DaChiudere(fotoProcessi(), cr) {
		if !visti[p.PID] {
			tutti = append(tutti, p)
		}
	}
	if len(tutti) == 0 {
		return
	}
	fine := time.Now().Add(5 * time.Second) // il tempo di uscire da soli, per tutti insieme
	for _, p := range tutti {
		h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, p.PID)
		if err != nil {
			continue // già uscito
		}
		if p.Nato != 0 && natoIl(h) != p.Nato {
			windows.CloseHandle(h) // PID riusato da un altro programma: non è lui
			continue
		}
		attesa := uint32(0)
		if d := time.Until(fine); d > 0 {
			attesa = uint32(d / time.Millisecond)
		}
		if ev, _ := windows.WaitForSingleObject(h, attesa); ev == windows.WAIT_OBJECT_0 {
			installa.Diario("%s (pid %d) uscito da solo", p.Nome, p.PID)
		} else {
			err := windows.TerminateProcess(h, 1)
			ev, _ := windows.WaitForSingleObject(h, 5000)
			installa.Diario("%s (pid %d, %s) chiuso a forza: errore=%v, uscito=%v", p.Nome, p.PID, p.Percorso, err, ev == windows.WAIT_OBJECT_0)
		}
		windows.CloseHandle(h)
	}
}

// cancellaAlRiavvio: Windows cancella il file (o la cartella vuota) al prossimo riavvio.
func cancellaAlRiavvio(p string) error {
	pp, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(pp, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
}

// ---------------- avvio a fine installazione ----------------

// apri avvia un programma installato. Tasso lo avvio direttamente: eredita i permessi da
// amministratore dell'installatore (dalla 3.1.1 il suo manifest è asInvoker, vedi preparaAperturaTasso). Zip e PDF li avvio tramite Esplora file, così partono come utente
// normale e non da amministratore (trascina e rilascia, finestre di dialogo, file dell'utente).
func apri(exe string, comeUtente bool) {
	if comeUtente {
		cmd := exec.Command(filepath.Join(cartellaWindows(), "explorer.exe"), exe)
		_ = cmd.Start()
		return
	}
	cmd := exec.Command(exe)
	cmd.Dir = filepath.Dir(exe)
	_ = cmd.Start()
}

// ---------------- il disinstallatore toglie se stesso ----------------

var daCancellareAllaFine string

// togliFile: come os.Remove, ma il disinstallatore in esecuzione non si può cancellare:
// lo rinomino (Windows lo permette) fuori dalla cartella, e lo cancello appena esco.
func togliFile(p string) error {
	me, _ := os.Executable()
	if me == "" || !strings.EqualFold(filepath.Clean(p), filepath.Clean(me)) {
		return os.Remove(p)
	}
	altro := filepath.Join(filepath.VolumeName(me)+`\`, "tasso-disinstalla-"+strconv.Itoa(os.Getpid())+".tmp")
	if strings.EqualFold(filepath.VolumeName(me), filepath.VolumeName(programFiles())) {
		altro = filepath.Join(programFiles(), "tasso-disinstalla-"+strconv.Itoa(os.Getpid())+".tmp")
	}
	if err := os.Rename(me, altro); err != nil {
		altro = me // non si sposta: lo toglie Windows al prossimo riavvio
	}
	daCancellareAllaFine = altro
	if pp, err := windows.UTF16PtrFromString(altro); err == nil {
		_ = windows.MoveFileEx(pp, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
	}
	return nil
}

// cancellaAllUscita: un cmd nascosto aspetta che il disinstallatore sia chiuso e lo cancella.
func cancellaAllUscita() {
	if daCancellareAllaFine == "" {
		return
	}
	cmd := exec.Command(filepath.Join(sistema32(), "cmd.exe"))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000,
		CmdLine: `cmd.exe /d /c ""` + filepath.Join(sistema32(), "ping.exe") + `" -n 4 127.0.0.1 >nul & del /f /q "` + daCancellareAllaFine + `""`}
	_ = cmd.Start()
}

// sddlTasso: i permessi di Programmi per la cartella di Tasso fuori da Programmi. Proprietario
// Amministratori; SYSTEM e Amministratori tutto; gli utenti solo leggere ed eseguire; niente
// ereditato dal disco (dove spesso "Utenti autenticati" possono modificare). Tasso.exe e il
// disinstallatore partono da amministratore: nessun programma normale deve poterli sostituire.
const sddlTasso = "O:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;0x1200a9;;;BU)"

// sidTrustedInstaller: NT SERVICE\TrustedInstaller (proprietario di molte cartelle di Windows).
const sidTrustedInstaller = "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"

// proprietarioAdmin: il proprietario di p è Amministratori, SYSTEM o TrustedInstaller.
func proprietarioAdmin(p string) bool {
	sd, err := windows.GetNamedSecurityInfo(p, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return false
	}
	o, _, err := sd.Owner()
	if err != nil || o == nil {
		return false
	}
	return o.IsWellKnown(windows.WinBuiltinAdministratorsSid) || o.IsWellKnown(windows.WinLocalSystemSid) ||
		o.String() == sidTrustedInstaller
}

// antenatiAdmin: le cartelle sopra base che esistono, tranne la radice del disco, sono degli
// amministratori. Se una è di un utente, lui potrebbe spostare via la cartella di Tasso e metterne
// una sua al suo posto (con un Tasso.exe suo, che poi partirebbe da amministratore).
func antenatiAdmin(base string) bool {
	radice := filepath.VolumeName(base) + `\`
	for d := filepath.Dir(base); len(d) > len(radice) && !strings.EqualFold(d, radice); d = filepath.Dir(d) {
		if _, err := os.Lstat(d); err != nil {
			continue // la creo io (proprietario Amministratori)
		}
		if !proprietarioAdmin(d) {
			installa.Diario("la cartella %s non è degli amministratori", d)
			return false
		}
	}
	return true
}

// sicurezzaCartella legge da Windows quello che serve a installa.DecidiCartellaSicura.
func sicurezzaCartella(base string) installa.SicurezzaCartella {
	s := installa.SicurezzaCartella{SenzaPermessi: discoSenzaPermessi(base)}
	if !s.SenzaPermessi {
		s.AntenatiAdmin = antenatiAdmin(base)
	}
	fi, err := os.Lstat(base)
	if err != nil {
		return s
	}
	s.Esiste = true
	s.Collegamento = !fi.IsDir() || fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0
	if a, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok && a.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		s.Collegamento = true
	}
	if voci, err := os.ReadDir(base); err == nil {
		s.Vuota = len(voci) == 0
	}
	if _, err := os.Lstat(filepath.Join(base, installa.FileStato)); err == nil {
		s.DiTasso = true
	}
	s.ProprietarioAdmin = proprietarioAdmin(base)
	return s
}

// discoSenzaPermessi: il disco di p non conserva proprietari e permessi (FAT32, exFAT: manca
// FILE_PERSISTENT_ACLS). Se non si riesce a leggere, si fa come per NTFS (controlli pieni).
func discoSenzaPermessi(p string) bool {
	vol := filepath.VolumeName(p)
	if vol == "" {
		return false
	}
	radice, err := windows.UTF16PtrFromString(vol + `\`)
	if err != nil {
		return false
	}
	var flag uint32
	if err := windows.GetVolumeInformation(radice, nil, 0, nil, nil, &flag, nil, 0); err != nil {
		return false
	}
	return flag&windows.FILE_PERSISTENT_ACLS == 0
}

// preparaCartellaTasso: la cartella base. Dentro Programmi: una cartella normale (ha già i
// permessi giusti). Fuori: nuova, nasce già protetta (CreateDirectory con sddlTasso: nemmeno un
// istante con i permessi del disco); già esistente, si usa solo se è sicura
// (installa.DecidiCartellaSicura) e le si rimettono proprietario e permessi, anche ai file dentro.
// Un disco senza permessi (FAT32, exFAT) non li supporta: lo scrivo nel diario e vado avanti.
func preparaCartellaTasso(base string) (bool, error) {
	if installa.DentroProgrammi(base, programFiles()) {
		_, err := os.Stat(base)
		creata := os.IsNotExist(err)
		return creata, os.MkdirAll(base, 0o755)
	}
	s := sicurezzaCartella(base)
	crea, err := installa.DecidiCartellaSicura(s)
	if err != nil {
		if s.DiTasso && !s.Collegamento {
			// installazione già fuori da Programmi dentro una cartella di un utente (versioni di
			// prima): l'aggiornamento non si blocca, lo scrivo e basta
			installa.Diario("attenzione: %s è dentro una cartella che un utente può cambiare", base)
		} else {
			return false, err
		}
	}
	sd, err := windows.SecurityDescriptorFromString(sddlTasso)
	if err != nil {
		return false, err
	}
	if crea {
		if err := os.MkdirAll(filepath.Dir(base), 0o755); err != nil {
			return false, err
		}
		p, err := windows.UTF16PtrFromString(base)
		if err != nil {
			return false, err
		}
		sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
		if err := windows.CreateDirectory(p, sa); err != nil {
			return false, err
		}
		installa.Diario("cartella %s creata con i permessi di Programmi", base)
		return true, nil
	}
	if err := proteggiAlbero(base, sd, s.DiTasso); err != nil {
		if s.DiTasso {
			// installazione che c'è già: un aggiornamento non si blocca per i permessi (com'era
			// prima di questa versione), lo scrivo e vado avanti
			installa.Diario("permessi di %s non rimessi, aggiorno lo stesso: %v", base, err)
			return false, nil
		}
		return false, err
	}
	return false, nil
}

// proteggiAlbero: proprietario Amministratori e permessi di sddlTasso alla cartella; con dentro=true
// anche a ogni file e cartella dentro (proprietario Amministratori, solo permessi ereditati), così
// nessun file messo lì da un utente resta suo.
func proteggiAlbero(base string, sd *windows.SECURITY_DESCRIPTOR, dentro bool) error {
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	err = windows.SetNamedSecurityInfo(base, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		owner, nil, dacl, nil)
	if errors.Is(err, windows.ERROR_NOT_SUPPORTED) || errors.Is(err, windows.ERROR_INVALID_FUNCTION) {
		installa.Diario("il disco di %s non ha permessi (FAT32/exFAT?): la cartella resta com'è", base)
		return nil
	}
	if err != nil {
		return err
	}
	installa.Diario("permessi di %s: solo gli amministratori possono modificare", base)
	if !dentro {
		return nil
	}
	vuota, err := windows.ACLFromEntries(nil, nil)
	if err != nil {
		return err
	}
	return filepath.WalkDir(base, func(p string, d fs.DirEntry, e error) error {
		if e != nil || p == base {
			return nil
		}
		if d.Type()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			if d.IsDir() {
				return fs.SkipDir // collegamenti e giunzioni: non li seguo
			}
			return nil
		}
		if err := windows.SetNamedSecurityInfo(p, windows.SE_FILE_OBJECT,
			windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.UNPROTECTED_DACL_SECURITY_INFORMATION,
			owner, nil, vuota, nil); err != nil {
			installa.Diario("permessi di %s non rimessi: %v", p, err)
		}
		return nil
	})
}

// ---------------- Tasso toglie le sue cose (disinstallazione da App e funzionalità) ----------------

// segnoTogliSilenzioso: c'è dentro Tasso.exe solo se Tasso conosce "--togli-silenzioso"
// (uninstall.go di Tasso). Un Tasso vecchio, avviato con un argomento che non conosce, si
// aprirebbe normalmente: allora non lo avvio e faccio solo la mia parte.
const segnoTogliSilenzioso = "tasso-togli-silenzioso-v1"

// pulisciTasso: "Tasso.exe --togli-silenzioso" toglie attività pianificate, Boost, modalità
// dell'Ufficio e traduzioni nei giochi, senza finestra. Scrive "TOGLI-OK" se è andato tutto bene,
// "TOGLI-NO <motivo>" (ed esce con 1) se adesso non si può (un gioco tradotto aperto...): allora
// la disinstallazione si ferma prima di togliere qualsiasi cosa. Se Tasso si blocca o va in errore
// vado avanti con la mia parte (attività, cartelle), che è comunque fatta.
func pulisciTasso(base string) error {
	exe := filepath.Join(base, "Tasso.exe")
	b, err := os.ReadFile(exe)
	if err != nil {
		installa.Diario("pulizia di Tasso saltata: %v", err)
		return nil
	}
	if !bytes.Contains(b, []byte(segnoTogliSilenzioso)) {
		installa.Diario("questo Tasso.exe non conosce --togli-silenzioso: tolgo solo attività e cartelle")
		return nil
	}
	// 20 minuti: Tasso copia in Documenti gli originali dei giochi, che possono essere grandi
	ctx, annulla := context.WithTimeout(context.Background(), 20*time.Minute)
	defer annulla()
	cmd := exec.CommandContext(ctx, exe, "--togli-silenzioso")
	cmd.Dir = base
	// leggo riga per riga: con "TOGLI-PASSO n" la finestra dice cosa sta facendo Tasso (R2)
	var tutto strings.Builder
	pipe, err := cmd.StdoutPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err == nil {
		sc := bufio.NewScanner(pipe)
		for sc.Scan() {
			riga := sc.Text()
			tutto.WriteString(riga + "\n")
			if n, ok := installa.PassoTogli(riga); ok {
				manda("passoTasso", n)
			}
		}
		err = cmd.Wait()
	}
	testo := tutto.String()
	installa.Diario("Tasso --togli-silenzioso: %v %q", err, testo)
	// Senza TOGLI-OK (errore, blocco, tempo scaduto) vado avanti con la mia parte: gli originali dei
	// giochi rimasti in lavoro\copie-giochi li mette al sicuro DisinstallaCon (SalvaCopieGiochi),
	// prima di cancellare qualsiasi cosa, e se non ci riesce si ferma.
	_, rifiuto := installa.EsitoTogliSilenzioso(testo, err)
	return rifiuto
}

// meteCopieGiochi: Documenti\Tasso - copie di sicurezza (copiando: i file prendono i permessi di
// Documenti), poi %ProgramData%\Tasso - copie di sicurezza (stesso disco di Programmi, il
// disinstallatore non la cancella). Mai le variabili d'ambiente.
func meteCopieGiochi() []installa.Meta {
	var m []installa.Meta
	if d := cartellaNota(windows.FOLDERID_Documents, ""); d != "" {
		m = append(m, installa.Meta{Cartella: filepath.Join(d, installa.NomeCopieSicurezza), Copia: true})
	}
	if pd := cartellaNota(windows.FOLDERID_ProgramData, ""); pd != "" {
		m = append(m, installa.Meta{Cartella: filepath.Join(pd, installa.NomeCopieSicurezza)})
	}
	return m
}

// ---------------- Tasso senza scudo sull'icona: l'attività \Tasso\Apri Tasso ----------------

// segnoPreparaApertura: c'è dentro Tasso.exe solo se Tasso conosce "--prepara-apertura"
// (apri_admin.go di Tasso 3.1.1). Un Tasso vecchio si aprirebbe normalmente: allora non lo avvio.
const segnoPreparaApertura = "tasso-prepara-apertura-v1"

// preparaAperturaTasso: dalla 3.1.1 Tasso.exe non chiede l'amministratore nel suo manifest (così
// Windows non mette lo scudo sull'icona) e si riapre da amministratore con l'attività pianificata
// \Tasso\Apri Tasso <SID> (una per utente), senza la richiesta di Windows a ogni apertura. L'attività la prepara Tasso
// stesso (Tasso.exe --prepara-apertura, da amministratore come l'installatore): così anche la
// prima apertura dopo l'installazione non chiede niente. Se non riesce non è un errore: Tasso la
// prepara alla prima apertura (dopo aver chiesto i permessi una volta). Con UAC e la password di un
// altro account non la preparo: sarebbe per quell'account, non per chi usa il PC.
func preparaAperturaTasso(base string, cc []installa.Componente) {
	tasso := false
	for _, p := range installa.ProgrammiPer(cc) {
		tasso = tasso || p == installa.ProgTasso
	}
	if !tasso {
		return
	}
	if conUtenteSessione(&installa.Installatore{}); utenteReg != nil {
		installa.Diario("attività Apri Tasso non preparata: l'installatore gira con un altro account")
		return
	}
	exe := filepath.Join(base, "Tasso.exe")
	b, err := os.ReadFile(exe)
	if err != nil || !bytes.Contains(b, []byte(segnoPreparaApertura)) {
		installa.Diario("questo Tasso.exe non conosce --prepara-apertura (%v)", err)
		return
	}
	ctx, annulla := context.WithTimeout(context.Background(), time.Minute)
	defer annulla()
	cmd := exec.CommandContext(ctx, exe, "--prepara-apertura")
	cmd.Dir = base
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.Output()
	installa.Diario("Tasso --prepara-apertura: %v %q", err, strings.TrimSpace(string(out)))
}

// togliAttivitaTasso: via la cartella \Tasso\ dell'Utilità di pianificazione con tutte le sue
// attività (Avvio con Windows, Profilo gioco, Pulizia settimanale, Apri Tasso <SID> di ogni utente).
func togliAttivitaTasso() error {
	ps := filepath.Join(sistema32(), "WindowsPowerShell", "v1.0", "powershell.exe")
	script := `$ErrorActionPreference='SilentlyContinue'
Get-ScheduledTask -TaskPath '\Tasso\' | Unregister-ScheduledTask -Confirm:$false
try { $s = New-Object -ComObject Schedule.Service; $s.Connect(); $s.GetFolder('\').DeleteFolder('Tasso', 0) } catch {}
if (Get-ScheduledTask -TaskPath '\Tasso\') { exit 1 }
exit 0`
	ctx, annulla := context.WithTimeout(context.Background(), 2*time.Minute)
	defer annulla()
	cmd := exec.CommandContext(ctx, ps, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.Dir = sistema32()
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd.Run()
}

// ---------------- utente della sessione (UAC con un altro account) ----------------

// utenteSessione: se l'utente collegato a questa sessione (quello di explorer.exe) non è chi fa
// girare l'installatore (UAC con la password di un altro account), restituisce il registro con il
// suo HKCU (HKEY_USERS\<SID>) e la sua %LOCALAPPDATA%. Tasso Zip e Tasso PDF girano come lui.
// ok = false se è lo stesso utente (il caso normale) o se non si riesce a saperlo.
func utenteSessione() (installa.Registro, func() string, bool) {
	var sessione uint32
	if windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &sessione) != nil {
		return nil, nil, false
	}
	io, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, nil, false
	}
	mioSID := io.User.Sid.String()
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, nil, false
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		if !strings.EqualFold(windows.UTF16ToString(pe.ExeFile[:]), "explorer.exe") {
			continue
		}
		var s uint32
		if windows.ProcessIdToSessionId(pe.ProcessID, &s) != nil || s != sessione {
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pe.ProcessID)
		if err != nil {
			continue
		}
		var tok windows.Token
		err = windows.OpenProcessToken(h, windows.TOKEN_QUERY|windows.TOKEN_IMPERSONATE|windows.TOKEN_DUPLICATE, &tok)
		windows.CloseHandle(h)
		if err != nil {
			continue
		}
		u, err := tok.GetTokenUser()
		if err != nil {
			tok.Close()
			continue
		}
		sid := u.User.Sid.String()
		if sid == mioSID {
			tok.Close()
			return nil, nil, false // stesso utente: va bene HKCU normale
		}
		k, err := registry.OpenKey(registry.USERS, sid, registry.ALL_ACCESS|registry.WOW64_64KEY)
		if err != nil {
			tok.Close()
			installa.Diario("HKCU dell'utente della sessione (%s) non aperto: %v", sid, err)
			return nil, nil, false
		}
		dati, _ := tok.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
		tok.Close()
		installa.Diario("UAC con un altro account: tracce di Zip e PDF nell'utente della sessione %s (%s)", sid, dati)
		return registroWin{hkcu: k}, func() string { return dati }, true
	}
	return nil, nil, false
}

var (
	utenteUna    sync.Once
	utenteReg    installa.Registro
	utenteCartel func() string
)

// conUtenteSessione aggiunge all'installatore l'utente della sessione, se è diverso.
func conUtenteSessione(in *installa.Installatore) *installa.Installatore {
	utenteUna.Do(func() {
		if r, d, ok := utenteSessione(); ok {
			utenteReg, utenteCartel = r, d
		}
	})
	if utenteReg != nil {
		in.RegistroUtente, in.CartellaDatiSessione = utenteReg, utenteCartel
	}
	return in
}

// ---------------- cancellazioni al riavvio rimaste da una disinstallazione ----------------

// annullaCancellazioniPendenti: dopo un'installazione riuscita in base, toglie da
// PendingFileRenameOperations le cancellazioni al riavvio dei file di Tasso dentro base (messe da
// una disinstallazione fatta prima di riavviare): altrimenti al riavvio Windows cancellerebbe i
// file nuovi con lo stesso nome. Solo quelle dei programmi appena installati (cc): reinstallando
// solo Tasso, i file di Tasso Zip e Tasso PDF tolti prima si cancellano lo stesso. Vedi
// installa.TogliCancellazioniPendenti. Se qualcosa non va non tocca niente.
func annullaCancellazioniPendenti(base string, cc []installa.Componente) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager`,
		registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	voci, tipo, err := k.GetStringsValue("PendingFileRenameOperations")
	if err != nil || tipo != registry.MULTI_SZ || len(voci) == 0 {
		return
	}
	// riscrivo solo se rileggendo ottengo esattamente gli stessi byte (niente pezzi persi)
	dim := 2 // il doppio zero finale
	for _, v := range voci {
		dim += 2 * (len(utf16.Encode([]rune(v))) + 1)
	}
	if n, _, err := k.GetValue("PendingFileRenameOperations", nil); err != nil || n != dim {
		installa.Diario("PendingFileRenameOperations in un formato che non conosco: non lo tocco")
		return
	}
	resto, tolte := installa.TogliCancellazioniPendenti(voci, base, append(installa.ProgrammiPer(cc), installa.ProgSetup))
	if tolte == 0 {
		return
	}
	if len(resto) == 0 {
		err = k.DeleteValue("PendingFileRenameOperations")
	} else {
		err = k.SetStringsValue("PendingFileRenameOperations", resto)
	}
	installa.Diario("cancellazioni al riavvio dentro %s annullate: %d (errore: %v)", base, tolte, err)
}

// ---------------- collegamento della 3.0 portatile sul Desktop ----------------

// leggiCollegamento: destinazione, argomenti e descrizione di un .lnk.
func leggiCollegamento(p string) (dest, arg, desc string, err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if e := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); e == nil {
		defer windows.CoUninitialize()
	}
	var sl *comObj
	r, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidShellLink)), 0, 1, /*CLSCTX_INPROC_SERVER*/
		uintptr(unsafe.Pointer(&iidIShellLinkW)), uintptr(unsafe.Pointer(&sl)))
	if int32(r) < 0 || sl == nil {
		return "", "", "", fmt.Errorf("ShellLink non disponibile (0x%08X)", uint32(r))
	}
	defer chiama(sl, 2)
	var pf *comObj
	if err := chiama(sl, 0, uintptr(unsafe.Pointer(&iidIPersistFile)), uintptr(unsafe.Pointer(&pf))); err != nil || pf == nil {
		return "", "", "", fmt.Errorf("IPersistFile: %v", err)
	}
	defer chiama(pf, 2)
	if err := chiama(pf, 5, u16(p), 0 /*STGM_READ*/); err != nil { // Load
		return "", "", "", err
	}
	buf := make([]uint16, 1024)
	// vtable di IShellLinkW: 3 GetPath, 6 GetDescription, 10 GetArguments
	if err := chiama(sl, 3, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0, 0); err != nil {
		return "", "", "", err
	}
	dest = windows.UTF16ToString(buf)
	for i := range buf {
		buf[i] = 0
	}
	if err := chiama(sl, 10, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); err != nil {
		return "", "", "", err
	}
	arg = windows.UTF16ToString(buf)
	for i := range buf {
		buf[i] = 0
	}
	if err := chiama(sl, 6, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); err != nil {
		return "", "", "", err
	}
	desc = windows.UTF16ToString(buf)
	return dest, arg, desc, nil
}

// aggiornaCollegamentoVecchio: se sul Desktop dell'utente c'è il Tasso.lnk della 3.0 portatile
// (installa.CollegamentoVecchioDaAggiornare), lo faccio puntare al Tasso installato in base,
// così non apre più la 3.0 scaricata. In ogni altro caso (o se non si legge) non tocco niente.
func aggiornaCollegamentoVecchio(base string) {
	lnk := installa.PercorsoCollegamentoVecchio(cartellaNota(windows.FOLDERID_Desktop, ""))
	if lnk == "" {
		return
	}
	if fi, err := os.Lstat(lnk); err != nil || !fi.Mode().IsRegular() {
		return
	}
	dest, arg, desc, err := leggiCollegamento(lnk)
	if err != nil {
		installa.Diario("%s non letto: non lo tocco (%v)", lnk, err)
		return
	}
	if !installa.CollegamentoVecchioDaAggiornare(dest, arg, desc, programFiles()) {
		return
	}
	if x86 := cartellaNota(windows.FOLDERID_ProgramFilesX86, ""); x86 != "" && installa.Dentro(dest, x86) {
		return // anche Programmi (x86) è una cartella di installazioni: non è la 3.0 portatile
	}
	nuovo := filepath.Join(base, "Tasso.exe")
	if fi, err := os.Lstat(nuovo); err != nil || !fi.Mode().IsRegular() {
		return
	}
	err = creaCollegamento(installa.Collegamento{Percorso: lnk, Destinazione: nuovo, Cartella: base,
		Descrizione: desc, Programma: installa.ProgTasso})
	installa.Diario("%s portava a %s (Tasso 3.0 portatile): ora porta a %s (errore: %v)", lnk, dest, nuovo, err)
}

// togliCollegamentoVecchio: tolto Tasso, via anche il Desktop\Tasso.lnk della 3.0 che
// aggiornaCollegamentoVecchio aveva fatto puntare al Tasso di base (solo se porta proprio a
// base\Tasso.exe, vedi installa.CollegamentoVecchioNostro). In ogni altro caso non tocco niente.
func togliCollegamentoVecchio(base string) {
	lnk := installa.PercorsoCollegamentoVecchio(cartellaNota(windows.FOLDERID_Desktop, ""))
	if lnk == "" {
		return
	}
	if fi, err := os.Lstat(lnk); err != nil || !fi.Mode().IsRegular() {
		return
	}
	dest, arg, _, err := leggiCollegamento(lnk)
	if err != nil || !installa.CollegamentoVecchioNostro(dest, arg, base) {
		return
	}
	err = togliFile(lnk)
	installa.Diario("%s portava a %s (Tasso tolto): via (errore: %v)", lnk, dest, err)
}

// ---------------- cartella di lavoro dell'installatore ----------------

// cartellaLavoro: %ProgramData%\TassoSetup con i permessi di Programmi (sddlTasso), per il diario
// e i dati della WebView2. L'installatore parte da amministratore: in %TEMP% dell'utente un altro
// programma potrebbe cambiare quei file mentre li uso. "" se non si può: allora uso %TEMP% come prima.
func cartellaLavoro() string {
	pd := cartellaNota(windows.FOLDERID_ProgramData, "")
	if pd == "" {
		return ""
	}
	base := filepath.Join(pd, installa.NomeCartellaLavoro)
	sd, err := windows.SecurityDescriptorFromString(sddlTasso)
	if err != nil {
		return ""
	}
	fi, err := os.Lstat(base)
	if err != nil && !os.IsNotExist(err) {
		return ""
	}
	esiste := err == nil
	cartella, coll, admin, protetta := false, false, false, false
	if esiste {
		cartella = fi.IsDir()
		coll = fi.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0 || collegamentoWin(base)
		admin = proprietarioAdmin(base)
		protetta = daclProtetto(base)
	}
	switch installa.DecidiCartellaLavoro(esiste, cartella, coll, admin, protetta) {
	case installa.LavoroUsa:
		// dentro=false: i file dentro li ho messi io (gli utenti non possono scrivere lì)
		if proteggiAlbero(base, sd, false) != nil {
			return ""
		}
		return base
	case installa.LavoroSpostaVia:
		// messa lì da qualcun altro: la sposto via (una giunzione si sposta, non si segue)
		if os.Rename(base, base+".vecchia-"+strconv.FormatInt(time.Now().UnixNano(), 36)) != nil {
			return ""
		}
	}
	p, err := windows.UTF16PtrFromString(base)
	if err != nil {
		return ""
	}
	sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	if windows.CreateDirectory(p, sa) != nil {
		return ""
	}
	return base
}

// daclProtetto: i permessi di p non ereditano da sopra (SE_DACL_PROTECTED), come quelli che metto io
// con sddlTasso. Una cartella con i permessi ereditati da ProgramData la rifaccio da capo.
func daclProtetto(p string) bool {
	sd, err := windows.GetNamedSecurityInfo(p, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return false
	}
	c, _, err := sd.Control()
	return err == nil && c&windows.SE_DACL_PROTECTED != 0
}

// collegamentoWin: p è un reparse point (collegamento simbolico, giunzione...).
func collegamentoWin(p string) bool {
	u, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return true
	}
	a, err := windows.GetFileAttributes(u)
	if err != nil {
		return true
	}
	return a&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}

// togliDatiWebView2Vecchi: le cartelle della WebView2 rimaste da installazioni chiuse male.
func togliDatiWebView2Vecchi(base string) {
	if base == "" {
		return
	}
	voci, err := os.ReadDir(base)
	if err != nil {
		return
	}
	for _, v := range voci {
		if !v.IsDir() || v.Type()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			continue
		}
		if info, err := v.Info(); err == nil && installa.DatiWebView2Vecchi(v.Name(), info.ModTime(), time.Now()) {
			_ = os.RemoveAll(filepath.Join(base, v.Name()))
		}
	}
}
