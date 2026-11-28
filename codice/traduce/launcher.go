package traduce

// Gioco partito da un launcher: in libreria c'è il launcher ("Hytale Launcher"), ma il gioco
// vero gira con un altro programma (HytaleClient.exe), magari in un'altra cartella. È lo stesso
// gioco della libreria se:
//   - il suo processo è figlio (o nipote) del launcher, o di un programma nella cartella del launcher;
//   - sta nella stessa cartella del launcher o in una sua sottocartella;
//   - il nome del gioco senza "Launcher" ("hytale") è una cartella del suo percorso o l'inizio
//     del nome del programma ("hytaleclient.exe");
//   - ha lo stesso editore del launcher (dalle informazioni del file), se non è un editore di
//     programmi qualsiasi (Microsoft, Google…).
// Qui la decisione, senza Windows: i dati del processo li raccoglie launcher_windows.go.

import (
	"strings"
	"unicode"
)

// datiProcesso: quello che serve sapere del programma in primo piano (percorsi in minuscolo).
type datiProcesso struct {
	Percorso string
	Editore  string   // CompanyName del file ("" = non lo so)
	Antenati []string // percorsi dei processi padre, nonno… (quelli ancora aperti)
}

// editori di tanti programmi diversi: lo stesso editore non vuol dire lo stesso gioco
var editoriGenerici = map[string]bool{
	"microsoft corporation": true, "microsoft": true, "google llc": true, "google inc.": true, "google": true,
	"mozilla corporation": true, "mozilla foundation": true, "nvidia corporation": true, "advanced micro devices, inc.": true,
	"amd": true, "intel corporation": true, "intel(r) corporation": true, "apple inc.": true, "oracle corporation": true,
	"python software foundation": true, "github, inc.": true, "the qt company ltd.": true, "valve": true, "valve corporation": true,
	"epic games, inc.": true, "unity technologies aps": true, "unity technologies": true, "electron": true, "github": true,
	"brave software, inc.": true, "opera norway as": true, "discord inc.": true, "obs project": true,
}

// cartellaDiProgramma: la cartella del programma, se non è troppo larga (radice del disco,
// Program Files, AppData…): lì dentro ci sono tanti programmi diversi.
func cartellaDiProgramma(exe string) string {
	d := strings.ToLower(strings.TrimRight(dirW(strings.ReplaceAll(exe, "/", `\`)), `\`))
	if strings.Count(d, `\`) < 2 {
		return ""
	}
	base := baseW(d)
	switch base {
	case "program files", "program files (x86)", "appdata", "local", "roaming", "programdata", "desktop", "downloads",
		"documents", "documenti", "bin", "binaries", "win64", "win32", "x64", "x86", "games", "giochi", "users", "programs":
		return ""
	}
	return d + `\`
}

// radiceNome: il nome del gioco senza "Launcher", "Client" e simili, solo lettere e numeri
// in minuscolo ("Hytale Launcher" → "hytale"). "" se troppo corto per fidarsi.
func radiceNome(nome string) string {
	var b strings.Builder
	for _, p := range strings.Fields(senzaLauncher(nome)) {
		for _, r := range strings.ToLower(p) {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				b.WriteRune(r)
			}
		}
	}
	if len([]rune(b.String())) < 4 {
		return ""
	}
	return b.String()
}

// sembraLauncher: la voce della libreria è un launcher (nel nome o nel programma).
func sembraLauncher(v VoceLibreria) bool {
	return strings.Contains(strings.ToLower(v.Nome), "launcher") || strings.Contains(strings.ToLower(baseW(v.Exe)), "launcher")
}

// giocoDaLibreria: il programma p è il gioco di una voce della libreria (anche se in libreria
// c'è il suo launcher). Restituisce il nome della voce.
func giocoDaLibreria(p datiProcesso, voci []VoceLibreria, editoreDi func(string) string) (string, bool) {
	perc := strings.ToLower(strings.ReplaceAll(p.Percorso, "/", `\`))
	if perc == "" {
		return "", false
	}
	exeBase := strings.TrimSuffix(baseW(perc), ".exe")
	for _, v := range voci {
		ve := strings.ToLower(strings.ReplaceAll(v.Exe, "/", `\`))
		if ve == "" {
			continue
		}
		if perc == ve {
			return v.Nome, true
		}
		cart := cartellaDiProgramma(ve)
		// 1. figlio del launcher (o di un programma nella sua cartella)
		for _, a := range p.Antenati {
			a = strings.ToLower(a)
			if a == ve || (cart != "" && strings.HasPrefix(a, cart)) {
				return v.Nome, true
			}
		}
		// 2. stessa cartella o sottocartella
		if cart != "" && strings.HasPrefix(perc, cart) {
			return v.Nome, true
		}
		// 3. il nome del gioco è una cartella del percorso o l'inizio del nome del programma
		if r := radiceNome(v.Nome); r != "" && (sembraLauncher(v) || strings.HasPrefix(exeBase, r)) {
			for _, c := range strings.Split(dirW(perc), `\`) {
				if pulisciNomeCartella(c) == r {
					return v.Nome, true
				}
			}
			if strings.HasPrefix(pulisciNomeCartella(exeBase), r) {
				return v.Nome, true
			}
		}
		// 4. stesso editore (non generico)
		if editoreDi != nil && p.Editore != "" {
			ed := strings.ToLower(strings.TrimSpace(p.Editore))
			if !editoriGenerici[ed] && ed == strings.ToLower(strings.TrimSpace(editoreDi(v.Exe))) {
				return v.Nome, true
			}
		}
	}
	return "", false
}

// pulisciNomeCartella: solo lettere e numeri, in minuscolo ("Hytale Launcher" → "hytalelauncher").
func pulisciNomeCartella(c string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(c) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// dirW e baseW: cartella e nome di un percorso di Windows (anche nelle prove su Linux).
func dirW(p string) string {
	if i := strings.LastIndex(p, `\`); i >= 0 {
		return p[:i]
	}
	return ""
}

func baseW(p string) string {
	return p[strings.LastIndex(p, `\`)+1:]
}
