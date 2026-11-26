// Package apertura: la logica (senza Windows) per aprire Tasso con i permessi da amministratore
// senza lo scudo sull'icona e senza la richiesta di Windows a ogni apertura.
//
// Come funziona (il resto è in ../apri_admin.go):
//   - Tasso.exe non chiede più l'amministratore nel suo manifest (asInvoker): così Windows non mette
//     lo scudo sull'icona (Desktop, menu Start, barra).
//   - Tasso, quando gira da amministratore, prepara l'attività pianificata \Tasso\Apri Tasso
//     (permessi più alti, utente collegato, nessun orario: parte solo quando la si chiama).
//   - Aperto senza permessi (doppio clic), Tasso avvia quell'attività con un parametro e si chiude;
//     la copia che parte dall'attività è da amministratore. Se l'attività non c'è o non parte,
//     Tasso chiede i permessi come prima (la finestra di Windows "Vuoi consentire...").
//
// Chiunque nella sessione può avviare l'attività con un parametro a piacere: per questo la copia
// che parte dall'attività tiene solo gli argomenti di questo elenco (ArgomentiDaAttivita) e butta
// tutti gli altri (per esempio quelli che cancellano file o tolgono Tasso).
package apertura

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

const (
	// CartellaAttivita e NomeAttivita: \Tasso\Apri Tasso <SID>, una per ogni utente (due
	// amministratori sullo stesso PC non si rubano l'attività), nella stessa cartella delle altre
	// attività di Tasso (così "Togli Tasso dal PC" e il disinstallatore la tolgono con le altre).
	CartellaAttivita = `\Tasso`
	NomeAttivita     = "Apri Tasso"
	// ArgDaAttivita: il primo argomento della copia avviata dall'attività (fisso nell'attività).
	ArgDaAttivita = "--da-attivita"
	// VersioneAttivita: cambia quando cambia XMLAttivita, così Tasso rifà l'attività vecchia.
	VersioneAttivita = "tasso-apri-2"
)

// NomeAttivitaPer: il nome dell'attività dell'utente sid ("Apri Tasso S-1-5-21-...").
func NomeAttivitaPer(sid string) string { return NomeAttivita + " " + sid }

// SidValido: un SID scritto come testo (S-1-...: solo cifre e trattini), da mettere nel nome
// dell'attività e nei permessi.
func SidValido(s string) bool {
	if len(s) < 5 || len(s) > 184 || !strings.HasPrefix(s, "S-1-") {
		return false
	}
	for _, c := range s[4:] {
		if !(c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return !strings.Contains(s, "--") && !strings.HasSuffix(s, "-")
}

// SDDLAttivita: i permessi dell'attività. SYSTEM e Amministratori tutto; l'utente solo leggerla e
// avviarla (serve alla copia senza permessi), non cambiarla né cancellarla. Nessun permesso ereditato.
func SDDLAttivita(sid string) string {
	return "D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FRFX;;;" + sid + ")"
}

// Silenziosi: argomenti delle attività che partono già da amministratore (pulizia programmata,
// Profilo gioco). Se arrivano a un Tasso senza permessi qualcosa non va: esco senza chiedere niente,
// invece di far comparire la richiesta di Windows all'improvviso.
func Silenziosi(args []string) bool {
	for _, a := range args {
		if a == "--scheduled" || a == "--profile" || strings.HasPrefix(a, "--auto=") {
			return true
		}
	}
	return false
}

// permessi: gli unici argomenti che passano dall'attività (e il segno che li accompagna).
//   - --controlla-aggiornamenti: lo usano Tasso Zip e Tasso PDF (girano senza permessi);
//   - --min: Tasso ridotto a icona.
var permessi = map[string]bool{
	"--controlla-aggiornamenti": true,
	"--min":                     true,
}

// TuttiPermessi: true se ogni argomento può passare dall'attività. Con altri argomenti Tasso non
// usa l'attività e chiede i permessi a Windows come prima, con tutti gli argomenti.
func TuttiPermessi(args []string) bool {
	for _, a := range args {
		if !permessi[a] {
			return false
		}
	}
	return true
}

// NonceValido: il segno che l'avvio dall'attività è partito (32 cifre esadecimali minuscole).
func NonceValido(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// ParametroAttivita: il parametro $(Arg0) dell'attività: il segno e gli argomenti permessi.
func ParametroAttivita(nonce string, args []string) string {
	parti := []string{nonce}
	for _, a := range args {
		if permessi[a] {
			parti = append(parti, a)
		}
	}
	return strings.Join(parti, " ")
}

// DaAttivita legge gli argomenti (senza il nome del programma). Se il primo è ArgDaAttivita,
// la copia è partita dall'attività: restituisce il segno (se c'è ed è valido) e SOLO gli
// argomenti permessi, una volta ciascuno. Altrimenti da = false e args tali e quali.
func DaAttivita(args []string) (da bool, nonce string, resto []string) {
	if len(args) == 0 || args[0] != ArgDaAttivita {
		return false, "", args
	}
	visti := map[string]bool{}
	for _, a := range args[1:] {
		switch {
		case nonce == "" && NonceValido(a):
			nonce = a
		case permessi[a] && !visti[a]:
			visti[a] = true
			resto = append(resto, a)
		}
	}
	return true, nonce, resto
}

// QuotaArg: un argomento per la riga di comando di Windows (regole di CommandLineToArgvW).
func QuotaArg(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n\v\"") {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	barre := 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\\':
			barre++
		case '"':
			b.WriteString(strings.Repeat(`\`, barre*2+1))
			b.WriteByte('"')
			barre = 0
		default:
			b.WriteString(strings.Repeat(`\`, barre))
			b.WriteByte(c)
			barre = 0
		}
	}
	b.WriteString(strings.Repeat(`\`, barre*2))
	b.WriteByte('"')
	return b.String()
}

// RigaComando: gli argomenti uniti per ShellExecute.
func RigaComando(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = QuotaArg(a)
	}
	return strings.Join(q, " ")
}

func esc(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// cartellaDi: la cartella di un percorso di Windows (C:\Program Files\Tasso\Tasso.exe ->
// C:\Program Files\Tasso), anche compilando su Linux.
func cartellaDi(p string) string {
	if i := strings.LastIndexByte(p, '\\'); i > 0 {
		if i == 2 && p[1] == ':' {
			return p[:3]
		}
		return p[:i]
	}
	return p
}

// XMLAttivita: l'attività \Tasso\Apri Tasso per exe (percorso completo di Tasso.exe, in una cartella
// dove scrivono solo gli amministratori) e l'utente sid. Nessun orario: parte solo quando Tasso la
// chiama. Più copie insieme (la seconda porta davanti la prima), anche a batteria, senza tempo
// massimo, priorità normale (di base le attività hanno priorità bassa: Tasso sarebbe più lento).
func XMLAttivita(exe, sid string) string {
	return `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Author>Tasso</Author>
    <Version>` + VersioneAttivita + `</Version>
    <Description>Apre Tasso con i permessi da amministratore senza chiederli ogni volta.</Description>
  </RegistrationInfo>
  <Principals>
    <Principal id="Author">
      <UserId>` + esc(sid) + `</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>HighestAvailable</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>Parallel</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>false</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <RunOnlyIfIdle>false</RunOnlyIfIdle>
    <WakeToRun>false</WakeToRun>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>4</Priority>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>"` + esc(exe) + `"</Command>
      <Arguments>` + ArgDaAttivita + ` $(Arg0)</Arguments>
      <WorkingDirectory>` + esc(cartellaDi(exe)) + `</WorkingDirectory>
    </Exec>
  </Actions>
</Task>
`
}

// Attivita: quello che serve sapere di un'attività letta da Windows (IRegisteredTask.Xml).
type Attivita struct {
	Versione  string
	Utente    string
	Comando   string // senza virgolette
	Argomenti string
	Livello   string
	Multiple  string
	Attiva    bool // Settings>Enabled (di base sì)
}

// LeggiAttivita legge l'XML di un'attività (quello che restituisce Windows, in UTF-16 già
// convertito in testo: la dichiarazione "UTF-16" si ignora).
func LeggiAttivita(testo string) (Attivita, error) {
	var t struct {
		Versione  string `xml:"RegistrationInfo>Version"`
		Principal []struct {
			UserID   string `xml:"UserId"`
			RunLevel string `xml:"RunLevel"`
		} `xml:"Principals>Principal"`
		Multiple string `xml:"Settings>MultipleInstancesPolicy"`
		Enabled  string `xml:"Settings>Enabled"`
		Exec     []struct {
			Command   string `xml:"Command"`
			Arguments string `xml:"Arguments"`
		} `xml:"Actions>Exec"`
	}
	d := xml.NewDecoder(strings.NewReader(testo))
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	if err := d.Decode(&t); err != nil {
		return Attivita{}, err
	}
	if len(t.Exec) != 1 || len(t.Principal) != 1 {
		return Attivita{}, errors.New("attività con più azioni o più utenti")
	}
	cmd := strings.TrimSpace(t.Exec[0].Command)
	if len(cmd) >= 2 && cmd[0] == '"' && cmd[len(cmd)-1] == '"' {
		cmd = cmd[1 : len(cmd)-1]
	}
	return Attivita{
		Versione:  strings.TrimSpace(t.Versione),
		Utente:    strings.TrimSpace(t.Principal[0].UserID),
		Comando:   cmd,
		Argomenti: strings.TrimSpace(t.Exec[0].Arguments),
		Livello:   strings.TrimSpace(t.Principal[0].RunLevel),
		Multiple:  strings.TrimSpace(t.Multiple),
		Attiva:    !strings.EqualFold(strings.TrimSpace(t.Enabled), "false"),
	}, nil
}

// Usabile: l'attività apre proprio exe, per l'utente sid, con il segno fisso davanti (la può usare
// la copia senza permessi di quell'utente).
func (a Attivita) Usabile(exe, sid string) bool {
	return a.Comando != "" && strings.EqualFold(a.Comando, exe) && strings.EqualFold(a.Utente, sid) &&
		strings.HasPrefix(a.Argomenti, ArgDaAttivita+" ")
}

// Aggiornata: come Usabile, e in più è quella di questa versione di Tasso, ed è attiva (non va
// rifatta; una disattivata a mano si rifà attiva).
func (a Attivita) Aggiornata(exe, sid string) bool {
	return a.Usabile(exe, sid) && a.Versione == VersioneAttivita && a.Argomenti == ArgDaAttivita+" $(Arg0)" &&
		a.Livello == "HighestAvailable" && a.Multiple == "Parallel" && a.Attiva
}
