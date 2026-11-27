package main

// "Il tuo nome" (3.2): il nome o nickname che hai scritto nell'installatore (o nelle Impostazioni
// di Tasso, di Tasso Zip o di Tasso PDF). Tasso lo usa per salutarti ("Bentornato, Sergio!",
// "Ciao, Sergio"), in pochi messaggi e, se lo lasci acceso, sul biglietto.
//
// Dove sta: HKCU\Software\Tasso, valore "Nome" (REG_SZ), uno per ogni utente di Windows e solo
// su questo PC. È condiviso con Tasso Zip e Tasso PDF (stessa chiave): cambiarlo in uno lo cambia
// in tutti. Non va MAI in rete: non entra nei messaggi di "Scrivici", nei numeri anonimi, negli
// aggiornamenti o nella classifica (spenta, CLASSIFICA_ATTIVA = false; quando ci sarà, il campo
// del nickname potrà solo proporlo, e lo manderà solo se lo scegli tu).
//
// Utente vero: se Tasso gira come un altro account (UAC con la password di un altro
// amministratore, vedi utenteDiverso in apri_admin.go), il nome giusto è quello di chi è collegato
// al PC: lo leggo e lo scrivo in HKEY_USERS\<SID di chi usa Esplora file>\Software\Tasso, come fa
// l'installatore (sistema_windows.go: utenteSessione), dove lo trovano anche Tasso Zip e Tasso PDF.
//
// Le regole (da 2 a 20 caratteri, lettere, numeri, spazio, punto, - e _) stanno in nomeutente/.

import (
	"strings"
	"sync"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"tasso/nomeutente"
)

const (
	valoreNome          = "Nome"
	valoreNomeBiglietto = "NomeBiglietto" // "Mostra il mio nome sul biglietto" (REG_DWORD, 0 = spento)
)

var nomeMu sync.Mutex

// sidSessione: il SID di chi usa questo PC (il proprietario di explorer.exe nella sessione di
// Tasso), se è diverso da quello con cui gira Tasso. "" = lo stesso utente (il caso normale) o non
// si sa (niente Esplora file): allora vale HKCU.
func sidSessione() string {
	io, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return ""
	}
	mio := io.User.Sid.String()
	var sessione uint32
	if windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &sessione) != nil {
		return ""
	}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	altro := ""
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
		sid := u.User.Sid.String()
		if strings.EqualFold(sid, mio) {
			return "" // Esplora file è nostro: lo stesso utente
		}
		if altro == "" {
			altro = sid
		}
	}
	return altro
}

// chiaveNome: dove sta il nome dell'utente vero (vedi sopra). Il SID arriva da Windows ma lo
// controllo lo stesso: solo "S-" seguito da cifre e trattini.
func chiaveNome() (registry.Key, string) {
	if sid := sidSessione(); sid != "" && strings.HasPrefix(sid, "S-") && strings.Trim(sid[2:], "0123456789-") == "" {
		return registry.USERS, sid + `\` + chiaveEcosistema
	}
	return registry.CURRENT_USER, chiaveEcosistema
}

// archivioRegistro: il valore "Nome" nella chiave dell'utente vero (nomeutente.Archivio).
type archivioRegistro struct{}

func (archivioRegistro) Leggi() (string, bool) {
	radice, percorso := chiaveNome()
	k, err := registry.OpenKey(radice, percorso, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer k.Close()
	v, tipo, err := k.GetStringValue(valoreNome)
	if err != nil || tipo != registry.SZ {
		return "", false
	}
	return v, true
}

func (archivioRegistro) Scrivi(n string) error {
	radice, percorso := chiaveNome()
	k, _, err := registry.CreateKey(radice, percorso, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(valoreNome, n)
}

// Togli: in HKCU e, se diverso, nell'utente vero; con il nome va via anche la scelta del biglietto.
func (archivioRegistro) Togli() error {
	radice, percorso := chiaveNome()
	type posto struct {
		k registry.Key
		p string
	}
	posti := []posto{{registry.CURRENT_USER, chiaveEcosistema}}
	if radice != registry.CURRENT_USER {
		posti = append(posti, posto{radice, percorso})
	}
	for _, p := range posti {
		if k, err := registry.OpenKey(p.k, p.p, registry.SET_VALUE); err == nil {
			_ = k.DeleteValue(valoreNome)
			_ = k.DeleteValue(valoreNomeBiglietto)
			k.Close()
		}
	}
	return nil
}

// leggiNome: il nome salvato, "" se non c'è o non è valido (scritto a mano, troppo lungo...).
func leggiNome() string {
	nomeMu.Lock()
	defer nomeMu.Unlock()
	return nomeutente.Leggi(archivioRegistro{})
}

// salvaNome pulisce e controlla il nome scritto e lo salva. Restituisce il nome salvato e l'esito
// ("" = salvato; "corto", "simbolo"... vedi nomeutente; "registro" = Windows non lo lascia scrivere).
func salvaNome(scritto string) (string, nomeutente.Esito) {
	nomeMu.Lock()
	defer nomeMu.Unlock()
	return nomeutente.Salva(archivioRegistro{}, scritto)
}

// togliNome: "Togli Tasso dal PC", quando Tasso è l'ultimo programma della famiglia, cancella anche il
// nome (in HKCU e, se diverso, nell'utente vero). Con Zip o PDF ancora sul PC il nome resta.
func togliNome() {
	nomeMu.Lock()
	defer nomeMu.Unlock()
	_ = archivioRegistro{}.Togli()
}

// nomeSulBiglietto: l'interruttore "Mostra il mio nome sul biglietto" (acceso di base). Valore
// NomeBiglietto (REG_DWORD) accanto al nome: 0 = spento, assente o altro = acceso.
func nomeSulBiglietto() bool {
	radice, percorso := chiaveNome()
	k, err := registry.OpenKey(radice, percorso, registry.QUERY_VALUE)
	if err != nil {
		return true
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue(valoreNomeBiglietto)
	return err != nil || v != 0
}

func salvaNomeSulBiglietto(acceso bool) bool {
	radice, percorso := chiaveNome()
	k, _, err := registry.CreateKey(radice, percorso, registry.SET_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v := uint32(0)
	if acceso {
		v = 1
	}
	return k.SetDWordValue(valoreNomeBiglietto, v) == nil
}

// nomeInfo: quello che serve alla finestra.
func nomeInfo() map[string]interface{} {
	return map[string]interface{}{"nome": leggiNome(), "biglietto": nomeSulBiglietto(), "min": nomeutente.Min, "max": nomeutente.Max}
}

func bindNome(w webview2.WebView) {
	_ = w.Bind("goNomeLeggi", func() map[string]interface{} { return nomeInfo() })
	_ = w.Bind("goNomeSalva", func(scritto string) map[string]interface{} {
		n, e := salvaNome(scritto)
		if e.Codice != nomeutente.OK {
			return map[string]interface{}{"ok": false, "errore": e.Codice, "simbolo": e.Simbolo}
		}
		return map[string]interface{}{"ok": true, "nome": n}
	})
	_ = w.Bind("goNomeBiglietto", func(acceso bool) bool { return salvaNomeSulBiglietto(acceso) })
}
