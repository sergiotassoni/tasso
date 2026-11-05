package main

// Tasso gira da amministratore. Pagine web, giochi e file scaricati invece devono partire come
// programmi normali: li faccio aprire a Esplora file, che gira senza permessi di amministratore.

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

func explorerExe() string { return filepath.Join(os.Getenv("SystemRoot"), "explorer.exe") }

// isWebURL: indirizzi che si possono aprire con il browser (o con il launcher del gioco).
func isWebURL(u string) bool {
	l := strings.ToLower(u)
	for _, p := range []string{"https://", "http://", "steam://", "com.epicgames.launcher://", "riotclient://", "battlenet://", "uplay://", "origin2://", "goggalaxy://", "ms-windows-store://", "ms-settings:", "discord://"} {
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
		dir := filepath.Join(appDir, "tmp")
		_ = os.MkdirAll(dir, 0o755)
		lnk := filepath.Join(dir, "apri-"+time.Now().Format("150405.000")+".lnk")
		r := runPSArgs(lnkScript, map[string]interface{}{"lnk": lnk, "t": p, "a": args, "w": filepath.Dir(p)}, time.Minute, nil)
		if r.Err != nil || !fileExists(lnk) {
			openPath(p, args) // ultima possibilità: meglio aprirlo che non aprirlo
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
