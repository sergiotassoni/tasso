package main

// Scritte "NUOVO" delle novità della versione (3.2): si vedono per 14 giorni dal primo avvio della
// versione e poi spariscono da sole. La data si salva in settings.json (novitaDal) con accanto la
// versione a cui è legata (novitaVer, solo "maggiore.minore": un 3.2.1 di correzioni non fa
// ripartire il conto). Se c'è già non cambia: riaprire, reinstallare o aggiornare alla stessa
// versione non la azzera. Una versione più nuova (3.3) riparte con la sua data.

import (
	"strings"
	"time"
)

const giorniNovita = 14

// versioneNovita: "3.2.1" -> "3.2".
func versioneNovita(v string) string {
	p := strings.Split(strings.TrimSpace(v), ".")
	if len(p) > 2 {
		p = p[:2]
	}
	return strings.Join(p, ".")
}

// segnaNovita mette la data delle novità se manca, se non si legge o se la versione è più nuova
// di quella salvata. True se ha cambiato qualcosa (da salvare).
func segnaNovita(s *Settings, ver string, oggi time.Time) bool {
	ver = versioneNovita(ver)
	if s.NovitaVer != "" && !verNewer(ver, s.NovitaVer) {
		if _, err := time.Parse("2006-01-02", s.NovitaDal); err == nil {
			return false
		}
	}
	s.NovitaVer = ver
	s.NovitaDal = oggi.Format("2006-01-02")
	return true
}

// novitaFino: il primo giorno (AAAA-MM-GG) in cui le scritte "NUOVO" non si vedono più ("" se la
// data non c'è: la pagina allora non le mostra).
func novitaFino(s *Settings) string {
	d, err := time.Parse("2006-01-02", s.NovitaDal)
	if err != nil {
		return ""
	}
	return d.AddDate(0, 0, giorniNovita).Format("2006-01-02")
}

func novitaFinoOra() string {
	setMu.Lock()
	defer setMu.Unlock()
	return novitaFino(&settings)
}
