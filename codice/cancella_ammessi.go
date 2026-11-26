package main

// Elenco delle cartelle dove la cancellazione sicura (cancella_sicura.go) può lavorare, deciso qui in
// Go e non dagli script. Prima bastava che la cartella non fosse una giunzione: se AppData\Local era
// rediretta (lo può fare un programma qualsiasi in HKCU\...\User Shell Folders) verso una cartella di
// sistema, ogni "svuota la cache in AppData" diventava una cancellazione da amministratore là dentro.
// Adesso il percorso VERO (dopo le giunzioni) deve stare in una di queste cartelle.
//
// Questa parte non usa Windows: si prova anche su Linux (cancella_ammessi_test.go).

import (
	"regexp"
	"strings"
)

type cartelleAmmesse struct {
	// Intere: si possono svuotare loro stesse e tutto quello che c'è sotto (Windows\Temp, C:\NVIDIA...).
	intere []string
	// Solo dentro: si può lavorare sotto, mai sulla cartella stessa (AppData\Local, AppData\Roaming).
	dentro []string
}

// Cestino: X:\$Recycle.Bin\<SID dell'utente>\<elemento> (la pulizia toglie gli elementi vecchi).
var reCestino = regexp.MustCompile(`(?i)^[a-z]:\\\$recycle\.bin\\s-1-[0-9-]+\\[^\\]+(\\.*)?$`)

// percorsoAmmesso: p (percorso vero, senza \ finale) sta in una delle cartelle ammesse?
func percorsoAmmesso(p string, a cartelleAmmesse) bool {
	p = strings.TrimRight(strings.TrimSpace(p), `\`)
	if len(p) <= 3 || strings.Contains(p, `\..`) {
		return false
	}
	for _, d := range a.intere {
		if d = strings.TrimRight(d, `\`); len(d) > 3 && dentroDi(p, d) {
			return true
		}
	}
	for _, d := range a.dentro {
		if d = strings.TrimRight(d, `\`); len(d) > 3 && dentroDi(p, d) && !strings.EqualFold(p, d) {
			return true
		}
	}
	// Download di Steam rimasti a metà, in qualunque libreria (anche D:\SteamLibrary): solo quello
	// che sta DENTRO steamapps\downloading, mai la cartella stessa.
	lp := strings.ToLower(p)
	if i := strings.Index(lp, `\steamapps\downloading\`); i > 2 && len(lp) > i+len(`\steamapps\downloading\`) {
		return true
	}
	return reCestino.MatchString(p)
}
