package main

// Tasso senza lo scudo sull'icona (3.1.1). Il manifest di Tasso.exe non chiede più l'amministratore
// (asInvoker): così Windows non mette lo scudo sull'icona. Tasso però ha bisogno dei permessi da
// amministratore, quindi all'apertura:
//   - se è già amministratore va avanti come sempre;
//   - se no avvia l'attività pianificata \Tasso\Apri Tasso <SID dell'utente> (la prepara Tasso stesso, o
//     l'installatore con Tasso.exe --prepara-apertura), che lo riapre da amministratore senza
//     chiedere niente, e si chiude;
//   - se l'attività non c'è o non parte, chiede i permessi come prima (la finestra di Windows
//     "Vuoi consentire a questa app di apportare modifiche?").
// La logica senza Windows (XML dell'attività, argomenti permessi) è in apertura/.

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"tasso/apertura"
)

// argPreparaApertura: Tasso.exe --prepara-apertura (lo usa l'installatore, da amministratore):
// prepara l'attività \Tasso\Apri Tasso ed esce (0 fatto, 1 no). Niente finestra.
const argPreparaApertura = "--prepara-apertura"

// segnoPreparaApertura: l'installatore avvia Tasso con --prepara-apertura solo se questa scritta è
// dentro Tasso.exe (un Tasso che non conosce l'argomento si aprirebbe normalmente).
const segnoPreparaApertura = "tasso-prepara-apertura-v1"

// attesaAttivita: quanto aspetto che la copia avviata dall'attività dica "sono partita".
const attesaAttivita = 15 * time.Second

var (
	pAllowSetFg = user32.NewProc("AllowSetForegroundWindow")
	pCoUninit   = ole32.NewProc("CoUninitialize")

	clsidTaskScheduler = windows.GUID{Data1: 0x0F87369F, Data2: 0xA4E5, Data3: 0x4CFC, Data4: [8]byte{0xBD, 0x3E, 0x73, 0xE6, 0x15, 0x45, 0x72, 0xDD}}
	iidITaskService    = windows.GUID{Data1: 0x2FABA4C7, Data2: 0x4DA9, Data3: 0x4013, Data4: [8]byte{0x96, 0x97, 0x20, 0xCC, 0x3F, 0xD4, 0x0F, 0x85}}
)

// daAttivita: questa copia è partita dall'attività (per portare la finestra davanti).
var daAttivita bool

// elevato: Tasso gira da amministratore (con UAC: dopo il "Sì", o dall'attività).
func elevato() bool { return windows.GetCurrentProcessToken().IsElevated() }

// elevazioneDivisa: utente amministratore con UAC acceso (due token: normale e da amministratore).
// Solo in questo caso l'attività serve: con UAC spento, o con l'account Administrator, Tasso parte
// già da amministratore; un utente normale dall'attività non avrebbe comunque i permessi.
func elevazioneDivisa() bool {
	var tipo, n uint32
	err := windows.GetTokenInformation(windows.GetCurrentProcessToken(), windows.TokenElevationType,
		(*byte)(unsafe.Pointer(&tipo)), uint32(unsafe.Sizeof(tipo)), &n)
	return err == nil && tipo == 2 // TokenElevationTypeFull
}

// sidUtente: l'utente di questo processo (lo stesso con e senza permessi).
func sidUtente() string {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return ""
	}
	return u.User.Sid.String()
}

// argomentiDaAttivita: da chiamare per prima in main (dopo hardenProcess). La copia partita
// dall'attività avvisa chi l'ha avviata e tiene SOLO gli argomenti permessi: l'attività la può
// avviare qualunque programma dell'utente, con un parametro a piacere.
func argomentiDaAttivita() {
	da, nonce, resto := apertura.DaAttivita(os.Args[1:])
	if !da {
		return
	}
	daAttivita = true
	os.Args = append([]string{os.Args[0]}, resto...)
	segnalaPartita(nonce)
}

// apriConPermessi: da chiamare in main prima della copia unica. true = questa copia deve uscire
// subito (Tasso è stato riaperto da amministratore, o l'utente ha detto no a Windows).
func apriConPermessi() bool {
	if elevato() {
		return false
	}
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return false // non so chi sono: vado avanti come prima (meglio aperto che niente)
	}
	exe = filepath.Clean(exe)
	args := os.Args[1:]
	// pulizia programmata o Profilo gioco: le loro attività partono già da amministratore. Senza
	// permessi qualcosa non va: esco in silenzio, niente richiesta di Windows all'improvviso.
	if apertura.Silenziosi(args) {
		return true
	}
	// la copia partita dall'attività non è da amministratore (per esempio utente normale): non
	// riprovo con l'attività (girerei in tondo), chiedo i permessi a Windows
	if !daAttivita && apertura.TuttiPermessi(args) && avviaAttivita(exe, args) {
		return true
	}
	chiediPermessi(exe, args)
	return true
}

// chiediPermessi: come prima della 3.1.1, la finestra di Windows per i permessi. Se l'utente dice
// no, Tasso non si apre (come prima: col manifest requireAdministrator non partiva proprio).
func chiediPermessi(exe string, args []string) {
	verbo, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	cart, _ := windows.UTF16PtrFromString(filepath.Dir(exe))
	var par *uint16
	if len(args) > 0 {
		par, _ = windows.UTF16PtrFromString(apertura.RigaComando(args))
	}
	_ = windows.ShellExecute(0, verbo, file, par, cart, windows.SW_SHOWNORMAL)
}

func nomeSegnale(nonce string) string { return `Local\TassoAperto-` + nonce }

// segnalaPartita: dico alla copia senza permessi che mi ha avviato che sono partita.
func segnalaPartita(nonce string) {
	if !apertura.NonceValido(nonce) {
		return
	}
	n, _ := windows.UTF16PtrFromString(nomeSegnale(nonce))
	h, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, n)
	if err != nil {
		return
	}
	_ = windows.SetEvent(h)
	windows.CloseHandle(h)
}

// avviaAttivita: avvia \Tasso\Apri Tasso (solo se apre proprio questo Tasso.exe, per questo utente)
// e aspetta che la copia nuova sia partita. false = non c'è, non è partita o non so: chiedo i permessi.
func avviaAttivita(exe string, args []string) bool {
	sid := sidUtente()
	if !apertura.SidValido(sid) {
		return false
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return false
	}
	nonce := hex.EncodeToString(b[:])
	n, _ := windows.UTF16PtrFromString(nomeSegnale(nonce))
	ev, err := windows.CreateEvent(nil, 1, 0, n)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(ev)
	// La copia nuova parte dall'Utilità di pianificazione, non da un clic: le lascio portare
	// la sua finestra davanti (se no si aprirebbe dietro le altre).
	pAllowSetFg.Call(uintptr(0xFFFFFFFF)) // ASFW_ANY
	partita := false
	conPianificatore(func(s comObj) {
		t := prendiAttivita(s, sid)
		if t == 0 {
			return
		}
		defer t.release()
		a, err := leggiAttivitaWin(t)
		if err != nil || !a.Usabile(exe, sid) {
			return
		}
		partita = eseguiAttivita(t, apertura.ParametroAttivita(nonce, args))
	})
	if !partita {
		return false
	}
	r, _ := windows.WaitForSingleObject(ev, uint32(attesaAttivita/time.Millisecond))
	return r == windows.WAIT_OBJECT_0
}

// leggiAttivitaWin: l'attività letta da Windows, con l'utente sempre come SID (Windows lo può
// restituire anche come PC\nome).
func leggiAttivitaWin(t comObj) (apertura.Attivita, error) {
	a, err := apertura.LeggiAttivita(xmlAttivita(t))
	if err == nil && a.Utente != "" && !strings.HasPrefix(strings.ToUpper(a.Utente), "S-1-") {
		if sid, _, _, e := windows.LookupSID("", a.Utente); e == nil && sid != nil {
			a.Utente = sid.String()
		}
	}
	return a, err
}

// ---------------- Utilità di pianificazione (COM, ITaskService) ----------------
// Tabelle delle funzioni (dopo IUnknown 0-2 e IDispatch 3-6):
//   ITaskService:    7 GetFolder, 10 Connect
//   ITaskFolder:     9 GetFolder, 11 CreateFolder, 13 GetTask, 16 RegisterTask, 18 GetSecurityDescriptor
//   IRegisteredTask: 12 Run, 20 get_Xml

// metodo: l'indirizzo della funzione numero idx dell'oggetto COM o.
func metodo(o comObj, idx int) uintptr {
	vtbl := *(*uintptr)(puntatore(uintptr(o)))
	return *(*uintptr)(puntatore(vtbl + uintptr(idx)*unsafe.Sizeof(uintptr(0))))
}

func nuovaBSTR(s string) uintptr {
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		return 0
	}
	b, _, _ := pSysAllocString.Call(uintptr(unsafe.Pointer(p)))
	return b
}

func liberaBSTR(b uintptr) {
	if b != 0 {
		pSysFreeString.Call(b)
	}
}

// conPianificatore: f con ITaskService collegato, su un filo con COM (modello MTA) tutto suo.
func conPianificatore(f func(s comObj)) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := pCoInitializeEx.Call(0, 0) // COINIT_MULTITHREADED
	if hrOK(hr) {
		defer pCoUninit.Call()
	} else if uint32(hr) != 0x80010106 { // RPC_E_CHANGED_MODE: COM c'è già (STA), va bene lo stesso
		return
	}
	var s comObj
	r, _, _ := pCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidTaskScheduler)), 0, 1, // CLSCTX_INPROC_SERVER
		uintptr(unsafe.Pointer(&iidITaskService)), uintptr(unsafe.Pointer(&s)))
	if !hrOK(r) || s == 0 {
		return
	}
	defer s.release()
	var vuoto variant // VT_EMPTY: computer, utente, dominio e password di questa sessione
	r, _, _ = syscall.SyscallN(metodo(s, 10), uintptr(s), uintptr(unsafe.Pointer(&vuoto)), uintptr(unsafe.Pointer(&vuoto)),
		uintptr(unsafe.Pointer(&vuoto)), uintptr(unsafe.Pointer(&vuoto)))
	if !hrOK(r) {
		return
	}
	f(s)
}

// prendiCartella: ITaskFolder di percorso (0 se non c'è).
func prendiCartella(s comObj, percorso string) comObj {
	b := nuovaBSTR(percorso)
	if b == 0 {
		return 0
	}
	defer liberaBSTR(b)
	var c comObj
	r, _, _ := syscall.SyscallN(metodo(s, 7), uintptr(s), b, uintptr(unsafe.Pointer(&c)))
	if !hrOK(r) {
		return 0
	}
	return c
}

// prendiAttivita: IRegisteredTask di \Tasso\Apri Tasso <sid> (0 se non c'è o non la posso vedere).
func prendiAttivita(s comObj, sid string) comObj {
	c := prendiCartella(s, apertura.CartellaAttivita)
	if c == 0 {
		return 0
	}
	defer c.release()
	b := nuovaBSTR(apertura.NomeAttivitaPer(sid))
	if b == 0 {
		return 0
	}
	defer liberaBSTR(b)
	var t comObj
	r, _, _ := syscall.SyscallN(metodo(c, 13), uintptr(c), b, uintptr(unsafe.Pointer(&t)))
	if !hrOK(r) {
		return 0
	}
	return t
}

// xmlAttivita: la descrizione dell'attività (IRegisteredTask.Xml).
func xmlAttivita(t comObj) string {
	var b uintptr
	r, _, _ := syscall.SyscallN(metodo(t, 20), uintptr(t), uintptr(unsafe.Pointer(&b)))
	if !hrOK(r) {
		return ""
	}
	defer liberaBSTR(b)
	return bstrToString(b)
}

// eseguiAttivita: IRegisteredTask.Run con il parametro $(Arg0).
func eseguiAttivita(t comObj, parametro string) bool {
	b := nuovaBSTR(parametro)
	if b == 0 {
		return false
	}
	defer liberaBSTR(b)
	v := variant{vt: 8, val: b} // VT_BSTR
	var in comObj
	r, _, _ := syscall.SyscallN(metodo(t, 12), uintptr(t), uintptr(unsafe.Pointer(&v)), uintptr(unsafe.Pointer(&in)))
	in.release()
	return hrOK(r)
}

// proprietarioAdmin: il proprietario della cartella \Tasso dell'Utilità di pianificazione è
// Amministratori, SYSTEM o TrustedInstaller (non un programma dell'utente che l'ha creata prima di
// Tasso con permessi suoi).
func proprietarioAdmin(c comObj) bool {
	var b uintptr
	r, _, _ := syscall.SyscallN(metodo(c, 18), uintptr(c), 1, uintptr(unsafe.Pointer(&b))) // GetSecurityDescriptor(OWNER)
	if !hrOK(r) {
		return false
	}
	testo := bstrToString(b)
	liberaBSTR(b)
	sd, err := windows.SecurityDescriptorFromString(testo)
	if err != nil {
		return false
	}
	o, _, err := sd.Owner()
	if err != nil || o == nil {
		return false
	}
	return o.IsWellKnown(windows.WinBuiltinAdministratorsSid) || o.IsWellKnown(windows.WinLocalSystemSid) ||
		o.String() == "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464" // TrustedInstaller
}

// registraAttivita: crea o rifà \Tasso\Apri Tasso <sid> (Tasso è da amministratore), con i
// permessi di apertura.SDDLAttivita. "" = fatto, altrimenti il motivo (per il registro).
func registraAttivita(s comObj, xml, sid string) string {
	c := prendiCartella(s, apertura.CartellaAttivita)
	if c == 0 {
		radice := prendiCartella(s, `\`)
		if radice == 0 {
			return "cartella principale non aperta"
		}
		b := nuovaBSTR(strings.TrimPrefix(apertura.CartellaAttivita, `\`))
		var vuoto variant
		r, _, _ := syscall.SyscallN(metodo(radice, 11), uintptr(radice), b, uintptr(unsafe.Pointer(&vuoto)), uintptr(unsafe.Pointer(&c)))
		liberaBSTR(b)
		radice.release()
		if !hrOK(r) || c == 0 {
			// creata nel frattempo da un'altra copia di Tasso?
			if c = prendiCartella(s, apertura.CartellaAttivita); c == 0 {
				return fmt.Sprintf("cartella \\Tasso non creata (%#x)", uint32(r))
			}
		}
	}
	defer c.release()
	if !proprietarioAdmin(c) {
		return "la cartella \\Tasso non è degli amministratori"
	}
	nome, testo := nuovaBSTR(apertura.NomeAttivitaPer(sid)), nuovaBSTR(xml)
	utente, sddl := nuovaBSTR(sid), nuovaBSTR(apertura.SDDLAttivita(sid))
	defer liberaBSTR(nome)
	defer liberaBSTR(testo)
	defer liberaBSTR(utente)
	defer liberaBSTR(sddl)
	if nome == 0 || testo == 0 || utente == 0 || sddl == 0 {
		return "memoria"
	}
	vUtente := variant{vt: 8, val: utente} // VT_BSTR: il SID dell'utente
	vSddl := variant{vt: 8, val: sddl}
	var vuoto variant // password: nessuna (InteractiveToken)
	var t comObj
	// 0x16 = TASK_CREATE_OR_UPDATE + TASK_DONT_ADD_PRINCIPAL_ACE (valgono solo i permessi di sddl);
	// 3 = TASK_LOGON_INTERACTIVE_TOKEN (solo con l'utente collegato, nessuna password salvata)
	r, _, _ := syscall.SyscallN(metodo(c, 16), uintptr(c), nome, testo, 0x16,
		uintptr(unsafe.Pointer(&vUtente)), uintptr(unsafe.Pointer(&vuoto)), 3, uintptr(unsafe.Pointer(&vSddl)), uintptr(unsafe.Pointer(&t)))
	t.release()
	if !hrOK(r) {
		return fmt.Sprintf("RegisterTask %#x", uint32(r))
	}
	return ""
}

// copiaInstallata: exe è il Tasso.exe installato dentro Programmi (Programmi\Tasso o la cartella
// dell'installatore dentro Programmi, vedi cartellaTasso: la stessa regola delle altre attività,
// taskExe in settings.go), dove scrivono solo gli amministratori. Solo questo può aprirsi
// dall'attività: un Tasso.exe in Download, o in una cartella scelta fuori da Programmi (magari su
// un disco senza permessi), lo potrebbe cambiare qualunque programma.
func copiaInstallata(exe string) bool {
	exe = filepath.Clean(exe)
	if strings.ContainsAny(exe, "\"%\r\n") || programFilesDir() == "" {
		return false
	}
	dir := cartellaTasso()
	dentro := false
	for _, pf := range []string{programFilesDir(), cartellaNota(windows.FOLDERID_ProgramFilesX86)} {
		dentro = dentro || pf != "" && dentroDi(dir, pf) && !strings.EqualFold(strings.TrimRight(dir, `\`), strings.TrimRight(pf, `\`))
	}
	return dentro && strings.EqualFold(exe, filepath.Join(dir, "Tasso.exe"))
}

// utenteDiverso: Tasso da amministratore gira con un altro account rispetto a chi è collegato
// (UAC con la password di un altro amministratore): l'attività sarebbe per quell'account, non per
// chi usa il PC. Guardo il proprietario di explorer.exe di questa sessione, come l'installatore.
// Se non lo so (niente Esplora file) non è diverso.
func utenteDiverso(sid string) bool {
	var sessione uint32
	if windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &sessione) != nil {
		return false
	}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	visto := false
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		if !strings.EqualFold(windows.UTF16ToString(pe.ExeFile[:]), "explorer.exe") {
			continue
		}
		var ss uint32
		if windows.ProcessIdToSessionId(pe.ProcessID, &ss) != nil || ss != sessione {
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pe.ProcessID)
		if err != nil {
			continue
		}
		var tok windows.Token
		err = windows.OpenProcessToken(h, windows.TOKEN_QUERY, &tok)
		windows.CloseHandle(h)
		if err != nil {
			continue
		}
		u, err := tok.GetTokenUser()
		tok.Close()
		if err != nil {
			continue
		}
		if strings.EqualFold(u.User.Sid.String(), sid) {
			return false // lo stesso utente
		}
		visto = true
	}
	return visto
}

// preparaAttivitaApri: crea o rifà \Tasso\Apri Tasso <sid> se serve (manca, è vecchia, è
// disattivata, è di un altro exe). Da amministratore, solo per il Tasso installato in Programmi,
// solo con UAC acceso e solo per l'utente collegato. "" = l'attività c'è ed è a posto, altrimenti
// il motivo.
func preparaAttivitaApri() string {
	exe, err := os.Executable()
	switch {
	case err != nil || exe == "":
		return "exe sconosciuto"
	case !elevato():
		return "senza permessi"
	case !elevazioneDivisa():
		return "UAC spento o account senza doppio token: non serve"
	}
	exe = filepath.Clean(exe)
	sid := sidUtente()
	switch {
	case !apertura.SidValido(sid):
		return "utente sconosciuto"
	case !copiaInstallata(exe):
		return "non è il Tasso installato in Programmi"
	case utenteDiverso(sid):
		return "Tasso gira con un altro account"
	}
	motivo := "Utilità di pianificazione non raggiungibile"
	conPianificatore(func(s comObj) {
		if t := prendiAttivita(s, sid); t != 0 {
			a, err := leggiAttivitaWin(t)
			t.release()
			if err == nil && a.Aggiornata(exe, sid) {
				motivo = ""
				return
			}
		}
		motivo = registraAttivita(s, apertura.XMLAttivita(exe, sid), sid)
	})
	return motivo
}

// preparaAperturaDaRiga: Tasso.exe --prepara-apertura (l'installatore). Scrive il segno, poi
// "APERTURA-OK" o "APERTURA-NO <motivo>".
func preparaAperturaDaRiga() int {
	fmt.Println(segnoPreparaApertura)
	if m := preparaAttivitaApri(); m != "" {
		fmt.Println("APERTURA-NO " + m)
		return 1
	}
	fmt.Println("APERTURA-OK")
	return 0
}
