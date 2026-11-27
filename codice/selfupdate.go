package main

// Aggiornamento di Tasso con un clic: scarica Tasso.zip dal progetto su GitHub, controlla
// l'impronta SHA-256 pubblicata accanto (Tasso.sha256), mette il nuovo Tasso.exe al posto
// di quello vecchio e riapre Tasso. Il vecchio resta come Tasso.old.exe e si cancella
// alla prossima apertura.

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"debug/pe"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"tasso/traduce"
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
	const maxFile = 80 << 20
	files := map[string][]byte{}
	for _, f := range zr.File {
		name := filepath.Base(f.Name)
		if name != "Tasso.exe" && !contains(updateExtras, name) {
			continue
		}
		// Un file più grande del previsto non lo taglio (verrebbe un Tasso.exe a metà): rifiuto tutto.
		if f.UncompressedSize64 > maxFile {
			updMsg("error", "Nel file scaricato c'è un file troppo grande: non aggiorno.")
			return
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		b, err := io.ReadAll(io.LimitReader(rc, maxFile+1))
		rc.Close()
		if len(b) > maxFile {
			updMsg("error", "Nel file scaricato c'è un file troppo grande: non aggiorno.")
			return
		}
		if err == nil {
			files[name] = b
		}
	}
	exe := files["Tasso.exe"]
	if len(exe) < 1<<20 || !bytes.HasPrefix(exe, []byte("MZ")) {
		updMsg("error", "Nel file scaricato manca Tasso.exe: non aggiorno.")
		return
	}
	// Stesso identico Tasso.exe di quello aperto (es. versione pubblicata senza cambiare il numero
	// dentro il programma): non lo reinstallo a ogni apertura.
	if me, err := os.Executable(); err == nil {
		if old, err := os.ReadFile(me); err == nil && sha256.Sum256(old) == sha256.Sum256(exe) {
			updMsg("error", "Hai già questa versione.")
			return
		}
	}
	// Da qui in poi non deve partire nient'altro: tra poco Tasso si chiude.
	updating.Store(true)
	if isRunning() || otherBusy("update") || traduce.StaInstallando() {
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
		updMsg("error", "Non posso scrivere nella cartella di Tasso. Scarica lo zip a mano: Impostazioni, \"oppure scarica lo zip\".")
		return
	}
	// Firma digitale: se questo Tasso è firmato, il nuovo deve essere firmato anche lui, con lo
	// stesso nome. Così, anche se qualcuno entrasse nel progetto su GitHub e cambiasse zip e impronta,
	// un Tasso finto non verrebbe mai installato.
	// Il controllo della firma vale solo se questo Tasso è firmato: altrimenti non lo dico.
	if exeHasSignature(cur) {
		updMsg("work", "Controllo la firma digitale…")
	} else {
		updMsg("work", "Controllo che il file sia quello giusto…")
	}
	// la firma la controllo su una copia in Programmi (solo gli amministratori ci scrivono): il file
	// nella cartella di Tasso un altro programma potrebbe scambiarlo durante il controllo
	verifica, err := copiaPerVerifica(exe)
	if err != nil {
		_ = os.Remove(newPath)
		updating.Store(false)
		updMsg("error", "Non riesco a preparare il controllo della firma: per sicurezza non aggiorno. Scarica lo zip a mano: Impostazioni, \"oppure scarica lo zip\".")
		return
	}
	msg := checkUpdateSignature(cur, verifica)
	_ = os.Remove(verifica)
	if msg != "" {
		_ = os.Remove(newPath)
		updating.Store(false)
		updMsg("error", msg)
		return
	}
	vecchio, _ := readSelfExe() // per controllare, se torno indietro, che rimetto proprio questo
	_ = os.Remove(oldPath)
	unlockSelfExe() // se no non potrei rinominarlo
	if err := os.Rename(cur, oldPath); err != nil {
		lockSelfExe()
		_ = os.Remove(newPath)
		updating.Store(false)
		updMsg("error", "Non riesco a sostituire Tasso.exe. Scarica lo zip a mano: Impostazioni, \"oppure scarica lo zip\".")
		return
	}
	if err := os.Rename(newPath, cur); err != nil {
		_ = os.Rename(oldPath, cur) // rimetto com'era
		lockSelfExe()
		_ = os.Remove(newPath)
		updating.Store(false)
		updMsg("error", rimessoMsg(vecchio, "Non riesco a sostituire Tasso.exe. Scarica lo zip a mano: Impostazioni, \"oppure scarica lo zip\"."))
		return
	}
	// Blocco subito il file nuovo e controllo che sia proprio quello scaricato e verificato: un
	// programma senza privilegi non deve poterlo cambiare tra il controllo della firma e l'avvio.
	locked := lockSelfExe()
	if b, err := readSelfExe(); !locked || err != nil || !bytes.Equal(b, exe) {
		unlockSelfExe()
		_ = os.Remove(cur)
		_ = os.Rename(oldPath, cur)
		lockSelfExe()
		updating.Store(false)
		updMsg("error", rimessoMsg(vecchio, "Il file nuovo è cambiato durante l'aggiornamento: per sicurezza non lo apro. Scarica lo zip a mano: Impostazioni, \"oppure scarica lo zip\"."))
		return
	}
	// Anche la copia in Programmi (quella delle attività pianificate) passa alla versione nuova,
	// così le due copie non si rimettono a vicenda la versione vecchia.
	if pf := programFilesDir(); pf != "" {
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
	// Aspetto che finisca un eventuale cambio di modalità, così non chiudo Tasso a metà di uno script.
	// 3.1: lo aspetto PRIMA di aprire la copia nuova, che aspetta la vecchia al massimo 2 minuti:
	// se il cambio durava di più, la nuova si arrendeva e Tasso restava chiuso.
	modeMu.Lock()
	cmd := exec.Command(cur, "--after-update")
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		modeMu.Unlock()
		updating.Store(false)
		updMsg("error", "Aggiornato, ma non riesco a riaprirlo: chiudi Tasso e riaprilo tu.")
		return
	}
	wv.Dispatch(func() { wv.Terminate() })
}

// cleanOldExe toglie il Tasso.old.exe lasciato dall'aggiornamento (riprova per qualche secondo:
// la copia vecchia potrebbe non essersi ancora chiusa del tutto).
func cleanOldExe() {
	// resti in Programmi di un controllo della firma o di un "Togli Tasso" interrotti (più vecchi di
	// 10 minuti: uno in corso adesso non lo tocco)
	if pf := programFilesDir(); pf != "" {
		for _, pat := range []string{"tasso-verifica-*.exe", "togli-tasso-*.cmd"} {
			m, _ := filepath.Glob(filepath.Join(pf, pat))
			for _, f := range m {
				if fi, err := os.Lstat(f); err == nil && fi.Mode().IsRegular() && time.Since(fi.ModTime()) > 10*time.Minute {
					_ = os.Remove(f)
				}
			}
		}
	}
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

// exeHasSignature: il file ha una firma Authenticode dentro (non dice se è valida: serve solo
// per scegliere il messaggio giusto mentre controllo).
func exeHasSignature(p string) bool {
	f, err := pe.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	const security = 4 // IMAGE_DIRECTORY_ENTRY_SECURITY
	switch h := f.OptionalHeader.(type) {
	case *pe.OptionalHeader64:
		return h.NumberOfRvaAndSizes > security && h.DataDirectory[security].Size > 0
	case *pe.OptionalHeader32:
		return h.NumberOfRvaAndSizes > security && h.DataDirectory[security].Size > 0
	}
	return false
}

type sigInfo struct {
	Status  string `json:"status"`
	Subject string `json:"subject"`
}

// sigFull: la firma più chi ha emesso il certificato (un pezzo del nome per riga).
type sigFull struct {
	sigInfo
	Issuer []string `json:"issuer"`
}

const sigCheckScript = `
function Sig([string]$p) {
  $s = Get-AuthenticodeSignature -LiteralPath $p -ErrorAction SilentlyContinue
  if (-not $s) { return [ordered]@{ status = 'Error'; subject = ''; issuer = @() } }
  $sub = ''; $iss = @()
  if ($s.SignerCertificate) {
    $sub = [string]$s.SignerCertificate.Subject
    # Un pezzo per riga (le virgole dentro le virgolette non contano): niente trucchi col nome.
    $iss = @(($s.SignerCertificate.IssuerName.Decode([Security.Cryptography.X509Certificates.X500DistinguishedNameFlags]::UseNewLines)) -split "\r?\n" | Where-Object { $_ })
  }
  return [ordered]@{ status = [string]$s.Status; subject = $sub; issuer = $iss }
}
$o = [ordered]@{ cur = (Sig ([string]$A.cur)); new = (Sig ([string]$A.new)) }
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject $o -Compress -Depth 4))
`

// checkUpdateSignature restituisce "" se il nuovo Tasso.exe si può installare, altrimenti il motivo.
func checkUpdateSignature(cur, newExe string) string {
	r := runPSArgs(sigCheckScript, map[string]string{"cur": cur, "new": newExe}, 2*time.Minute, nil)
	var res struct {
		Cur sigFull `json:"cur"`
		New sigFull `json:"new"`
	}
	if r.JSON == "" || json.Unmarshal([]byte(r.JSON), &res) != nil {
		return "Non riesco a controllare la firma digitale del nuovo Tasso: per sicurezza non aggiorno. Scarica lo zip a mano: Impostazioni, \"oppure scarica lo zip\"."
	}
	if msg := sigDecision(res.Cur.sigInfo, res.New.sigInfo); msg != "" {
		return msg
	}
	return issuerDecision(res.Cur.Issuer, res.New.Issuer)
}

// issuerDecision: se il certificato di questo Tasso è di Certum, anche quello del nuovo deve
// esserlo (lo stesso nome con un certificato di un'altra autorità non basta). Se questo Tasso non
// è firmato da Certum non aggiungo niente: decide sigDecision.
func issuerDecision(cur, nw []string) string {
	if !isCertum(cur) || isCertum(nw) {
		return ""
	}
	return "Il nuovo Tasso è firmato con un certificato di un'altra autorità: per sicurezza non lo installo."
}

func isCertum(rdn []string) bool {
	for _, x := range rdn {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(x)), "cn=certum") {
			return true
		}
	}
	return false
}

// sigDecision: la regola, separata per poterla provare.
func sigDecision(cur, nw sigInfo) string {
	if cur.Subject == "" {
		return "" // questo Tasso non è firmato (versione di prova): non c'è una firma da confrontare
	}
	if cur.Status != "Valid" {
		// Firmato, ma Windows ora non riesce a verificare la firma (certificati non aggiornati):
		// senza un controllo vero non installo niente.
		return "Windows non riesce a verificare la firma di questo Tasso: per sicurezza non aggiorno da solo. Scarica lo zip a mano: Impostazioni, \"oppure scarica lo zip\"."
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

// copiaFirmata: questi byte sono un Tasso con una firma valida di Sergio Tassoni, rilasciata da
// Certum (controllata su una copia nuova in Programmi, dove nessun altro programma può cambiarla).
func copiaFirmata(exe []byte) bool {
	p, err := copiaPerVerifica(exe)
	if err != nil {
		return false
	}
	defer os.Remove(p)
	r := runPSArgs(sigCheckScript, map[string]string{"cur": p, "new": p}, 2*time.Minute, nil)
	var res struct {
		New sigFull `json:"new"`
	}
	if r.JSON == "" || json.Unmarshal([]byte(r.JSON), &res) != nil {
		return false
	}
	return res.New.Status == "Valid" && isCertum(res.New.Issuer) && strings.Contains(res.New.Subject, "Sergio Tassoni")
}

// copiaPerVerifica: il nuovo Tasso in un file di Programmi nuovo di zecca (O_EXCL), per la firma.
func copiaPerVerifica(exe []byte) (string, error) {
	pf := programFilesDir()
	if pf == "" {
		return "", os.ErrNotExist
	}
	p := filepath.Join(pf, "tasso-verifica-"+strconv.Itoa(os.Getpid())+".exe")
	_ = os.Remove(p) // resto di un aggiornamento interrotto (in Programmi lo scrive solo un amministratore)
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	_, err = f.Write(exe)
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		_ = os.Remove(p)
		return "", err
	}
	return p, nil
}

// rimessoMsg: dopo aver rimesso il vecchio Tasso controllo che sia proprio quello di prima (la
// cartella di Tasso può essere scrivibile da altri programmi).
func rimessoMsg(vecchio []byte, msg string) string {
	if b, err := readSelfExe(); len(vecchio) == 0 || err != nil || !bytes.Equal(b, vecchio) {
		// non mi fido del file: lo sblocco, così readSelfExe fallisce e non viene mai copiato in Programmi
		unlockSelfExe()
		return "Aggiornamento non riuscito e il file di Tasso non è più quello di prima: chiudi Tasso e riscarica lo zip da sergiotassoni.github.io/tasso."
	}
	return msg
}
