package installa

import (
	"os"
	"path/filepath"
	"strings"
)

// Tracce dell'utente: Tasso Zip e Tasso PDF, quando l'utente lo chiede dentro il programma,
// scrivono da soli in HKEY_CURRENT_USER il menu del tasto destro e (Tasso Zip) "Apri con" e
// il doppio clic sugli archivi. Se il programma si disinstalla, quelle voci resterebbero lì e
// porterebbero a un exe che non c'è più: le tolgo, insieme alla cartella dei dati del
// programma in %LOCALAPPDATA% (impostazioni, file temporanei, dati di WebView2).
//
// I nomi qui devono restare uguali a quelli di tassozip (menu_windows.go, associa_windows.go)
// e tassopdf (menu_windows.go).

// gruppoTracce: chiavi scritte insieme dal programma. Prova è una chiave con un comando:
// se il comando avvia un altro exe (un'altra copia del programma, non quella installata),
// il gruppo non è nostro e non lo tocco.
type gruppoTracce struct {
	prova   string      // chiave del comando ("" = nessuna prova)
	alberi  []string    // chiavi da togliere con tutte le sottochiavi
	valori  [][2]string // {chiave, nome del valore} da togliere
	seVale  [][2]string // {chiave, valore}: tolgo il valore predefinito della chiave solo se è questo
	vuote   []string    // chiavi da togliere se restano vuote
	exeNome string
	// scelte: estensioni di cui togliere la scelta "Apri con > … > Sempre" dell'utente
	// (FileExts\<ext>\UserChoice e UserChoiceLatest), solo se porta a questo programma (sceltaNostra)
	scelte []string
}

// chiaveScelteUtente e scelteUtente: dove Windows tiene "Apri con > … > Sempre" (UserChoiceLatest
// nei Windows 11 più nuovi). Stessi nomi di Tasso Zip (associa_windows.go).
const chiaveScelteUtente = `Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\`

var scelteUtente = []string{"UserChoiceLatest", "UserChoice"}

// progIdScelta: il programma scritto in una chiave di scelteUtente. In UserChoiceLatest Windows
// 11 lo tiene nella sottochiave "ProgId" (valore "ProgId"; la chiave ha solo "Hash"): prima
// quella, poi il valore diretto. È la stessa regola di Tasso Zip (associa_windows.go).
func progIdScelta(r Registro, chiave string) string {
	for _, c := range []string{chiave + `\ProgId`, chiave} {
		if v, ok := r.LeggiTesto(HKCU, c, "ProgId"); ok && v != "" {
			return v
		}
	}
	return ""
}

// exeDelComando: l'exe all'inizio di un comando come `"C:\…\TassoZip.exe" --zip "%1"`.
func exeDelComando(c string) string {
	c = strings.TrimSpace(c)
	if !strings.HasPrefix(c, `"`) {
		return ""
	}
	i := strings.Index(c[1:], `"`)
	if i <= 0 {
		return ""
	}
	return c[1 : 1+i]
}

// esisteFile: il file c'è? (nil = os.Stat; i test lo cambiano per le copie "D:\Portatili\…")
func (in *Installatore) esisteFile(p string) bool {
	if in.EsisteFile != nil {
		return in.EsisteFile(p)
	}
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// sceltaDiTassoZip: il ProgId scelto dall'utente porta a Tasso Zip (il suo tipo di file o "Apri con >
// Tasso Zip"). È la stessa regola di diTassoZip in Tasso Zip.
func sceltaDiTassoZip(p string) bool {
	return strings.EqualFold(p, progIDZip) || strings.EqualFold(p, `Applications\TassoZip.exe`)
}

var (
	estMenuZip  = []string{".zip", ".7z", ".rar", ".tar", ".gz", ".tgz", ".bz2", ".tbz2", ".tbz", ".xz", ".txz", ".zst", ".tzst", ".001"}
	estAssocZip = []string{".zip", ".7z", ".rar", ".tar", ".gz", ".tgz", ".bz2", ".tbz2", ".xz", ".txz", ".zst", ".tzst"}
	estMenuPDF  = []string{".pdf", ".jpg", ".jpeg", ".png"}
)

const progIDZip = "TassoZip.Archivio"

// tracceDi: i gruppi di chiavi in HKCU di un programma.
func tracceDi(p Programma) []gruppoTracce {
	cl := `Software\Classes\`
	switch p {
	case ProgZip:
		menu := gruppoTracce{prova: cl + `*\shell\TassoZip\shell\1comprimi\command`, exeNome: "TassoZip.exe",
			alberi: []string{cl + `*\shell\TassoZip`, cl + `Directory\shell\TassoZip`}}
		vecchio := gruppoTracce{prova: cl + `*\shell\TassoZip.Comprimi\command`, exeNome: "TassoZip.exe"}
		for _, v := range []string{"TassoZip.Comprimi", "TassoZip.Estrai"} {
			vecchio.alberi = append(vecchio.alberi, cl+`*\shell\`+v, cl+`Directory\shell\`+v)
		}
		for _, e := range estMenuZip {
			menu.alberi = append(menu.alberi, cl+`SystemFileAssociations\`+e+`\shell\TassoZip`)
			for _, v := range []string{"TassoZip.Comprimi", "TassoZip.Estrai"} {
				vecchio.alberi = append(vecchio.alberi, cl+`SystemFileAssociations\`+e+`\shell\`+v)
			}
		}
		assoc := gruppoTracce{prova: cl + progIDZip + `\shell\open\command`, exeNome: "TassoZip.exe",
			alberi: []string{cl + progIDZip, cl + `Applications\TassoZip.exe`, `Software\Tasso\TassoZip`},
			valori: [][2]string{{`Software\RegisteredApplications`, "Tasso Zip"}}}
		for _, e := range estAssocZip {
			assoc.valori = append(assoc.valori, [2]string{cl + e + `\OpenWithProgids`, progIDZip})
			assoc.seVale = append(assoc.seVale, [2]string{cl + e, progIDZip})
			assoc.vuote = append(assoc.vuote, cl+e+`\OpenWithProgids`, cl+e)
			assoc.scelte = append(assoc.scelte, e)
		}
		return []gruppoTracce{menu, vecchio, assoc}
	case ProgPDF:
		menu := gruppoTracce{prova: cl + `SystemFileAssociations\.pdf\shell\TassoPDF\shell\1apri\command`, exeNome: "TassoPDF.exe"}
		for _, e := range estMenuPDF {
			menu.alberi = append(menu.alberi, cl+`SystemFileAssociations\`+e+`\shell\TassoPDF`)
		}
		return []gruppoTracce{menu}
	}
	return nil
}

// cartellaDatiProgramma: il nome della cartella dei dati in %LOCALAPPDATA%.
func cartellaDatiProgramma(p Programma) string {
	switch p {
	case ProgZip:
		return "Tasso Zip"
	case ProgPDF:
		// dentro c'è anche la firma salvata con "Ricorda" (firma.dat, cifrata per l'utente): senza
		// Tasso PDF non serve più a niente e non deve restare sul PC di chi ha tolto il programma
		return "Tasso PDF"
	}
	return ""
}

// togliTracceUtente toglie le voci del programma in HKCU (solo quelle che avviano l'exe
// installato in base) e la sua cartella dei dati. Restituisce le cartelle non tolte del tutto.
func (in *Installatore) togliTracceUtente(base string, p Programma) []string {
	var nonTolti []string
	if r := in.registroTracce(); r != nil {
		for _, g := range tracceDi(p) {
			exe := strings.ToLower(filepath.Join(CartellaDi(base, p), g.exeNome))
			if g.prova != "" {
				if c, ok := r.LeggiTesto(HKCU, g.prova, ""); ok && c != "" && !strings.Contains(strings.ToLower(c), exe) {
					// un'altra copia del programma: le sue voci restano, ma solo se quella copia c'è
					// davvero (una copia aperta una volta da TEMP, che Windows ha già cancellato,
					// lascerebbe per sempre voci che avviano un exe inesistente)
					if altro := exeDelComando(c); altro == "" || in.esisteFile(altro) {
						continue
					}
				}
			}
			for _, a := range g.alberi {
				_ = r.TogliAlbero(HKCU, a)
				togliGenitoriVuoti(r, a)
			}
			for _, v := range g.valori {
				_ = r.TogliValore(HKCU, v[0], v[1])
			}
			for _, v := range g.seVale {
				if c, ok := r.LeggiTesto(HKCU, v[0], ""); ok && strings.EqualFold(c, v[1]) {
					_ = r.TogliValore(HKCU, v[0], "")
				}
			}
			for _, k := range g.vuote {
				if r.Vuota(HKCU, k) {
					_ = r.TogliChiave(HKCU, k)
				}
			}
			// "Apri con > Tasso Zip > Sempre": senza Tasso Zip Windows chiederebbe "Come vuoi aprire?".
			// Si legge e, solo se porta a Tasso Zip, si cancella (mai scritta: niente hash).
			// UserChoiceLatest ha la sottochiave ProgId: si toglie tutto l'albero.
			for _, e := range g.scelte {
				togliRestiWindows(r, e)
				for _, c := range scelteUtente {
					k := chiaveScelteUtente + e + `\` + c
					if sceltaDiTassoZip(progIdScelta(r, k)) {
						if err := r.TogliAlbero(HKCU, k); err != nil {
							Diario("scelta \"Apri con\" di %s non tolta: %v", e, err)
						}
					}
				}
			}
		}
	}
	dati, togli := in.CartellaDatiUtente, in.togliCartellaUtente // R3c-3: con il token limitato dell'utente
	if in.CartellaDatiSessione != nil {
		// Zip e PDF: la %LOCALAPPDATA% dell'utente che li usa, tolta con i SUOI permessi (R3b-1)
		dati, togli = in.CartellaDatiSessione, in.togliCartellaSessione
	}
	if dati != nil {
		if nome := cartellaDatiProgramma(p); nome != "" {
			if radice := dati(); radice != "" && filepath.IsAbs(radice) {
				d := filepath.Join(radice, nome)
				if err := togli(d); err != nil {
					Diario("cartella dati %s non tolta: %v", d, err)
					nonTolti = append(nonTolti, d)
				}
			}
		}
	}
	return nonTolti
}

// togliRestiWindows: quello che Windows stesso scrive quando si usa Tasso Zip e che porta a lui
// (innocuo, ma il registro deve tornare com'era): TassoZip.Archivio in FileExts\<ext>\OpenWithProgids,
// TassoZip.exe in FileExts\<ext>\OpenWithList (con la sua lettera in MRUList) e gli avvisi
// "nuova app" in ApplicationAssociationToasts. Gli stessi di Tasso Zip (associa_windows.go).
func togliRestiWindows(r Registro, e string) {
	fe := chiaveScelteUtente + e
	_ = r.TogliValore(HKCU, fe+`\OpenWithProgids`, progIDZip)
	if mru, ok := r.LeggiTesto(HKCU, fe+`\OpenWithList`, "MRUList"); ok {
		nuovo := mru
		for c := 'a'; c <= 'z'; c++ {
			if v, ok := r.LeggiTesto(HKCU, fe+`\OpenWithList`, string(c)); ok && strings.EqualFold(v, "TassoZip.exe") {
				if r.TogliValore(HKCU, fe+`\OpenWithList`, string(c)) == nil {
					nuovo = strings.ReplaceAll(nuovo, string(c), "")
				}
			}
		}
		if nuovo != mru {
			_ = r.ScriviTesto(HKCU, fe+`\OpenWithList`, "MRUList", nuovo)
		}
	}
	toasts := `Software\Microsoft\Windows\CurrentVersion\ApplicationAssociationToasts`
	_ = r.TogliValore(HKCU, toasts, progIDZip+"_"+e)
	_ = r.TogliValore(HKCU, toasts, `Applications\TassoZip.exe_`+e)
}

// togliGenitoriVuoti: dopo aver tolto "…\.pdf\shell\TassoPDF", toglie anche "…\.pdf\shell" e
// "…\.pdf" se sono rimaste vuote (le aveva create il programma). Mai sopra Software\Classes.
func togliGenitoriVuoti(r Registro, chiave string) {
	for {
		i := strings.LastIndex(chiave, `\`)
		if i < 0 {
			return
		}
		chiave = chiave[:i]
		l := strings.ToLower(chiave)
		if l == `software\classes` || l == `software` || l == `software\tasso` || !strings.HasPrefix(l, `software\`) {
			return
		}
		if !r.Vuota(HKCU, chiave) {
			return
		}
		if r.TogliChiave(HKCU, chiave) != nil {
			return
		}
	}
}
