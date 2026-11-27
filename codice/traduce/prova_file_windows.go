//go:build windows && collaudo

package traduce

// Prova dal vivo della traduzione dentro il gioco: installo, apro il gioco, aspetto, fotografo lo
// schermo ogni tanto, chiudo il gioco. Le mod restano installate (si tolgono con --togli-file).

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	provaFileCartella string
	provaFileNome     string
	provaFileUscita   string
	provaFileSecondi  = 240
	togliFileCartella string
)

func init() {
	for i, a := range os.Args {
		if i+1 >= len(os.Args) {
			break
		}
		switch a {
		case "--prova-file":
			provaFileCartella = os.Args[i+1]
		case "--nome-gioco":
			provaFileNome = os.Args[i+1]
		case "--uscita":
			provaFileUscita = os.Args[i+1]
		case "--secondi":
			fmt.Sscan(os.Args[i+1], &provaFileSecondi)
		case "--togli-file":
			togliFileCartella = os.Args[i+1]
		}
	}
}

func provaFileSeServe() bool {
	if togliFileCartella != "" {
		go func() {
			defer func() { wv.Dispatch(func() { wv.Terminate() }) }()
			e := fermaModGioco(togliFileCartella)
			impostaGiocoTradotto(togliFileCartella, false)
			_ = os.WriteFile(filepath.Join(dirApp, "logs", "togli-file.txt"), []byte("esito: '"+e+"'\r\n"), 0o644)
		}()
		return true
	}
	if provaFileCartella == "" {
		return false
	}
	go eseguiProvaFile()
	return true
}

func eseguiProvaFile() {
	defer guard("prova-file")
	defer func() { wv.Dispatch(func() { wv.Terminate() }) }()
	_ = os.MkdirAll(provaFileUscita, 0o755)
	var note []string
	nota := func(s string) {
		note = append(note, time.Now().Format("15:04:05")+" "+s)
		_ = os.WriteFile(filepath.Join(provaFileUscita, "prova.txt"), []byte(strings.Join(note, "\r\n")+"\r\n"), 0o644)
	}
	if lin := os.Getenv("TASSO_COPIA_LINGUA"); lin != "" {
		// diagnosi: metto nel file aggiunto i testi ufficiali di un'altra lingua del gioco al
		// posto dell'inglese. Se il gioco (in inglese) li mostra, il file aggiunto viene caricato
		nota("copia lingua " + lin + " al posto dell'inglese: " + copiaLinguaComeInglese(provaFileCartella, lin))
	} else if _, ok := leggiManifesto(baseDi(chiaveGioco(provaFileCartella)), chiaveGioco(provaFileCartella)); !ok {
		t0 := time.Now()
		applicaModGioco(provaFileNome, provaFileCartella)
		_, ok := leggiManifesto(baseDi(chiaveGioco(provaFileCartella)), chiaveGioco(provaFileCartella))
		nota(fmt.Sprintf("installazione: %v in %.1f s", ok, time.Since(t0).Seconds()))
		if !ok {
			return
		}
	}
	m, _ := leggiManifesto(baseDi(chiaveGioco(provaFileCartella)), chiaveGioco(provaFileCartella))
	nota("aggiunti: " + strings.Join(m.Aggiunti, ", ") + " | sostituiti: " + strings.Join(m.Sostituiti, ", "))
	// i giochi Steam si aprono da Steam: lanciando l'exe Steam chiede conferma con una finestra
	// "parametri personalizzati" che aspetta un clic (visto con PC Cafe Simulator, 24/11)
	if url := avvioSteamDi(m.Cartella); url != "" {
		if err := exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start(); err != nil {
			nota("avvio gioco: " + err.Error())
			return
		}
		nota("gioco avviato da Steam: " + url)
	} else {
		cmd := exec.Command(filepath.Join(m.Cartella, m.Exe))
		cmd.Dir = m.Cartella
		if err := cmd.Start(); err != nil {
			nota("avvio gioco: " + err.Error())
			return
		}
		nota("gioco avviato")
	}
	shipping := ""
	if sh, _ := filepath.Glob(filepath.Join(m.Cartella, "*", "Binaries", "Win64", "*-Shipping.exe")); len(sh) > 0 {
		shipping = filepath.Base(sh[0])
	}
	aperto := func() bool { return processoAperto(m.Exe) || (shipping != "" && processoAperto(shipping)) }
	inizio := time.Now()
	n := 0
	for time.Since(inizio) < time.Duration(provaFileSecondi)*time.Second {
		time.Sleep(15 * time.Second)
		n++
		w, h := dimensioniSchermo()
		img, err := cattura(Zona{X: 0, Y: 0, W: w, H: h})
		if err == nil {
			salvaPNG(img, filepath.Join(provaFileUscita, fmt.Sprintf("gioco-%03ds.png", int(time.Since(inizio).Seconds()))))
		}
		nota(fmt.Sprintf("%3.0f s: richieste di traduzione dal gioco %d, gioco aperto %v", time.Since(inizio).Seconds(), modRichieste.Load(), aperto()))
		if !aperto() && time.Since(inizio) > 60*time.Second {
			break
		}
	}
	_ = exec.Command("taskkill", "/IM", m.Exe, "/T", "/F").Run()
	if shipping != "" {
		_ = exec.Command("taskkill", "/IM", shipping, "/T", "/F").Run()
	}
	nota("gioco chiuso")
	time.Sleep(3 * time.Second)
}

func dimensioniSchermo() (int, int) {
	w, _, _ := pGetSysMetrics.Call(0)
	h, _, _ := pGetSysMetrics.Call(1)
	return int(w), int(h)
}

// avvioSteamDi: "steam://rungameid/<id>" se la cartella è un gioco di Steam (steamapps\common\X).
func avvioSteamDi(cartella string) string {
	common := filepath.Dir(cartella)
	if !strings.EqualFold(filepath.Base(common), "common") {
		return ""
	}
	acf, _ := filepath.Glob(filepath.Join(filepath.Dir(common), "appmanifest_*.acf"))
	for _, f := range acf {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		d, id := reAcfDir.FindSubmatch(b), reAcfID.FindSubmatch(b)
		if d != nil && id != nil && strings.EqualFold(string(d[1]), filepath.Base(cartella)) {
			return "steam://rungameid/" + string(id[1])
		}
	}
	return ""
}

func copiaLinguaComeInglese(cartella, lin string) string {
	// anche in collaudo: mai file in un gioco con anti-cheat, mai sopra una traduzione già messa
	if haAntiCheat(cartella) {
		return "gioco con anti-cheat: niente file"
	}
	paks := CartellaPaksUnreal(cartella)
	if paks == "" {
		return "non è un gioco Unreal (niente cartella Paks)"
	}
	if m, _ := filepath.Glob(filepath.Join(paks, "~mods", "*Tasso*")); len(m) > 0 {
		return "c'è già una traduzione di Tasso: toglila prima"
	}
	files, err := leggiLocresLingua(paks, lin)
	if err != nil {
		return err.Error()
	}
	var tenuti []LocresDaTradurre
	for _, f := range files {
		if strings.HasPrefix(strings.ToLower(f.Nome), "engine/") {
			continue
		}
		parti := strings.Split(f.Nome, "/")
		parti[len(parti)-2] = "en"
		f.Nome = strings.Join(parti, "/")
		tenuti = append(tenuti, f)
	}
	m := ManifestoMod{Gioco: provaFileNome, Cartella: cartella, Tipo: "unreal", Exe: exeGioco(cartella), Quando: time.Now(), Lingue: [2]string{"en", lin}}
	rel, _ := filepath.Rel(cartella, filepath.Join(paks, "~mods", nomeModUnreal))
	m.Aggiunti = []string{rel}
	m.CreataMods = !esisteVoce(filepath.Join(paks, "~mods"))
	if err := scriviManifesto(baseCopie(), chiaveGioco(cartella), m); err != nil {
		return err.Error()
	}
	if _, _, err := ScriviModUnreal(cartella, paks, tenuti); err != nil {
		_ = TogliModGioco(baseDi(chiaveGioco(cartella)), chiaveGioco(cartella))
		return err.Error()
	}
	var nomi []string
	for _, f := range tenuti {
		nomi = append(nomi, f.Nome)
	}
	return "ok: " + strings.Join(nomi, ", ")
}
