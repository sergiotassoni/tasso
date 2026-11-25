package main

// Le cose di Windows: registro vero, collegamenti (.lnk con COM), cartelle note,
// programmi aperti, scelta della cartella, spazio libero, togliere il disinstallatore aperto.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"tassosetup/installa"
)

// ---------------- registro ----------------

type registroWin struct{}

func radice(r installa.Radice) registry.Key {
	if r == installa.HKCU {
		return registry.CURRENT_USER
	}
	return registry.LOCAL_MACHINE
}

func (registroWin) ScriviTesto(r installa.Radice, chiave, nome, valore string) error {
	k, _, err := registry.CreateKey(radice(r), chiave, registry.SET_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(nome, valore)
}

func (registroWin) ScriviNumero(r installa.Radice, chiave, nome string, valore uint32) error {
	k, _, err := registry.CreateKey(radice(r), chiave, registry.SET_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetDWordValue(nome, valore)
}

func (registroWin) LeggiTesto(r installa.Radice, chiave, nome string) (string, bool) {
	k, err := registry.OpenKey(radice(r), chiave, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return "", false
	}
	defer k.Close()
	s, _, err := k.GetStringValue(nome)
	return s, err == nil
}

func (registroWin) TogliValore(r installa.Radice, chiave, nome string) error {
	k, err := registry.OpenKey(radice(r), chiave, registry.SET_VALUE|registry.WOW64_64KEY)
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

func (registroWin) TogliChiave(r installa.Radice, chiave string) error {
	err := registry.DeleteKey(radice(r), chiave)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}

func (registroWin) TogliAlbero(r installa.Radice, chiave string) error {
	k, err := registry.OpenKey(radice(r), chiave, registry.ENUMERATE_SUB_KEYS|registry.WOW64_64KEY)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	figli, _ := k.ReadSubKeyNames(-1)
	k.Close()
	for _, f := range figli {
		if err := (registroWin{}).TogliAlbero(r, chiave+`\`+f); err != nil {
			return err
		}
	}
	return (registroWin{}).TogliChiave(r, chiave)
}

func (registroWin) Vuota(r installa.Radice, chiave string) bool {
	k, err := registry.OpenKey(radice(r), chiave, registry.QUERY_VALUE|registry.ENUMERATE_SUB_KEYS|registry.WOW64_64KEY)
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
		buf := make([]uint16, 1024)
		sz := uint32(len(buf))
		if windows.QueryFullProcessImageName(h, 0, &buf[0], &sz) == nil {
			if nome, ok := cerca[strings.ToLower(windows.UTF16ToString(buf[:sz]))]; ok {
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
func chiudiProgrammi(base string, cc []installa.Componente) bool {
	var lista []aperto
	for _, a := range programmiAperti(base, cc) {
		if a.nome == "Tasso" {
			switch chiudiTassoBene(filepath.Join(base, "Tasso.exe"), a.pid) {
			case tassoChiuso:
				continue
			case tassoRifiuta:
				return false
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
	time.Sleep(300 * time.Millisecond) // Windows rilascia i file un attimo dopo
	return ok && len(programmiAperti(base, cc)) == 0
}

// ---------------- avvio a fine installazione ----------------

// apri avvia un programma installato. Tasso chiede comunque l'amministratore (suo manifest):
// lo avvio direttamente. Zip e PDF li avvio tramite Esplora file, così partono come utente
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
