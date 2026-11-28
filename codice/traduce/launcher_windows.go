//go:build windows

package traduce

// Dati del programma in primo piano per launcher.go: padri del processo (dall'elenco dei
// processi di Windows, controllando che il padre sia nato prima: un numero di processo può
// essere riusato) ed editore del file (CompanyName nelle informazioni di versione).

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	muParenti    sync.Mutex
	cacheParenti = map[string]esitoParente{} // pid|percorso -> esito
	cacheEditori = map[string]string{}
)

type esitoParente struct {
	nome   string
	ok     bool
	quando time.Time
}

// giocoParenteDiFinestra: la finestra h è di un gioco della libreria partito da un launcher
// (o nella sua cartella, o dello stesso editore). Esito ricordato per un minuto per processo.
func giocoParenteDiFinestra(h uintptr) (string, bool) {
	if Ospite.VociLibreria == nil || h == 0 {
		return "", false
	}
	var pid uint32
	pGetWindowThreadProcessId.Call(h, uintptr(unsafe.Pointer(&pid)))
	perc := percorsoDiFinestra(h)
	if pid == 0 || perc == "" {
		return "", false
	}
	chiave := fmt.Sprint(pid, "|", strings.ToLower(perc))
	muParenti.Lock()
	if e, ok := cacheParenti[chiave]; ok && time.Since(e.quando) < time.Minute {
		muParenti.Unlock()
		return e.nome, e.ok
	}
	muParenti.Unlock()
	d := datiProcesso{Percorso: perc, Editore: editoreFile(perc), Antenati: antenatiProcesso(pid)}
	nome, ok := giocoDaLibreria(d, Ospite.VociLibreria(), editoreFile)
	muParenti.Lock()
	if len(cacheParenti) > 200 {
		cacheParenti = map[string]esitoParente{}
	}
	cacheParenti[chiave] = esitoParente{nome, ok, time.Now()}
	muParenti.Unlock()
	return nome, ok
}

// antenatiProcesso: i percorsi di padre, nonno… del processo (al massimo 6), finché sono aperti.
func antenatiProcesso(pid uint32) []string {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	padre := map[uint32]uint32{}
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		padre[pe.ProcessID] = pe.ParentProcessID
	}
	var out []string
	figlio := pid
	nato := nascitaProcesso(pid)
	for i := 0; i < 6; i++ {
		p, ok := padre[figlio]
		if !ok || p == 0 || p == 4 || p == figlio {
			break
		}
		np := nascitaProcesso(p)
		if np == 0 || (nato != 0 && np > nato) {
			break // il padre vero è chiuso e il numero è stato riusato da un altro programma
		}
		if perc := percorsoProcesso(p); perc != "" {
			out = append(out, strings.ToLower(perc))
		}
		figlio, nato = p, np
	}
	return out
}

func nascitaProcesso(pid uint32) int64 {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(h)
	var c, e, k, u windows.Filetime
	if windows.GetProcessTimes(h, &c, &e, &k, &u) != nil {
		return 0
	}
	return c.Nanoseconds()
}

func percorsoProcesso(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

// editoreFile: il CompanyName del programma ("" se non c'è). Ricordato per percorso.
func editoreFile(p string) string {
	k := strings.ToLower(p)
	muParenti.Lock()
	if e, ok := cacheEditori[k]; ok {
		muParenti.Unlock()
		return e
	}
	muParenti.Unlock()
	e := leggiEditore(p)
	muParenti.Lock()
	if len(cacheEditori) > 500 {
		cacheEditori = map[string]string{}
	}
	cacheEditori[k] = e
	muParenti.Unlock()
	return e
}

func leggiEditore(p string) (editore string) {
	defer func() {
		if recover() != nil {
			editore = ""
		}
	}()
	var zero windows.Handle
	n, err := windows.GetFileVersionInfoSize(p, &zero)
	if err != nil || n == 0 || n > 4<<20 {
		return ""
	}
	buf := make([]byte, n)
	if windows.GetFileVersionInfo(p, 0, n, unsafe.Pointer(&buf[0])) != nil {
		return ""
	}
	var trad unsafe.Pointer
	var lt uint32
	lingue := []string{"040904b0", "040904e4", "000004b0"}
	if windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\VarFileInfo\Translation`, unsafe.Pointer(&trad), &lt) == nil && lt >= 4 {
		w := unsafe.Slice((*uint16)(trad), 2)
		lingue = append([]string{fmt.Sprintf("%04x%04x", w[0], w[1])}, lingue...)
	}
	for _, l := range lingue {
		var v unsafe.Pointer
		var lv uint32
		if windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\StringFileInfo\`+l+`\CompanyName`, unsafe.Pointer(&v), &lv) == nil && lv > 0 {
			return strings.TrimSpace(windows.UTF16PtrToString((*uint16)(v)))
		}
	}
	return ""
}
