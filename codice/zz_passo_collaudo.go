//go:build collaudo

package main

// Nome zz_: il suo init deve girare dopo quelli di steps_*.go (che aggiungono i passaggi).
// Solo per le prove (build con -tags collaudo, mai nella versione pubblicata):
// Tasso-collaudo.exe --passo=of_new_call esegue un passaggio e scrive i messaggi in
// %LOCALAPPDATA%\Tasso\logs\passo-<id>.txt, senza aprire la finestra.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"

	"tasso/apertura"
)

func init() {
	// partito dall'Utilità di pianificazione (attività Apri Tasso): niente modalità di prova, gli
	// argomenti li sceglie chi avvia l'attività (apertura/collaudo.go li ha già filtrati)
	if len(os.Args) > 1 && os.Args[1] == apertura.ArgDaAttivita {
		return
	}
	for _, a := range os.Args[1:] {
		if strings.HasPrefix(a, "--guardia=") {
			// --guardia=6: fa girare la guardia della Modalità riunione per 6 minuti senza finestra
			// (anche con un altro Tasso aperto) e scrive ogni 30 s se riunione.json c'è ancora
			min, _ := strconv.Atoi(strings.TrimPrefix(a, "--guardia="))
			dir := filepath.Join(os.Getenv("LOCALAPPDATA"), "Tasso", "logs")
			_ = os.MkdirAll(dir, 0o755)
			f, err := os.Create(filepath.Join(dir, "guardia.txt"))
			if err != nil {
				os.Exit(2)
			}
			go officeGuardia()
			fine := time.Now().Add(time.Duration(min) * time.Minute)
			for time.Now().Before(fine) {
				_, e := os.Stat(filepath.Join(officeDir(), "riunione.json"))
				fmt.Fprintf(f, "%s  riunione.json: %v  microfono in uso: %v\n", time.Now().Format("15:04:05"), e == nil, micInUso())
				time.Sleep(30 * time.Second)
			}
			f.Close()
			os.Exit(0)
		}
		if a == "--prova-togli" {
			// "Togli Tasso dal PC" a secco: scrive cosa toglierebbe, senza toccare niente
			appDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "Tasso")
			var b strings.Builder
			for _, d := range cartelleTasso() {
				fmt.Fprintf(&b, "cartella: %s (%.1f MB)\n", d, float64(dimensioneCartella(d))/1e6)
			}
			fmt.Fprintf(&b, "portachiavi presente: %v\n", vaultExists())
			// copie dei giochi: dalla 3.2 in Programmi\Tasso\lavoro, prima in AppData (qui non sposto niente)
			for _, copie := range []string{filepath.Join(cartellaTasso(), "lavoro", "copie-giochi"), filepath.Join(os.Getenv("LOCALAPPDATA"), "TassoTraduce", "copie-giochi")} {
				voci, _ := os.ReadDir(copie)
				for _, v := range voci {
					if !v.IsDir() {
						continue
					}
					m, _ := os.ReadFile(filepath.Join(copie, v.Name(), "manifesto.json"))
					fmt.Fprintf(&b, "traduzione in un gioco: %s (%s) manifesto=%v plugin-utente=%v\n", v.Name(), copie, len(m) > 0, func() bool { _, e := os.Stat(filepath.Join(copie, v.Name(), "plugin-utente")); return e == nil }())
				}
			}
			bk, _ := os.ReadDir(filepath.Join(appDir, "backup"))
			fmt.Fprintf(&b, "backup del registro da spostare in Documenti: %d file\n", len(bk))
			r := runPS(`Get-ScheduledTask -TaskPath '\Tasso\' -ErrorAction SilentlyContinue | ForEach-Object { Say ('attività: ' + $_.TaskName) }
$lnk = Join-Path ([Environment]::GetFolderPath('Desktop')) 'Tasso.lnk'
if (Test-Path -LiteralPath $lnk) { Say ('collegamento Desktop -> ' + (New-Object -ComObject WScript.Shell).CreateShortcut($lnk).TargetPath) } else { Say 'collegamento Desktop: nessuno' }
foreach ($k in 'HKCU:\Software\Tasso','HKLM:\SOFTWARE\Tasso') { Say ($k + ': ' + (Test-Path -LiteralPath $k)) }`, time.Minute, func(m string) { b.WriteString(m + "\n") })
			_ = r
			piano := pianoTogliTasso()
			dis, dirDis := disinstallatoreEcosistema()
			fmt.Fprintf(&b, "\n--- ecosistema ---\ncartella di Tasso: %s\nmodalità dell'installatore: %q (fissa: %q)\nlingua dell'installatore: %q, lingua di Tasso: %s\ndisinstallatore: %q in %q\necosistema: %v\nintere: %q\nfile: %q\nse vuote: %q\n",
				cartellaTasso(), modalitaInstallatore(), modalitaFissa(), linguaInstallatore(), curLang(), dis, dirDis, piano.eco, piano.intere, piano.file, piano.seVuote)
			testo, err := scriptCancellazione(piano)
			fmt.Fprintf(&b, "\n--- comando che girerebbe dopo la chiusura (errore: %v) ---\n%s", err, testo)
			_ = os.MkdirAll(filepath.Join(appDir, "logs"), 0o755)
			_ = os.WriteFile(filepath.Join(appDir, "logs", "prova-togli.txt"), []byte(b.String()), 0o644)
			os.Exit(0)
		}
		if a == "--prova-mouse" {
			// mouse senza accelerazione per uno sparatutto (cs2) e poi rimesso com'era, con le
			// chiamate vere di Windows; il registro non deve cambiare
			appDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "Tasso") // come in main (boost.json)
			dir := filepath.Join(appDir, "logs")
			_ = os.MkdirAll(dir, 0o755)
			f, err := os.Create(filepath.Join(dir, "prova-mouse.txt"))
			if err != nil {
				os.Exit(2)
			}
			reg := func() string {
				var out []string
				for _, n := range []string{"MouseSpeed", "MouseThreshold1", "MouseThreshold2"} {
					v, _ := regGet(registry.CURRENT_USER, `Control Panel\Mouse`, n)
					out = append(out, n+"="+v)
				}
				return strings.Join(out, " ")
			}
			m0, ok := mouseGet()
			fmt.Fprintf(f, "prima: mouse %v (letto %v), registro %s, boost.mouse %v\n", m0, ok, reg(), mouseAccelActive())
			originale := m0
			if ok && m0[2] == 0 {
				// accelerazione spenta: la accendo solo per questa sessione (niente registro) per
				// provare il caso vero, e alla fine la rimetto spenta
				mouseSet([3]int32{6, 10, 1})
				m0, _ = mouseGet()
				fmt.Fprintf(f, "accesa per la prova (solo sessione): mouse %v, registro %s\n", m0, reg())
			}
			fmt.Fprintf(f, "spengo per cs2: %q\n", mouseAccelOff("cs2"))
			m1, _ := mouseGet()
			fmt.Fprintf(f, "durante: mouse %v, registro %s, boost.mouse %v\n", m1, reg(), mouseAccelActive())
			fmt.Fprintf(f, "gioco non competitivo (nomanssky): %q\n", mouseAccelOff("nomanssky"))
			fmt.Fprintf(f, "rimetto: %q\n", mouseAccelRestore())
			m2, _ := mouseGet()
			fmt.Fprintf(f, "dopo: mouse %v, registro %s, boost.mouse %v\n", m2, reg(), mouseAccelActive())
			fmt.Fprintf(f, "esito: %v\n", m2 == m0 && !mouseAccelActive())
			if originale != m2 {
				mouseSet(originale)
				m3, _ := mouseGet()
				fmt.Fprintf(f, "rimesso come all'inizio: mouse %v, registro %s\n", m3, reg())
			}
			f.Close()
			os.Exit(0)
		}
		if !strings.HasPrefix(a, "--passo=") {
			continue
		}
		id := strings.TrimPrefix(a, "--passo=")
		dir := filepath.Join(os.Getenv("LOCALAPPDATA"), "Tasso", "logs")
		_ = os.MkdirAll(dir, 0o755)
		f, err := os.Create(filepath.Join(dir, "passo-"+id+".txt"))
		if err != nil {
			os.Exit(2)
		}
		found := false
		for _, s := range allSteps {
			if s.ID != id {
				continue
			}
			found = true
			t0 := time.Now()
			r := runPS(s.Script, s.Timeout, func(m string) { fmt.Fprintln(f, time.Now().Format("15:04:05")+"  "+m) })
			fmt.Fprintf(f, "-- fine in %.1f s, errore: %v\n", time.Since(t0).Seconds(), r.Err)
		}
		if !found {
			fmt.Fprintln(f, "passaggio non trovato: "+id)
		}
		f.Close()
		os.Exit(0)
	}
}
