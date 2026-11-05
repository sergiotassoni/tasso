package main

// Aggiornamento di Tasso con un clic: scarica Tasso.zip dal progetto su GitHub, controlla
// l'impronta SHA-256 pubblicata accanto (Tasso.sha256), mette il nuovo Tasso.exe al posto
// di quello vecchio e riapre Tasso. Il vecchio resta come Tasso.old.exe e si cancella
// alla prossima apertura.

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const shaURL = "https://raw.githubusercontent.com/sergiotassoni/tasso/main/Tasso.sha256"

// File che, se ci sono accanto a Tasso.exe, vengono aggiornati insieme a lui.
var updateExtras = []string{"Manuale_Tasso.pdf", "LEGGIMI.txt", "INDICE.txt", "LICENZE.txt", "Manual_Tasso_EN.pdf", "README.txt", "INDEX.txt", "LICENSES.txt"}

func updMsg(stage, msg string) {
	emit("onSelfUpdate", map[string]interface{}{"stage": stage, "msg": msg})
}

func httpGet(url string, max int64, timeout time.Duration) ([]byte, error) {
	client := &http.Client{Timeout: timeout, Transport: &http.Transport{Proxy: http.ProxyFromEnvironment}}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("risposta %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("file troppo grande")
	}
	return b, nil
}

func selfUpdate() {
	if isRunning() {
		updMsg("error", "Sta girando una pulizia: aspetta che finisca, poi aggiorna.")
		return
	}
	u := checkUpdate()
	if e, ok := u["error"].(string); ok && e != "" {
		updMsg("error", e)
		return
	}
	if n, _ := u["newer"].(bool); !n {
		updMsg("error", "Hai già l'ultima versione.")
		return
	}
	latest, _ := u["latest"].(string)
	updMsg("work", "Scarico Tasso "+latest+"…")
	sum, err := httpGet(shaURL, 200, 20*time.Second)
	if err != nil {
		updMsg("error", "Non riesco a scaricare l'aggiornamento: controlla la connessione e riprova.")
		return
	}
	f := strings.Fields(string(sum) + " x")
	want := strings.ToLower(strings.TrimSpace(f[0]))
	// Formato: "impronta  Tasso.zip  versione". Se la versione non è ancora quella nuova,
	// il sito non ha finito di aggiornarsi: meglio aspettare che reinstallare la vecchia.
	if len(f) >= 4 && f[2] != latest {
		updMsg("error", "Il sito sta ancora preparando la versione "+latest+": riprova tra qualche minuto.")
		return
	}
	data, err := httpGet(downloadURL, 80<<20, 5*time.Minute)
	if err != nil {
		updMsg("error", "Download non riuscito: controlla la connessione e riprova.")
		return
	}
	h := sha256.Sum256(data)
	if hex.EncodeToString(h[:]) != want {
		updMsg("error", "Il file scaricato non è quello giusto (impronta diversa): non lo uso. Riprova tra qualche minuto.")
		return
	}
	updMsg("work", "Controllo il file scaricato…")
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		updMsg("error", "Il file scaricato è rovinato: riprova.")
		return
	}
	files := map[string][]byte{}
	for _, f := range zr.File {
		name := filepath.Base(f.Name)
		if name != "Tasso.exe" && !contains(updateExtras, name) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		b, err := io.ReadAll(io.LimitReader(rc, 80<<20))
		rc.Close()
		if err == nil {
			files[name] = b
		}
	}
	exe := files["Tasso.exe"]
	if len(exe) < 1<<20 || !bytes.HasPrefix(exe, []byte("MZ")) {
		updMsg("error", "Nel file scaricato manca Tasso.exe: non aggiorno.")
		return
	}
	// Da qui in poi non deve partire nient'altro: tra poco Tasso si chiude.
	updating.Store(true)
	if isRunning() || otherBusy("update") {
		updating.Store(false)
		updMsg("error", "Tasso sta facendo un altro lavoro: aspetta che finisca, poi aggiorna.")
		return
	}
	cur, err := os.Executable()
	if err != nil {
		updating.Store(false)
		updMsg("error", "Non trovo dove si trova Tasso.")
		return
	}
	dir := filepath.Dir(cur)
	newPath := filepath.Join(dir, "Tasso.new.exe")
	oldPath := strings.TrimSuffix(cur, filepath.Ext(cur)) + ".old.exe"
	updMsg("work", "Installo la nuova versione…")
	if err := os.WriteFile(newPath, exe, 0o755); err != nil {
		_ = os.Remove(newPath)
		updating.Store(false)
		updMsg("error", "Non posso scrivere nella cartella di Tasso. Scarica lo zip a mano da Info.")
		return
	}
	// Firma digitale: se questo Tasso è firmato, il nuovo deve essere firmato anche lui, con lo
	// stesso nome. Così, anche se qualcuno entrasse nel progetto su GitHub e cambiasse zip e impronta,
	// un Tasso finto non verrebbe mai installato.
	updMsg("work", "Controllo la firma digitale…")
	if msg := checkUpdateSignature(cur, newPath); msg != "" {
		_ = os.Remove(newPath)
		updating.Store(false)
		updMsg("error", msg)
		return
	}
	_ = os.Remove(oldPath)
	if err := os.Rename(cur, oldPath); err != nil {
		_ = os.Remove(newPath)
		updating.Store(false)
		updMsg("error", "Non riesco a sostituire Tasso.exe. Scarica lo zip a mano da Info.")
		return
	}
	if err := os.Rename(newPath, cur); err != nil {
		_ = os.Rename(oldPath, cur) // rimetto com'era
		_ = os.Remove(newPath)
		updating.Store(false)
		updMsg("error", "Non riesco a sostituire Tasso.exe. Scarica lo zip a mano da Info.")
		return
	}
	// Anche la copia in Programmi (quella delle attività pianificate) passa alla versione nuova,
	// così le due copie non si rimettono a vicenda la versione vecchia.
	if pf := os.Getenv("ProgramFiles"); pf != "" {
		pdir := filepath.Join(pf, "Tasso")
		if fileExists(filepath.Join(pdir, "Tasso.exe")) && !strings.EqualFold(filepath.Clean(dir), filepath.Clean(pdir)) {
			_ = installProgramCopy(pdir, exe, latest)
		} else if strings.EqualFold(filepath.Clean(dir), filepath.Clean(pdir)) {
			_ = os.WriteFile(filepath.Join(pdir, "VERSIONE"), []byte(latest), 0o644)
		}
	}
	for name, b := range files {
		if name == "Tasso.exe" {
			continue
		}
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			_ = os.WriteFile(p, b, 0o644)
		}
	}
	updMsg("done", "Fatto! Tasso "+latest+" si riapre tra un attimo.")
	time.Sleep(1500 * time.Millisecond)
	cmd := exec.Command(cur, "--after-update")
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		updating.Store(false)
		updMsg("error", "Aggiornato, ma non riesco a riaprirlo: chiudi Tasso e riaprilo tu.")
		return
	}
	// Aspetto che finisca un eventuale cambio di modalità, così non chiudo Tasso a metà di uno script.
	modeMu.Lock()
	wv.Dispatch(func() { wv.Terminate() })
}

// cleanOldExe toglie il Tasso.old.exe lasciato dall'aggiornamento (riprova per qualche secondo:
// la copia vecchia potrebbe non essersi ancora chiusa del tutto).
func cleanOldExe() {
	cur, err := os.Executable()
	if err != nil {
		return
	}
	old := strings.TrimSuffix(cur, filepath.Ext(cur)) + ".old.exe"
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(old); err != nil {
			return
		}
		if os.Remove(old) == nil {
			return
		}
		time.Sleep(2 * time.Second)
	}
}

type sigInfo struct {
	Status  string `json:"status"`
	Subject string `json:"subject"`
}

const sigCheckScript = `
function Sig([string]$p) {
  $s = Get-AuthenticodeSignature -LiteralPath $p -ErrorAction SilentlyContinue
  if (-not $s) { return [ordered]@{ status = 'Error'; subject = '' } }
  $sub = ''
  if ($s.SignerCertificate) { $sub = [string]$s.SignerCertificate.Subject }
  return [ordered]@{ status = [string]$s.Status; subject = $sub }
}
$o = [ordered]@{ cur = (Sig ([string]$A.cur)); new = (Sig ([string]$A.new)) }
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject $o -Compress -Depth 3))
`

// checkUpdateSignature restituisce "" se il nuovo Tasso.exe si può installare, altrimenti il motivo.
func checkUpdateSignature(cur, newExe string) string {
	r := runPSArgs(sigCheckScript, map[string]string{"cur": cur, "new": newExe}, 2*time.Minute, nil)
	var res struct {
		Cur sigInfo `json:"cur"`
		New sigInfo `json:"new"`
	}
	if r.JSON == "" || json.Unmarshal([]byte(r.JSON), &res) != nil {
		return "Non riesco a controllare la firma digitale del nuovo Tasso: per sicurezza non aggiorno. Scarica lo zip a mano da Info."
	}
	return sigDecision(res.Cur, res.New)
}

// sigDecision: la regola, separata per poterla provare.
func sigDecision(cur, nw sigInfo) string {
	if cur.Status != "Valid" || cur.Subject == "" {
		return "" // questo Tasso non è firmato (versione di prova): non c'è una firma da confrontare
	}
	if nw.Status != "Valid" {
		return "Il nuovo Tasso non ha una firma digitale valida: per sicurezza non lo installo."
	}
	if nw.Subject != cur.Subject {
		return "Il nuovo Tasso è firmato da qualcun altro: per sicurezza non lo installo."
	}
	return ""
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
