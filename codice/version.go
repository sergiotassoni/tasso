package main

// Controllo aggiornamenti: Tasso legge il file VERSION sul sito del progetto e dice se c'è
// una versione più nuova di quella attuale. L'aggiornamento parte solo se lo chiedi tu (selfupdate.go).

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const versionURL = "https://raw.githubusercontent.com/sergiotassoni/tasso/main/VERSION"
const downloadURL = "https://github.com/sergiotassoni/tasso/raw/main/Tasso.zip"

// Contatto: niente più server Discord di Tasso (deciso il 4/11/2026), solo email.
const discordURL = "mailto:tasso.services@proton.me?subject=Tasso"

// parseVer trasforma "2.3" in numeri confrontabili (2,3).
func parseVer(s string) []int {
	var out []int
	for _, p := range strings.Split(strings.TrimSpace(s), ".") {
		n, _ := strconv.Atoi(strings.TrimSpace(p))
		out = append(out, n)
	}
	return out
}

// verNewer dice se a è più recente di b.
func verNewer(a, b string) bool {
	av, bv := parseVer(a), parseVer(b)
	for i := 0; i < len(av) || i < len(bv); i++ {
		x, y := 0, 0
		if i < len(av) {
			x = av[i]
		}
		if i < len(bv) {
			y = bv[i]
		}
		if x != y {
			return x > y
		}
	}
	return false
}

func checkUpdate() map[string]interface{} {
	res := map[string]interface{}{"current": appVersion, "download": downloadURL}
	client := &http.Client{Timeout: 12 * time.Second, Transport: &http.Transport{Proxy: http.ProxyFromEnvironment}}
	req, _ := http.NewRequest("GET", versionURL, nil)
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := client.Do(req)
	if err != nil {
		res["error"] = "Non riesco a controllare gli aggiornamenti: controlla la connessione."
		return res
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		res["error"] = "Controllo aggiornamenti non disponibile ora, riprova più tardi."
		return res
	}
	buf, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
	latest := strings.TrimSpace(string(buf))
	// Solo cifre e punti, così una pagina di errore non diventa un "numero di versione".
	if latest == "" || len(latest) > 20 {
		res["error"] = "Controllo aggiornamenti non riuscito."
		return res
	}
	for _, c := range latest {
		if (c < '0' || c > '9') && c != '.' {
			res["error"] = "Controllo aggiornamenti non riuscito."
			return res
		}
	}
	res["latest"] = latest
	res["newer"] = verNewer(latest, appVersion)
	return res
}
