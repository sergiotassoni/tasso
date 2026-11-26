package main

// Tasso senza lo scudo sull'icona (3.1.1). Il manifest di Tasso.exe non chiede più l'amministratore
// (asInvoker): così Windows non mette lo scudo sull'icona. Tasso però ha bisogno dei permessi da
// amministratore, quindi all'apertura:
//   - se è già amministratore va avanti come sempre;
//   - se no avvia l'attività pianificata \Tasso\Apri Tasso (la prepara Tasso stesso, o
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
	if sid == "" {
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
		t := prendiAttivita(s)
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
//   ITaskFolder:     9 GetFolder, 11 CreateFolder, 13 GetTask, 16 RegisterTask
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

// prendiAttivita: IRegisteredTask di \Tasso\Apri Tasso (0 se non c'è o non la posso vedere).
func prendiAttivita(s comObj) comObj {
	c := prendiCartella(s, apertura.CartellaAttivita)
	if c == 0 {
		return 0
	}
	defer c.release()
	b := nuovaBSTR(apertura.NomeAttivita)
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

// registraAttivita: crea o rifà \Tasso\Apri Tasso (Tasso è da amministratore).
func registraAttivita(s comObj, xml string) bool {
	c := prendiCartella(s, apertura.CartellaAttivita)
	if c == 0 {
		radice := prendiCartella(s, `\`)
		if radice == 0 {
			return false
		}
		b := nuovaBSTR(strings.TrimPrefix(apertura.CartellaAttivita, `\`))
		var vuoto variant
		r, _, _ := syscall.SyscallN(metodo(radice, 11), uintptr(radice), b, uintptr(unsafe.Pointer(&vuoto)), uintptr(unsafe.Pointer(&c)))
		liberaBSTR(b)
		radice.release()
		if !hrOK(r) || c == 0 {
			// creata nel frattempo da un'altra copia di Tasso?
			if c = prendiCartella(s, apertura.CartellaAttivita); c == 0 {
				return false
			}
		}
	}
	defer c.release()
	nome, testo := nuovaBSTR(apertura.NomeAttivita), nuovaBSTR(xml)
	defer liberaBSTR(nome)
	defer liberaBSTR(testo)
	if nome == 0 || testo == 0 {
		return false
	}
	var vuoto variant
	var t comObj
	// 6 = TASK_CREATE_OR_UPDATE; 3 = TASK_LOGON_INTERACTIVE_TOKEN (solo con l'utente collegato)
	r, _, _ := syscall.SyscallN(metodo(c, 16), uintptr(c), nome, testo, 6,
		uintptr(unsafe.Pointer(&vuoto)), uintptr(unsafe.Pointer(&vuoto)), 3, uintptr(unsafe.Pointer(&vuoto)), uintptr(unsafe.Pointer(&t)))
	t.release()
	return hrOK(r)
}

// copiaInstallata: exe è il Tasso.exe installato (Programmi\Tasso o la cartella dell'installatore),
// in una cartella dove scrivono solo gli amministratori. Solo questo può aprirsi dall'attività:
// un Tasso.exe in Download lo potrebbe cambiare qualunque programma.
func copiaInstallata(exe string) bool {
	exe = filepath.Clean(exe)
	if strings.ContainsAny(exe, "\"%\r\n") {
		return false
	}
	if strings.EqualFold(exe, filepath.Join(cartellaTasso(), "Tasso.exe")) {
		return true
	}
	_, dir := disinstallatoreEcosistema() // ecosistema.go: la cartella scritta dall'installatore
	return dir != "" && strings.EqualFold(exe, filepath.Join(filepath.Clean(dir), "Tasso.exe"))
}

// preparaAttivitaApri: crea o rifà \Tasso\Apri Tasso se serve. Da amministratore, solo per il Tasso
// installato e solo con UAC acceso. true = l'attività c'è ed è a posto.
func preparaAttivitaApri() bool {
	exe, err := os.Executable()
	if err != nil || exe == "" || !elevato() || !elevazioneDivisa() {
		return false
	}
	exe = filepath.Clean(exe)
	sid := sidUtente()
	if sid == "" || !copiaInstallata(exe) {
		return false
	}
	ok := false
	conPianificatore(func(s comObj) {
		if t := prendiAttivita(s); t != 0 {
			a, err := leggiAttivitaWin(t)
			t.release()
			if err == nil && a.Aggiornata(exe, sid) {
				ok = true
				return
			}
		}
		ok = registraAttivita(s, apertura.XMLAttivita(exe, sid))
	})
	return ok
}

// preparaAperturaDaRiga: Tasso.exe --prepara-apertura (l'installatore). Scrive il segno, poi
// "APERTURA-OK" o "APERTURA-NO".
func preparaAperturaDaRiga() int {
	fmt.Println(segnoPreparaApertura)
	if preparaAttivitaApri() {
		fmt.Println("APERTURA-OK")
		return 0
	}
	fmt.Println("APERTURA-NO")
	return 1
}
