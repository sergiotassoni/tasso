package main

// Tasso gira da amministratore. Pagine web, giochi e file scaricati invece devono partire come
// programmi normali: li faccio aprire a Esplora file, che gira senza permessi di amministratore.

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// explorerExe: Esplora file dalla vera cartella di Windows (chiesta a Windows, non presa da SystemRoot).
func explorerExe() string { return filepath.Join(winDir(), "explorer.exe") }

// isWebURL: indirizzi che si possono aprire con il browser (o con il launcher del gioco).
func isWebURL(u string) bool {
	l := strings.ToLower(u)
	for _, p := range []string{"https://", "http://", "steam://", "com.epicgames.launcher://", "riotclient://", "battlenet://", "uplay://", "origin2://", "goggalaxy://", "ms-windows-store://", "ms-settings:", "discord://", "mailto:", "amazon-games://", `shell:appsfolder\`} {
		if strings.HasPrefix(l, p) {
			return !strings.ContainsAny(u, "\"\r\n")
		}
	}
	return false
}

// openUser apre un file, una cartella o un programma come utente normale.
// Con argomenti passa da un collegamento temporaneo, perché Esplora file non accetta argomenti.
func openUser(p, args string) bool {
	if strings.ContainsAny(p, "\"\r\n") {
		return false
	}
	if args == "" {
		return openPath(explorerExe(), "\""+p+"\"")
	}
	if !fileExists(p) {
		return false
	}
	// Il collegamento lo preparo in sottofondo (ci vuole un secondo): la finestra di Tasso non si blocca.
	go func() {
		defer guard("openuser.go:39")
		// 3.1: se il collegamento non si crea, NON apro il programma da amministratore (prima succedeva):
		// riprovo nella cartella di lavoro di Tasso in Programmi, poi lascio perdere.
		var lnk string
		for _, dir := range []string{filepath.Join(appDir, "tmp"), scriptDir()} {
			_ = os.MkdirAll(dir, 0o755)
			try := filepath.Join(dir, "apri-"+time.Now().Format("150405.000")+".lnk")
			r := runPSArgs(lnkScript, map[string]interface{}{"lnk": try, "t": p, "a": args, "w": filepath.Dir(p)}, time.Minute, nil)
			if r.Err == nil && fileExists(try) {
				lnk = try
				break
			}
		}
		if lnk == "" {
			logPanic("openuser", "collegamento non creato per "+p, nil)
			return
		}
		openPath(explorerExe(), "\""+lnk+"\"")
		time.Sleep(time.Minute)
		_ = os.Remove(lnk)
	}()
	return true
}

const lnkScript = `
$sh = New-Object -ComObject WScript.Shell
$l = $sh.CreateShortcut([string]$A.lnk)
$l.TargetPath = [string]$A.t
$l.Arguments = [string]$A.a
$l.WorkingDirectory = [string]$A.w
$l.Save()
`
