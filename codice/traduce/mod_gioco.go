package traduce

// Traduzione dentro il gioco (giochi Unity): invece di leggere lo schermo, il testo viene preso
// direttamente dal gioco e rimesso tradotto al suo posto, con il carattere del gioco.
// Strumenti liberi: BepInEx (LGPL-2.1) carica il componente XUnity.AutoTranslator (MIT), che
// chiede ogni scritta a Tasso Traduce sul PC (http://127.0.0.1, niente internet: i traduttori
// in rete di XUnity NON vengono installati).
//
// Regole di sicurezza:
//   - solo giochi Unity senza anti-imbroglio; se il gioco ha già delle mod non lo tocco;
//   - nessun file del gioco viene cambiato: aggiungo solo i file di BepInEx accanto all'exe. Se un
//     file dovesse esistere già, prima ne faccio una copia di sicurezza;
//   - un elenco di tutto quello che ho aggiunto (manifesto) permette a "Ferma traduzione" di
//     togliere tutto e rimettere il gioco esattamente com'era.

import (
	"archive/zip"
	"crypto/rand"
	"debug/pe"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const portaModGioco = 47321

var (
	zipBepInExIL2CPP = FileDaScaricare{
		URL:    "https://builds.bepinex.dev/projects/bepinex_be/793/BepInEx-Unity.IL2CPP-win-x64-6.0.0-be.793%2Bdbc82f0.zip",
		Nome:   "BepInEx-Unity.IL2CPP-win-x64-6.0.0-be.793.zip",
		SHA256: "5b63d7d0e92a2448ccd13b738ff95ff1cc1026c6d718555d4df8e6c824953ac4",
		Byte:   34339235,
	}
	zipXUnityIL2CPP = FileDaScaricare{
		URL:    "https://github.com/bbepis/XUnity.AutoTranslator/releases/download/v5.6.3.1/XUnity.AutoTranslator-BepInEx-IL2CPP-5.6.3.1.zip",
		Nome:   "XUnity.AutoTranslator-BepInEx-IL2CPP-5.6.3.1.zip",
		SHA256: "f1d4d3151a061edfcda815e4472fe1f1de8756bdc23708c9b5ebb9bfe736fb1f",
		Byte:   762471,
	}
	zipBepInExMono = FileDaScaricare{
		URL:    "https://github.com/BepInEx/BepInEx/releases/download/v5.4.23.6/BepInEx_win_x64_5.4.23.6.zip",
		Nome:   "BepInEx_win_x64_5.4.23.6.zip",
		SHA256: "6c57a714a892efb4e9e1fef63d608a3391f30ec984031c465404e4ee381a0755",
		Byte:   642076,
	}
	zipXUnityMono = FileDaScaricare{
		URL:    "https://github.com/bbepis/XUnity.AutoTranslator/releases/download/v5.6.3.1/XUnity.AutoTranslator-BepInEx-5.6.3.1.zip",
		Nome:   "XUnity.AutoTranslator-BepInEx-5.6.3.1.zip",
		SHA256: "6c3ff03aafe9708f25c2115c0c04c255b6a9d88e716896ae7bacb590a8777609",
		Byte:   759511,
	}
)

// TipoUnity: "il2cpp", "mono" o "" (non è un gioco Unity che so tradurre dai file).
func TipoUnity(cartella string) (tipo, exe string) {
	voci, err := os.ReadDir(cartella)
	if err != nil {
		return "", ""
	}
	for _, v := range voci {
		n := v.Name()
		if v.IsDir() || !strings.EqualFold(filepath.Ext(n), ".exe") || strings.EqualFold(n, "UnityCrashHandler64.exe") || strings.EqualFold(n, "UnityCrashHandler32.exe") {
			continue
		}
		base := strings.TrimSuffix(n, filepath.Ext(n))
		dati := filepath.Join(cartella, base+"_Data")
		if st, err := os.Stat(dati); err != nil || !st.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(cartella, "GameAssembly.dll")); err == nil {
			return "il2cpp", n
		}
		if _, err := os.Stat(filepath.Join(dati, "Managed", "Assembly-CSharp.dll")); err == nil {
			return "mono", n
		}
	}
	return "", ""
}

// ModGiaPresenti: il gioco ha già un caricatore di mod (non mescolo le mie con quelle dell'utente).
func ModGiaPresenti(cartella string) string {
	for _, n := range []string{"BepInEx", "winhttp.dll", "version.dll", "doorstop_config.ini", "MelonLoader", "dinput8.dll"} {
		if _, err := os.Stat(filepath.Join(cartella, n)); err == nil {
			return n
		}
	}
	return ""
}

// ManifestoMod: tutto quello che ho aggiunto o sostituito in un gioco.
type ManifestoMod struct {
	Gioco      string            `json:"gioco"`
	Cartella   string            `json:"cartella"`
	Tipo       string            `json:"tipo"`
	Exe        string            `json:"exe"`
	Quando     time.Time         `json:"quando"`
	Aggiunti   []string          `json:"aggiunti"`              // percorsi relativi alla cartella del gioco (file e cartelle in cima)
	Sostituiti []string          `json:"sostituiti"`            // file che esistevano: l'originale è nella copia di sicurezza
	Token      string            `json:"token"`                 // segreto nell'indirizzo: solo questo gioco può chiedere traduzioni
	Lingue     [2]string         `json:"lingue"`                // da → a
	CreataMods bool              `json:"creata_mods,omitempty"` // Unreal: la cartella ~mods l'ha creata Tasso
	AntiCheat  bool              `json:"anti_cheat,omitempty"`  // installata su un gioco con anti-cheat, per scelta dell'utente
	Impronte   map[string]string `json:"impronte,omitempty"`    // sha256 degli originali messi da parte
	Firma      string            `json:"firma,omitempty"`       // vedi firma_manifesto.go
}

func cartellaCopie(dirApp, chiave string) string {
	return filepath.Join(dirApp, "copie-giochi", chiave)
}

// leggiFileManifesto: i byte di manifesto.json di quel gioco in dirApp (cartella protetta o AppData),
// senza seguire collegamenti (leggiSenzaLink; nella cartella protetta basta il ripiego semplice).
func leggiFileManifesto(dirApp, chiave string) ([]byte, error) {
	rel := filepath.Join("copie-giochi", chiave, "manifesto.json")
	b, err := leggiSenzaLink(dirApp, rel)
	if errors.Is(err, errNonVerificato) && èCartellaProtetta(dirApp) {
		// Programmi\Tasso\lavoro: solo gli amministratori ci scrivono, quindi se Windows non sa
		// dire il percorso vero bastano i controlli con Lstat e os.SameFile
		b, err = leggiSenzaLinkSemplice(dirApp, rel)
	}
	return b, err
}

func leggiManifesto(dirApp, chiave string) (ManifestoMod, bool) {
	var m ManifestoMod
	b, err := leggiFileManifesto(dirApp, chiave)
	if err != nil || json.Unmarshal(b, &m) != nil {
		return m, false
	}
	if !manifestoFirmato(dirApp, m) {
		return m, false // scritto o cambiato da un altro programma: non lo uso
	}
	if !manifestoAffidabile(dirApp, m) {
		return m, false // in AppData: firmato ma senza il segno di Tasso nel gioco (D8 della 3.2)
	}
	return m, true
}

func scriviManifesto(dirApp, chiave string, m ManifestoMod) error {
	d := cartellaCopie(dirApp, chiave)
	if !copieSenzaLink(dirApp, chiave, "manifesto.json.tmp") || !copieSenzaLink(dirApp, chiave, "manifesto.json") {
		return errors.New("la cartella delle copie di sicurezza porta altrove (link o giunzione): per sicurezza non scrivo")
	}
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	if err := firmaManifesto(dirApp, chiave, &m); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	tmp := filepath.Join(d, "manifesto.json.tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(d, "manifesto.json"))
}

// voluto: quali file degli zip copio. Dei traduttori di XUnity tengo SOLO CustomTranslate
// (quello che parla con Tasso sul PC): Google, Bing, DeepL & co. non vengono installati.
func voluto(nome string) bool {
	n := strings.ReplaceAll(nome, `\`, "/")
	if strings.HasSuffix(n, "/") {
		return false
	}
	if i := strings.Index(n, "XUnity.AutoTranslator/Translators/"); i >= 0 {
		return strings.HasSuffix(n, "/CustomTranslate.dll") && !strings.Contains(n[i:], "FullNET")
	}
	base := strings.ToLower(n[strings.LastIndex(n, "/")+1:])
	if base == "changelog.txt" || strings.HasPrefix(base, "readme") {
		return false // non servono, e potrebbero coprire un file del gioco con lo stesso nome
	}
	return true
}

// estraiInGioco copia i file di uno zip nella cartella del gioco, ricordando cosa ha aggiunto.
// Un file che esiste già viene prima copiato nella copia di sicurezza.
func estraiInGioco(zipPath, cartella, copie string, m *ManifestoMod, inCima map[string]bool, salva func() error) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	base := filepath.Clean(cartella) + string(os.PathSeparator)
	// prima giro a vuoto: se qualcosa non va (percorso fuori dal gioco, cartella del gioco già
	// esistente) mi fermo PRIMA di toccare un solo file
	prova := map[string]bool{}
	for k, v := range inCima {
		prova[k] = v
	}
	for _, f := range r.File {
		if !voluto(f.Name) {
			continue
		}
		dest := filepath.Join(cartella, filepath.FromSlash(strings.ReplaceAll(f.Name, `\`, "/")))
		if !strings.HasPrefix(filepath.Clean(dest), base) {
			return fmt.Errorf("percorso non valido nello zip: %s", f.Name)
		}
		rel, _ := filepath.Rel(cartella, dest)
		primo := strings.Split(filepath.ToSlash(rel), "/")[0]
		if prova[primo] {
			continue
		}
		if _, err := os.Stat(dest); err == nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(cartella, primo)); err != nil {
			prova[primo] = true
		} else {
			return fmt.Errorf("il gioco ha già una cartella %s: per sicurezza non lo tocco", primo)
		}
	}
	for _, f := range r.File {
		if !voluto(f.Name) {
			continue
		}
		dest := filepath.Join(cartella, filepath.FromSlash(strings.ReplaceAll(f.Name, `\`, "/")))
		if !strings.HasPrefix(filepath.Clean(dest), base) { // niente file fuori dalla cartella del gioco
			return fmt.Errorf("percorso non valido nello zip: %s", f.Name)
		}
		rel, _ := filepath.Rel(cartella, dest)
		primo := strings.Split(filepath.ToSlash(rel), "/")[0]
		if !senzaLink(cartella, rel) || !senzaLink(copie, filepath.Join("originali", rel)) {
			// una giunzione nel gioco (o nelle copie) farebbe scrivere Tasso da amministratore altrove
			return fmt.Errorf("nel percorso di %s c'è un link o una giunzione: per sicurezza non lo tocco", rel)
		}
		if _, err := os.Stat(dest); err == nil {
			// esiste già: se l'ho messo io con lo zip prima va bene, altrimenti copia di sicurezza
			if !inCima[primo] {
				if err := copiaFile(dest, filepath.Join(copie, "originali", rel)); err != nil {
					return fmt.Errorf("copia di sicurezza di %s: %w", rel, err)
				}
				m.Sostituiti = append(m.Sostituiti, rel)
				// su disco PRIMA di sovrascrivere: se qualcosa va storto l'originale torna
				if err := salva(); err != nil {
					return fmt.Errorf("non riesco a scrivere l'elenco dei file sostituiti: %w", err)
				}
			}
		} else if _, err := os.Stat(filepath.Join(cartella, primo)); err != nil && !inCima[primo] {
			inCima[primo] = true
			m.Aggiunti = append(m.Aggiunti, primo)
			// il manifesto sa di questo file PRIMA che io lo crei: se il PC si spegne a metà,
			// "Ferma traduzione" lo toglie lo stesso
			if err := salva(); err != nil {
				return fmt.Errorf("non riesco a scrivere l'elenco dei file aggiunti: %w", err)
			}
		} else if !inCima[primo] {
			// cartella del gioco che esiste già (es. "dotnet"): non ci scrivo dentro
			return fmt.Errorf("il gioco ha già una cartella %s: per sicurezza non lo tocco", primo)
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := scriviDaZip(f, dest); err != nil {
			return err
		}
	}
	return nil
}

func scriviDaZip(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func copiaFile(da, a string) error {
	// leggo solo un file normale (un link farebbe leggere a Tasso da amministratore un file qualsiasi)
	// e non scrivo sopra un link: il file nuovo resta dove deve stare
	if fi, err := os.Lstat(da); err != nil {
		return err
	} else if !fi.Mode().IsRegular() {
		return fmt.Errorf("%s non è un file normale", da)
	}
	if fi, err := os.Lstat(a); err == nil && !fi.Mode().IsRegular() {
		return fmt.Errorf("%s non è un file normale", a)
	}
	if err := os.MkdirAll(filepath.Dir(a), 0o755); err != nil {
		return err
	}
	in, err := os.Open(da)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(a)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// configXUnity: inglese → italiano, solo con Tasso sul PC.
func configXUnity(chiave, token, da, a string) string {
	return fmt.Sprintf(`[Service]
Endpoint=CustomTranslate
FallbackEndpoint=

[General]
Language=%s
FromLanguage=%s

[Custom]
Url=http://127.0.0.1:%d/gioco/%s/%s
EnableShortDelay=True
DisableSpamChecks=True

[Behaviour]
MaxCharactersPerTranslation=1000
EnableUIResizing=True
EnableBatching=False
UseStaticTranslations=False
TranslationPostProcessing=ReplaceHtmlEntities

[TextFrameworks]
EnableUGUI=True
EnableUIElements=True
EnableNGUI=True
EnableTextMeshPro=True
EnableTextMesh=True
EnableIMGUI=False
`, codiceXUnity(a), codiceXUnity(da), portaModGioco, chiave, token)
}

func codiceXUnity(c string) string {
	switch c {
	case "zh":
		return "zh-CN"
	case "zt":
		return "zh-TW"
	case "nb":
		return "no"
	}
	return c
}

// configBepInEx: niente finestra nera della console mentre si gioca.
const configBepInEx = `[Logging.Console]
Enabled = false

[Logging.Disk]
Enabled = true
`

// InstallaModGioco mette BepInEx + XUnity nel gioco. zips: [bepinex, xunity] già scaricati.
func InstallaModGioco(dirApp, nome, cartella, chiave string, zips []string, da, a string) (ManifestoMod, error) {
	tipo, exe := TipoUnity(cartella)
	if tipo == "" {
		return ManifestoMod{}, errors.New("non è un gioco Unity: lo traduco con la sovrimpressione")
	}
	if !percorsoSenzaLink(cartella) {
		return ManifestoMod{}, errors.New(motivoCartellaCollegamento)
	}
	if g := ModGiaPresenti(cartella); g != "" {
		return ManifestoMod{}, fmt.Errorf("il gioco ha già delle mod (%s): per sicurezza non lo tocco", g)
	}
	if _, gia := leggiManifesto(dirApp, chiave); gia {
		return ManifestoMod{}, errors.New("la traduzione dentro il gioco è già installata")
	}
	copie := cartellaCopie(dirApp, chiave)
	if !copieSenzaLink(dirApp, chiave, "originali") {
		return ManifestoMod{}, errors.New("la cartella delle copie di sicurezza porta altrove (link o giunzione): per sicurezza non lo tocco")
	}
	tok := make([]byte, 16)
	_, _ = rand.Read(tok)
	m := ManifestoMod{Gioco: nome, Cartella: cartella, Tipo: tipo, Exe: exe, Quando: time.Now(), Token: hex.EncodeToString(tok), Lingue: [2]string{da, a}}
	inCima := map[string]bool{}
	// il manifesto si scrive PRIMA di toccare il gioco e prima di ogni cosa nuova in cima
	salva := func() error { return scriviManifesto(dirApp, chiave, m) }
	if err := salva(); err != nil {
		return ManifestoMod{}, err
	}
	for _, z := range zips {
		err := estraiInGioco(z, cartella, copie, &m, inCima, salva)
		if err == nil {
			err = salva()
		}
		if err != nil {
			_ = TogliModGioco(dirApp, chiave)
			return ManifestoMod{}, err
		}
	}
	cfg := filepath.Join(cartella, "BepInEx", "config")
	if !senzaLink(cartella, filepath.Join("BepInEx", "config", "AutoTranslatorConfig.ini")) || !senzaLink(cartella, filepath.Join("BepInEx", "config", "BepInEx.cfg")) {
		_ = TogliModGioco(dirApp, chiave)
		return ManifestoMod{}, errors.New("nella cartella BepInEx c'è un link o una giunzione: per sicurezza non lo tocco")
	}
	_ = os.MkdirAll(cfg, 0o755)
	if err := os.WriteFile(filepath.Join(cfg, "AutoTranslatorConfig.ini"), []byte(configXUnity(chiave, m.Token, da, a)), 0o644); err != nil {
		_ = TogliModGioco(dirApp, chiave)
		return ManifestoMod{}, err
	}
	_ = os.WriteFile(filepath.Join(cfg, "BepInEx.cfg"), []byte(configBepInEx), 0o644)
	// traduzioni già fatte in una prova precedente: le rimetto, così non si riparte da zero
	salvate := filepath.Join(copie, "traduzioni")
	if _, err := os.Stat(salvate); err == nil && senzaLink(copie, "traduzioni") && senzaLink(cartella, filepath.Join("BepInEx", "Translation")) {
		_ = copiaCartella(salvate, filepath.Join(cartella, "BepInEx", "Translation"))
	}
	return m, scriviManifesto(dirApp, chiave, m)
}

// TogliModGioco: "Ferma traduzione". Tolgo solo quello che ho aggiunto io e rimetto gli originali.
// Le traduzioni fatte nel gioco le tengo da parte per la prossima volta.
func TogliModGioco(dirApp, chiave string) error {
	if !copieSenzaLink(dirApp, chiave, "") {
		// riprovare non serve: da sistemare a mano (mai "chiudi il gioco e riprova" all'infinito)
		return &ErrDaSistemareAMano{Gioco: chiave, Motivo: motivoCopieCollegamento, Dove: cartellaCopie(dirApp, chiave)}
	}
	m, ok := leggiManifesto(dirApp, chiave)
	if !ok {
		if esisteVoce(filepath.Join(cartellaCopie(dirApp, chiave), "manifesto.json")) {
			// c'è ma non si legge (PC spento mentre lo scrivevo) o non ha la firma di Tasso: non
			// dico "fatto" se non lo è, e non tocco niente
			return fmt.Errorf("l'elenco dei file aggiunti è rovinato: togli a mano BepInEx, winhttp.dll e doorstop dalla cartella del gioco")
		}
		return nil
	}
	copie := cartellaCopie(dirApp, chiave)
	// (il controllo dei collegamenti nella cartella delle copie è già in cima, prima di leggere l'elenco)
	nomeG := m.Gioco
	if nomeG == "" {
		nomeG = filepath.Base(m.Cartella)
	}
	if _, err := os.Lstat(m.Cartella); err != nil {
		return fmt.Errorf("la cartella del gioco non c'è più: non tolgo niente")
	}
	if !percorsoSenzaLink(m.Cartella) {
		// la cartella del gioco (o una cartella sopra) è un collegamento o una giunzione: da
		// amministratore non ci scrivo. Riprovare non serve: da sistemare a mano.
		return &ErrDaSistemareAMano{Gioco: nomeG, Motivo: motivoCartellaCollegamento, Dove: filepath.Join(copie, "originali")}
	}
	if m.Tipo == "unreal" {
		if err := togliModUnreal(m); err != nil {
			var iu *errInUso
			if errors.As(err, &iu) {
				return err
			}
			return &ErrDaSistemareAMano{Gioco: nomeG, Motivo: motivoCartellaCollegamento, Dove: copie}
		}
		return os.Remove(filepath.Join(copie, "manifesto.json"))
	}
	// Il manifesto sta nella cartella dei dati dell'utente (la può cambiare qualsiasi programma) e
	// Tasso lo usa da amministratore: tocco la cartella solo se è ancora un gioco Unity vero (un .exe
	// con la sua cartella _Data), come quando ho installato. Così un elenco finto non può far
	// cancellare o sostituire file in Windows, in Programmi o in un altro programma.
	if !cartellaModValida(m.Cartella) {
		return errors.New("la cartella nell'elenco dei file aggiunti non è un gioco Unity: per sicurezza non tocco niente")
	}
	if tr := filepath.Join(m.Cartella, "BepInEx", "Translation"); esisteVoce(tr) && senzaLink(m.Cartella, filepath.Join("BepInEx", "Translation")) && senzaLink(copie, "traduzioni") {
		_ = os.RemoveAll(filepath.Join(copie, "traduzioni"))
		_ = copiaCartella(tr, filepath.Join(copie, "traduzioni"))
	}
	// plugin che l'utente ha aggiunto da solo dentro BepInEx: li metto da parte, non li cancello
	for _, sub := range []string{"plugins", "patchers"} {
		if !senzaLink(m.Cartella, filepath.Join("BepInEx", sub)) {
			continue // BepInEx o la sua cartella portano altrove (giunzione o link): non sposto niente
		}
		voci, _ := os.ReadDir(filepath.Join(m.Cartella, "BepInEx", sub))
		for _, v := range voci {
			n := v.Name()
			if strings.HasPrefix(n, "XUnity.") || strings.HasPrefix(strings.ToLower(n), "readme") {
				continue
			}
			da := filepath.Join(m.Cartella, "BepInEx", sub, n)
			a := filepath.Join(copie, "plugin-utente", sub, n)
			if esisteVoce(a) { // messo da parte già un'altra volta: non lo copro
				a += "-" + time.Now().Format("20060102-150405")
			}
			if !senzaLink(copie, filepath.Join("plugin-utente", sub, filepath.Base(a))) {
				continue // la copia porta altrove: un file del gioco finirebbe in Windows o in Programmi
			}
			_ = os.MkdirAll(filepath.Dir(a), 0o755)
			if os.Rename(da, a) != nil {
				// altro disco (gioco su D:, Tasso su C:): copio, e se non riesco non tolgo niente
				var err error
				if v.IsDir() {
					err = copiaCartella(da, a)
				} else {
					err = copiaFile(da, a)
				}
				if err != nil {
					return fmt.Errorf("non riesco a mettere da parte il plugin %s: non tolgo niente", n)
				}
			}
		}
	}
	// errori: file in uso (di solito il gioco è aperto): riprovare serve. daControllare: originali
	// che non sono quelli messi da parte (impronta diversa, mancanti) o percorsi che passano da un
	// collegamento: riprovare non serve, li salto e lo dico (ErrOriginaliDaControllare).
	var errori, daControllare, nonTolti []string
	for _, a := range m.Aggiunti {
		if a == "" || strings.ContainsAny(a, `/\:`) || a == "." || a == ".." || fileDelGiocoUnity(a) {
			continue
		}
		if !senzaCollegamenti(m.Cartella, a) {
			nonTolti = append(nonTolti, a) // resta nel gioco: lo dico a parte (R6)
			continue
		}
		if err := os.RemoveAll(filepath.Join(m.Cartella, a)); err != nil {
			errori = append(errori, a)
		}
	}
	for _, s := range m.Sostituiti {
		if filepath.IsAbs(s) || strings.Contains(s, "..") || strings.Contains(s, ":") || fileDelGiocoUnity(primoPezzo(s)) || fileDelGiocoUnity(filepath.Base(s)) {
			continue
		}
		if !senzaLink(m.Cartella, s) || !senzaLink(copie, filepath.Join("originali", s)) {
			daControllare = append(daControllare, s) // un pezzo del percorso porta fuori dal gioco: non scrivo
			continue
		}
		// l'originale messo da parte deve essere proprio quello (impronta firmata), e la strada
		// fino al file nel gioco non deve passare da collegamenti
		// leggo l'originale una volta sola: controllo l'impronta e scrivo proprio quei byte (se lo
		// riaprissi, un altro programma potrebbe scambiarlo tra il controllo e la copia)
		dati, err := os.ReadFile(filepath.Join(copie, "originali", s))
		if err != nil || m.Impronte[s] == "" || improntaByte(dati) != m.Impronte[s] || !senzaCollegamenti(m.Cartella, s) {
			daControllare = append(daControllare, s)
			continue
		}
		dst := filepath.Join(m.Cartella, s)
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		if err := os.WriteFile(dst, dati, 0o644); err != nil {
			errori = append(errori, s)
		}
	}
	if len(errori) > 0 {
		return fmt.Errorf("non sono riuscito a togliere: %s (il gioco è aperto?)", strings.Join(errori, ", "))
	}
	if len(daControllare) > 0 || len(nonTolti) > 0 {
		// la traduzione è tolta (tranne nonTolti); gli originali messi da parte restano, con un nome
		// che dice di controllarli (togliendo Tasso vanno nelle copie di sicurezza con il resto)
		dove := filepath.Join(copie, "originali-da-controllare-"+time.Now().Format("20060102-150405"))
		if esisteVoce(filepath.Join(copie, "originali")) {
			if err := os.Rename(filepath.Join(copie, "originali"), dove); err != nil {
				// R5: senza il nome nuovo una traduzione nuova li cancellerebbe togliendola: l'elenco
				// resta, e si riprova quando i file non sono più in uso
				return &errInUso{"non sono riuscito a mettere da parte gli originali (" + err.Error() + "): riprova a gioco chiuso"}
			}
		} else {
			dove = copie
		}
		_ = os.Remove(filepath.Join(copie, "manifesto.json"))
		return &ErrOriginaliDaControllare{Gioco: nomeG, File: daControllare, NonTolti: nonTolti, Dove: dove}
	}
	_ = os.Remove(filepath.Join(copie, "manifesto.json"))
	_ = os.RemoveAll(filepath.Join(copie, "originali"))
	return nil
}

// ErrOriginaliDaControllare: la traduzione è stata tolta, ma questi file non li ho rimessi (o tolti)
// perché la copia messa da parte non è quella di Tasso o il percorso passa da un collegamento.
// Riprovare non serve: il gioco si rimette a posto con "Verifica file"; le copie restano in Dove.
type ErrOriginaliDaControllare struct {
	Gioco    string
	File     []string // originali non rimessi
	NonTolti []string // file aggiunti da Tasso rimasti nel gioco (il percorso passa da un collegamento)
	Dove     string
}

func (e *ErrOriginaliDaControllare) Error() string {
	var s string
	if len(e.NonTolti) > 0 {
		s = "Ho tolto la traduzione, tranne questi file che passano da un collegamento (li ho lasciati nel gioco): " + strings.Join(e.NonTolti, ", ")
		if len(e.File) > 0 {
			s += ". Questi file non li ho rimessi com'erano, perché la copia messa da parte è cambiata o il percorso passa da un collegamento: " + strings.Join(e.File, ", ")
		}
	} else {
		s = "Ho tolto la traduzione, ma questi file non li ho rimessi com'erano, perché la copia messa da parte è cambiata o il percorso passa da un collegamento: " + strings.Join(e.File, ", ")
	}
	return s + ". Usa \"Verifica file\" nel programma da cui lanci il gioco. Le copie da controllare sono in " + e.Dove + "."
}

// ErrDaSistemareAMano: la traduzione dentro il gioco non la posso togliere da qui (la cartella del
// gioco, o quella delle copie, passa da un collegamento o da una giunzione). Riprovare non serve:
// il gioco si rimette a posto con "Verifica file"; gli originali sono in Dove.
type ErrDaSistemareAMano struct {
	Gioco, Motivo, Dove string
}

func (e *ErrDaSistemareAMano) Error() string {
	return "Attenzione: " + e.Motivo + ". " + msgSistemaAMano(e.Gioco, e.Dove)
}

// I motivi (tradotti nella sezione "go" delle lingue).
const (
	motivoCartellaCollegamento = "la cartella del gioco passa da un collegamento o da una giunzione, e per sicurezza Tasso non ci scrive"
	motivoCopieCollegamento    = "la cartella delle copie di sicurezza passa da un collegamento o da una giunzione"
)

// senzaLink: nessun pezzo di rel dentro base (cartelle e file, se ci sono) è un link o una giunzione
// di Windows. Una giunzione la può creare anche un programma normale: seguendola, Tasso da
// amministratore scriverebbe fuori dal gioco (per esempio in System32). Se un pezzo non c'è ancora
// va bene: lo crea Tasso come cartella normale.
func senzaLink(base, rel string) bool {
	p := base
	for _, pezzo := range strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == '\\' }) {
		p = filepath.Join(p, pezzo)
		fi, err := os.Lstat(p)
		if err != nil {
			return os.IsNotExist(err)
		}
		if fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return false
		}
	}
	return true
}

// copieSenzaLink: come senzaLink per la cartella delle copie di sicurezza (dentro dirApp, che sta nei
// dati dell'utente): copie-giochi, la cartella del gioco e rel non devono portare altrove.
func copieSenzaLink(dirApp, chiave, rel string) bool {
	return senzaLink(dirApp, filepath.Join("copie-giochi", chiave, rel))
}

// cartellaModValida: la cartella di un manifesto si tocca solo se è un percorso completo e un gioco
// Unity vero (TipoUnity: un .exe con accanto la sua cartella _Data). Un programma normale non può
// creare un .exe dentro Windows, Programmi o la radice del disco.
func cartellaModValida(c string) bool {
	if c == "" || !filepath.IsAbs(c) {
		return false
	}
	tipo, _ := TipoUnity(c)
	return tipo != ""
}

// fileDelGiocoUnity: nomi che sono del gioco, mai di BepInEx o XUnity (che in cima non hanno .exe né
// cartelle _Data): "Ferma traduzione" non li toglie né li sovrascrive, anche se un elenco cambiato
// li dicesse aggiunti o sostituiti.
func fileDelGiocoUnity(nome string) bool {
	n := strings.ToLower(nome)
	return strings.HasSuffix(n, ".exe") || strings.HasSuffix(n, "_data") || n == "unityplayer.dll" ||
		n == "gameassembly.dll" || n == "monobleedingedge"
}

func primoPezzo(rel string) string {
	return strings.Split(strings.ReplaceAll(rel, `\`, "/"), "/")[0]
}

func copiaCartella(da, a string) error {
	return filepath.Walk(da, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(da, p)
		if !info.IsDir() && !info.Mode().IsRegular() {
			return nil // link, giunzione o altro: non lo seguo
		}
		if !senzaLink(a, rel) {
			return fmt.Errorf("nel percorso di %s c'è un link o una giunzione", rel)
		}
		if info.IsDir() {
			return os.MkdirAll(filepath.Join(a, rel), 0o755)
		}
		return copiaFile(p, filepath.Join(a, rel))
	})
}

// ---- correzioni dell'IA nel file delle traduzioni di XUnity ----
// XUnity chiede ogni frase una volta sola e la salva in _AutoGeneratedTranslations.txt. Quando
// il correttore IA migliora una frase, la cambio in quel file: al prossimo avvio del gioco si
// vede già corretta.

func escapeXUnity(s string) string {
	r := strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\r", `\r`, "=", `\=`)
	return r.Replace(s)
}

// CorreggiFileXUnity sostituisce le righe "inglese=bozza" con "inglese=corretta". Ritorna quante.
func CorreggiFileXUnity(file string, corr map[string][2]string) (int, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return 0, err
	}
	righe := strings.Split(string(b), "\n")
	n := 0
	for i, r := range righe {
		chiave, val, ok := tagliaRigaXUnity(strings.TrimRight(r, "\r"))
		if !ok {
			continue
		}
		for en, c := range corr {
			if chiave == escapeXUnity(en) && val == escapeXUnity(c[0]) {
				righe[i] = chiave + "=" + escapeXUnity(c[1])
				n++
			}
			// "- Trova la chiave": nel gioco la frase ha il trattino davanti (TraduciPer lo rimette)
			if chiave == escapeXUnity("- "+en) && val == escapeXUnity("- "+c[0]) {
				righe[i] = chiave + "=" + escapeXUnity("- "+c[1])
				n++
			}
		}
	}
	if n == 0 {
		return 0, nil
	}
	tmp := file + ".tasso"
	if err := os.WriteFile(tmp, []byte(strings.Join(righe, "\n")), 0o644); err != nil {
		return 0, err
	}
	return n, os.Rename(tmp, file)
}

// tagliaRigaXUnity divide "chiave=valore" al primo "=" non preceduto da "\".
func tagliaRigaXUnity(r string) (string, string, bool) {
	if strings.HasPrefix(r, "//") {
		return "", "", false
	}
	for i := 0; i < len(r); i++ {
		if r[i] == '\\' {
			i++
			continue
		}
		if r[i] == '=' {
			return r[:i], r[i+1:], true
		}
	}
	return "", "", false
}

func esisteVoce(p string) bool { _, err := os.Stat(p); return err == nil }

// versioneMetadataMassima: la versione più nuova dei dati IL2CPP che il BepInEx incluso sa
// leggere (Cpp2IL: "We support 23-106"). Unity 6.6 usa la 108: lì la traduzione dentro il gioco
// NON parte (visto su Moon Sweepers il 22/11/2026), quindi non la propongo nemmeno.
const versioneMetadataMassima = 106

// ModPossibile: si può tradurre dentro il gioco? Se no, il motivo in parole semplici.
func ModPossibile(cartella string) (bool, string) {
	if !percorsoSenzaLink(cartella) {
		return false, motivoCartellaCollegamento
	}
	tipo, exe := TipoUnity(cartella)
	if tipo == "mono" || tipo == "il2cpp" {
		// gli strumenti che installo sono solo a 64 bit: in un gioco a 32 bit il gioco non partirebbe
		if !a64Bit(cartella, exe) {
			return false, "gioco a 32 bit: traduco sullo schermo"
		}
	}
	switch tipo {
	case "mono":
		return true, ""
	case "il2cpp":
		base := strings.TrimSuffix(exe, filepath.Ext(exe))
		f, err := os.Open(filepath.Join(cartella, base+"_Data", "il2cpp_data", "Metadata", "global-metadata.dat"))
		if err != nil {
			return false, "non trovo i dati del gioco"
		}
		defer f.Close()
		var h [8]byte
		if _, err := io.ReadFull(f, h[:]); err != nil {
			return false, "non riesco a leggere i dati del gioco"
		}
		magia := uint32(h[0]) | uint32(h[1])<<8 | uint32(h[2])<<16 | uint32(h[3])<<24
		ver := int(uint32(h[4]) | uint32(h[5])<<8 | uint32(h[6])<<16 | uint32(h[7])<<24)
		if magia != 0xFAB11BAF {
			return false, "i dati del gioco sono protetti"
		}
		if ver > versioneMetadataMassima {
			return false, "gioco troppo nuovo per gli strumenti liberi"
		}
		return true, ""
	}
	if CartellaPaksUnreal(cartella) != "" {
		if !forzaPerProva("TASSO_FILE_UNREAL") {
			// spento finché non funziona in un gioco vero (24/11): non leggo l'indice dei pak, che
			// nei giochi grandi (Palworld, 39 GB) pesa centinaia di MB
			return false, "per i giochi Unreal per ora traduco sullo schermo"
		}
		return UnrealPossibile(cartella)
	}
	return false, "solo giochi Unity e Unreal"
}

// ModNonPartita: dal registro di BepInEx, la traduzione dentro il gioco non è partita.
func ModNonPartita(cartella string) bool {
	b, err := os.ReadFile(filepath.Join(cartella, "BepInEx", "LogOutput.log"))
	if err != nil {
		return false
	}
	s := string(b)
	for _, segno := range []string{"Unable to execute IL2CPP chainloader", "Failed to generate Il2Cpp interop", "Unsupported metadata version", "Could not load XUnity", "XUnity.AutoTranslator: Failed"} {
		if strings.Contains(s, segno) {
			return true
		}
	}
	return false
}

// a64Bit: il programma del gioco (o UnityPlayer.dll, che carica winhttp.dll) è x64. Se non
// riesco a leggerlo dico di no: meglio la traduzione sullo schermo che un gioco che non parte.
func a64Bit(cartella, exe string) bool {
	for _, n := range []string{"UnityPlayer.dll", exe} {
		if n == "" {
			continue
		}
		m, ok := macchinaPE(filepath.Join(cartella, n))
		if !ok {
			continue
		}
		return m == pe.IMAGE_FILE_MACHINE_AMD64
	}
	return false
}

// macchinaPE: il tipo di processore scritto nell'intestazione del programma (solo i pochi byte che servono).
func macchinaPE(p string) (uint16, bool) {
	f, err := os.Open(p)
	if err != nil {
		return 0, false
	}
	defer f.Close()
	var h [64]byte
	if _, err := io.ReadFull(f, h[:]); err != nil || h[0] != 'M' || h[1] != 'Z' {
		return 0, false
	}
	off := int64(uint32(h[0x3c]) | uint32(h[0x3d])<<8 | uint32(h[0x3e])<<16 | uint32(h[0x3f])<<24)
	var c [6]byte
	if _, err := f.ReadAt(c[:], off); err != nil || string(c[:4]) != "PE\x00\x00" {
		return 0, false
	}
	return uint16(c[4]) | uint16(c[5])<<8, true
}
