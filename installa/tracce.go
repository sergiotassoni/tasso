package installa

import (
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
					continue // un'altra copia del programma: le sue voci restano
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
			for _, e := range g.scelte {
				for _, c := range scelteUtente {
					k := chiaveScelteUtente + e + `\` + c
					if v, ok := r.LeggiTesto(HKCU, k, "ProgId"); ok && sceltaDiTassoZip(v) {
						if err := r.TogliChiave(HKCU, k); err != nil {
							Diario("scelta \"Apri con\" di %s non tolta: %v", e, err)
						}
					}
				}
			}
		}
	}
	dati := in.CartellaDatiUtente
	if in.CartellaDatiSessione != nil {
		dati = in.CartellaDatiSessione // Zip e PDF: la %LOCALAPPDATA% dell'utente che li usa
	}
	if dati != nil {
		if nome := cartellaDatiProgramma(p); nome != "" {
			if radice := dati(); radice != "" && filepath.IsAbs(radice) {
				d := filepath.Join(radice, nome)
				if err := in.togliCartella(d); err != nil {
					nonTolti = append(nonTolti, d)
				}
			}
		}
	}
	return nonTolti
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
