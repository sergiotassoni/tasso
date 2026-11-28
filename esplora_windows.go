// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

//go:build windows

package main

// La vista "Esplora i file" (come i classici gestori di archivi): cartelle, dischi, ricerca,
// Info, Verifica, Ripara, Elimina (sempre nel Cestino), apertura dei file.
// Le letture lente (cartelle, ricerca, misure) girano in un filo a parte e rispondono con
// un evento alla pagina: la finestra non si blocca mai, neanche su una cartella di rete lenta.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"tassozip/archivio"
	"tassozip/esplora"
	"tassozip/testi"
)

var (
	trovaMu   sync.Mutex
	trovaStop context.CancelFunc

	kernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	procGetLogicalDrives      = kernel32.NewProc("GetLogicalDrives")
	procGetDriveTypeW         = kernel32.NewProc("GetDriveTypeW")
	procGetVolumeInformationW = kernel32.NewProc("GetVolumeInformationW")
	procSHFileOperationW      = shell32.NewProc("SHFileOperationW")
)

func collegaEsplora() {
	w := finestra
	_ = w.Bind("goEsplora", func(id int, dir string, nascosti bool) string {
		go leggiCartella(id, dir, nascosti)
		return "ok"
	})
	_ = w.Bind("goLuoghi", luoghi)
	_ = w.Bind("goTrova", func(id int, dir, testo string, nascosti bool) string {
		ctx, stop := context.WithCancel(context.Background())
		trovaMu.Lock()
		if trovaStop != nil {
			trovaStop()
		}
		trovaStop = stop
		trovaMu.Unlock()
		go func() {
			defer stop()
			defer recuperaPanico("trova")
			r, err := esplora.Trova(ctx, dir, testo, esplora.OpzioniTrova{Nascosti: nascosti})
			if errors.Is(err, context.Canceled) {
				return
			}
			manda("trovati", map[string]interface{}{"id": id, "ris": r, "codice": archivio.Codice(err)})
		}()
		return "ok"
	})
	_ = w.Bind("goFermaTrova", func() string {
		trovaMu.Lock()
		if trovaStop != nil {
			trovaStop()
		}
		trovaMu.Unlock()
		return "ok"
	})
	_ = w.Bind("goInfoPercorsi", func(id int, percorsi []string) string {
		go infoPercorsi(id, percorsi)
		return "ok"
	})
	_ = w.Bind("goVerifica", avviaVerifica)
	_ = w.Bind("goRipara", avviaRipara)
	_ = w.Bind("goCestino", cestino)
	_ = w.Bind("goApriFile", func(p string, confermato bool) string {
		fi, err := os.Stat(p)
		if err != nil || fi.IsDir() {
			return "no"
		}
		if pericoloso(p) && !confermato {
			return "conferma"
		}
		if w := finestra; w != nil && !chiusa.Load() {
			w.Dispatch(func() { apriFile(p) })
		}
		return "ok"
	})
	_ = w.Bind("goScegliDestinazione", func() []string {
		return scegli(testi.T(lingua(), "scegli_destinazione"), true, false, nil)
	})
	_ = w.Bind("goEstraiIn", func(percorso, password, dest, modo string) string {
		opz := archivio.OpzioniEstrai{Password: password, Modo: modoEstrai(modo), Destinazione: dest}
		return avviaEstrai(percorso, opz, "estrai")
	})
}

func recuperaPanico(dove string) {
	if r := recover(); r != nil {
		registraErrore("%s: %v\n%s", dove, r, debug.Stack())
	}
}

// cartellaIniziale: l'ultima usata, se c'è ancora; altrimenti Download (o la cartella utente).
func cartellaIniziale() string {
	if p := impostazioniCopia().Esplora; p != "" {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			return p
		}
	}
	if p, err := windows.KnownFolderPath(windows.FOLDERID_Downloads, 0); err == nil {
		return p
	}
	if p, err := os.UserHomeDir(); err == nil {
		return p
	}
	return `C:\`
}

func leggiCartella(id int, dir string, nascosti bool) {
	defer recuperaPanico("esplora")
	if strings.TrimSpace(dir) == "" {
		dir = cartellaIniziale()
	}
	dir = os.ExpandEnv(strings.Trim(strings.TrimSpace(dir), `"`))
	if len(dir) == 2 && dir[1] == ':' {
		dir += `\` // "D:" = la radice del disco D
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	ctx, stop := context.WithTimeout(context.Background(), 20*time.Second)
	defer stop()
	el, err := esplora.Elenca(ctx, dir, esplora.Opzioni{Nascosti: nascosti})
	if err == nil {
		modificaImpostazioni(func(s *impostazioni) { s.Esplora = el.Percorso; s.Nascosti = nascosti })
	}
	codice := ""
	if err != nil {
		codice = archivio.Codice(err)
		if errors.Is(err, esplora.ErrNonCartella) {
			codice = "non_cartella"
		}
	}
	manda("cartella", map[string]interface{}{"id": id, "el": el, "codice": codice})
}

type luogo struct {
	Chiave   string `json:"chiave"` // per il testo ("desktop", "download"...) o "" per i dischi
	Nome     string `json:"nome"`
	Percorso string `json:"percorso"`
	Tipo     string `json:"tipo"` // "speciale", "fisso", "rimovibile", "rete", "cd"
}

// luoghi: cartelle speciali e dischi, per il piccolo elenco a sinistra.
func luoghi() map[string]interface{} {
	var out []luogo
	for _, s := range []struct {
		k  string
		id *windows.KNOWNFOLDERID
	}{
		{"desktop", windows.FOLDERID_Desktop}, {"download", windows.FOLDERID_Downloads},
		{"documenti", windows.FOLDERID_Documents}, {"immagini", windows.FOLDERID_Pictures},
	} {
		if p, err := windows.KnownFolderPath(s.id, 0); err == nil {
			out = append(out, luogo{Chiave: s.k, Percorso: p, Tipo: "speciale"})
		}
	}
	if p, err := os.UserHomeDir(); err == nil {
		out = append(out, luogo{Chiave: "home", Nome: filepath.Base(p), Percorso: p, Tipo: "speciale"})
	}
	maschera, _, _ := procGetLogicalDrives.Call()
	for i := 0; i < 26; i++ {
		if maschera&(1<<i) == 0 {
			continue
		}
		radice := string(rune('A'+i)) + `:\`
		r, _ := windows.UTF16PtrFromString(radice)
		t, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(r)))
		tipo := map[uintptr]string{2: "rimovibile", 3: "fisso", 4: "rete", 5: "cd", 6: "fisso"}[t]
		if tipo == "" {
			continue
		}
		nome := ""
		if tipo == "fisso" { // l'etichetta solo dei dischi fissi (gli altri possono essere lenti)
			buf := make([]uint16, 261)
			if ok, _, _ := procGetVolumeInformationW.Call(uintptr(unsafe.Pointer(r)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0, 0, 0, 0, 0); ok != 0 {
				nome = windows.UTF16ToString(buf)
			}
		}
		out = append(out, luogo{Nome: nome, Percorso: radice, Tipo: tipo})
	}
	return map[string]interface{}{"luoghi": out, "iniziale": cartellaIniziale(), "nascosti": impostazioniCopia().Nascosti}
}

// infoPercorsi: per "Info". Misura file e cartelle; per un archivio solo, anche cosa c'è dentro.
func infoPercorsi(id int, percorsi []string) {
	defer recuperaPanico("info")
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	out := map[string]interface{}{"id": id}
	out["misura"] = esplora.MisuraPercorsi(ctx, percorsi, 0, 10*time.Second)
	if len(percorsi) == 1 && archivio.SembraArchivio(percorsi[0]) {
		el, err := archivio.Elenca(ctx, percorsi[0], "")
		if err != nil {
			out["codice"] = archivio.Codice(err)
		} else {
			var file, cartelle int
			var orig, comp int64
			compNoto := true
			for _, v := range el.Voci {
				if v.Cartella {
					cartelle++
					continue
				}
				file++
				orig += v.Dim
				if v.Compresso >= 0 {
					comp += v.Compresso
				} else {
					compNoto = false
				}
			}
			if !compNoto {
				comp = el.Dim // per 7z solidi e tar compressi: la dimensione dell'archivio
			}
			out["archivio"] = map[string]interface{}{
				"tipo": el.Tipo, "metodo": el.Metodo, "solido": el.Solido, "cifrato": el.Cifrato,
				"file": file, "cartelle": cartelle, "orig": orig, "comp": comp, "dim": el.Dim, "saltate": el.Saltate,
			}
		}
	}
	manda("info", out)
}

func avviaVerifica(percorso, password string) string {
	ctx, _ := inizia()
	if ctx == nil {
		return "occupato"
	}
	go func() {
		defer finisci()
		defer func() {
			if r := recover(); r != nil {
				registraErrore("verifica: %v\n%s", r, debug.Stack())
				manda("errore", map[string]interface{}{"tipo": "verifica", "codice": "generico", "dettagli": fmt.Sprint(r), "percorso": percorso})
			}
		}()
		ris, err := archivio.Verifica(ctx, percorso, archivio.OpzioniVerifica{Password: password, Avanzamento: avanzamento})
		if err != nil {
			c := archivio.Codice(err)
			// Uno zip con password non è rovinato: Tasso Zip non lo sa leggere. Niente "Ripara".
			if c == "annullato" || c == "password" || c == "password_sbagliata" || c == "zip_cifrato" || c == "zip_cifrato_aes" {
				erroreVersoPagina("verifica", percorso, err)
				return
			}
			// Un archivio rovinato non è un errore del programma: è il risultato della verifica.
			manda("verificato", map[string]interface{}{"percorso": percorso, "ok": false, "codice": c, "dettagli": testi.Dettagli(lingua(), err.Error())})
			return
		}
		manda("verificato", map[string]interface{}{"percorso": percorso, "ok": true, "ris": ris})
	}()
	return "ok"
}

func avviaRipara(percorso string) string {
	ctx, _ := inizia()
	if ctx == nil {
		return "occupato"
	}
	go func() {
		defer finisci()
		defer func() {
			if r := recover(); r != nil {
				registraErrore("ripara: %v\n%s", r, debug.Stack())
				manda("errore", map[string]interface{}{"tipo": "ripara", "codice": "generico", "dettagli": fmt.Sprint(r), "percorso": percorso})
			}
		}()
		ris, err := archivio.RiparaZip(ctx, percorso, "", avanzamento)
		if err != nil {
			erroreVersoPagina("ripara", percorso, err)
			return
		}
		manda("fine", map[string]interface{}{
			"tipo": "ripara", "percorso": ris.Archivio, "file": ris.Salvati, "saltati": testi.Motivi(lingua(), ris.Persi), "archivio": percorso,
		})
	}()
	return "ok"
}

// shFileOpStruct: SHFILEOPSTRUCTW (allineamento naturale a 64 bit).
type shFileOpStruct struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

const (
	foDelete           = 3
	fofNoConfirmation  = 0x10
	fofAllowUndo       = 0x40
	fofWantNukeWarning = 0x4000
)

// protetto: cartelle che Tasso Zip non manda mai nel Cestino (dischi, cartelle di Windows,
// cartelle speciali dell'utente).
func protetto(p string) bool {
	p = filepath.Clean(p)
	if esplora.Padre(p) == "" {
		return true // radice di un disco o di una condivisione
	}
	var vietate []string
	for _, id := range []*windows.KNOWNFOLDERID{windows.FOLDERID_Desktop, windows.FOLDERID_Downloads, windows.FOLDERID_Documents,
		windows.FOLDERID_Pictures, windows.FOLDERID_Music, windows.FOLDERID_Videos, windows.FOLDERID_Profile,
		windows.FOLDERID_Windows, windows.FOLDERID_ProgramFiles, windows.FOLDERID_ProgramFilesX86, windows.FOLDERID_LocalAppData,
		windows.FOLDERID_RoamingAppData, windows.FOLDERID_UserProfiles} {
		if v, err := windows.KnownFolderPath(id, 0); err == nil {
			vietate = append(vietate, v)
		}
	}
	vietate = append(vietate, cartella)
	if exe, err := os.Executable(); err == nil {
		vietate = append(vietate, exe)
	}
	for _, v := range vietate {
		if strings.EqualFold(filepath.Clean(v), p) {
			return true
		}
	}
	return false
}

// cestino manda i file nel Cestino di Windows (FOF_ALLOWUNDO: si possono rimettere a posto).
// La conferma l'ha già chiesta la pagina. Se un file non può andare nel Cestino (per es. su
// una chiavetta o in rete), Windows avvisa (FOF_WANTNUKEWARNING) invece di cancellarlo per
// sempre in silenzio: l'utente può dire di no. Gira nel filo della finestra.
func cestino(percorsi []string) map[string]interface{} {
	var lista []uint16
	n := 0
	for _, p := range percorsi {
		p = filepath.Clean(p)
		if !filepath.IsAbs(p) || protetto(p) {
			return map[string]interface{}{"ok": false, "codice": "protetto", "nome": filepath.Base(p)}
		}
		if _, err := os.Lstat(p); err != nil {
			continue
		}
		u, err := windows.UTF16FromString(p)
		if err != nil {
			continue
		}
		lista = append(lista, u...) // con lo zero finale
		n++
	}
	if n == 0 {
		return map[string]interface{}{"ok": false, "codice": "non_trovato"}
	}
	lista = append(lista, 0) // doppio zero alla fine
	op := shFileOpStruct{wFunc: foDelete, pFrom: &lista[0], fFlags: fofAllowUndo | fofNoConfirmation | fofWantNukeWarning}
	if finestra != nil {
		op.hwnd = uintptr(finestra.Window())
	}
	r, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op)))
	if op.fAnyOperationsAborted != 0 {
		return map[string]interface{}{"ok": false, "codice": "annullato"}
	}
	if r != 0 {
		registraErrore("cestino: SHFileOperation 0x%x", r)
		return map[string]interface{}{"ok": false, "codice": "generico"}
	}
	return map[string]interface{}{"ok": true, "n": n}
}
