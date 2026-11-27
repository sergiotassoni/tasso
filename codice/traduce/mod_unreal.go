package traduce

// Traduzione dentro i giochi Unreal: le scritte del gioco stanno nei file di lingua (.locres)
// chiusi dentro i .pak. Tasso li legge (senza cambiare niente), traduce le frasi e scrive UN file
// nuovo, Paks\~mods\zz_TassoTraduce_P.pak: Unreal carica i pak "_P" dopo gli originali, quindi il
// gioco usa le frasi tradotte al posto di quelle della lingua scelta nel gioco. Gli originali non
// si toccano mai; per tornare com'era basta togliere quel file (lo fa "Ferma traduzione").

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"tasso/traduce/unreal"
)

const nomeModUnreal = "zz_TassoTraduce_P.pak"

// CartellaPaksUnreal: la cartella Content\Paks del gioco (es. Palworld\Pal\Content\Paks), "" se
// il gioco non è fatto con Unreal.
func CartellaPaksUnreal(cartella string) string {
	prova := []string{filepath.Join(cartella, "Content", "Paks")}
	voci, _ := os.ReadDir(cartella)
	for _, v := range voci {
		if v.IsDir() && v.Name() != "Engine" {
			prova = append(prova, filepath.Join(cartella, v.Name(), "Content", "Paks"))
		}
	}
	for _, p := range prova {
		if m, _ := filepath.Glob(filepath.Join(p, "*.pak")); len(m) > 0 {
			return p
		}
	}
	return ""
}

// pakOriginali: i pak del gioco (non quelli delle mod).
func pakOriginali(paks string) []string {
	m, _ := filepath.Glob(filepath.Join(paks, "*.pak"))
	sort.Strings(m)
	return m
}

// UnrealPossibile: si possono leggere i file di lingua di questo gioco?
func UnrealPossibile(cartella string) (bool, string) {
	paks := CartellaPaksUnreal(cartella)
	if paks == "" {
		return false, "non è un gioco Unreal"
	}
	if !percorsoSenzaLink(cartella) || !percorsoSenzaLink(paks) {
		return false, motivoCartellaCollegamento
	}
	for _, pp := range pakOriginali(paks) {
		p, err := unreal.ApriPak(pp)
		if err != nil {
			if strings.Contains(err.Error(), "cifrat") {
				return false, "i file del gioco sono chiusi a chiave"
			}
			continue
		}
		p.Chiudi()
		return true, ""
	}
	return false, "non riesco a leggere i file del gioco"
}

// LocresDaTradurre: un file di lingua del gioco nella lingua "da".
type LocresDaTradurre struct {
	Nome string // percorso dentro il pak (es. Pal/Content/Localization/Game/en/Game.locres)
	L    *unreal.Locres
}

// LeggiLocresUnreal: i file di lingua del gioco da tradurre. Si parte dall'inglese (la lingua in
// cui i giochi sono scritti, la traduzione viene meglio) se c'è, altrimenti dalla lingua del
// gioco; il risultato va al posto della lingua impostata nel gioco ("gioco"), così non devi
// cambiare niente nelle impostazioni del gioco. Restituisce anche la lingua di partenza.
func LeggiLocresUnreal(paks, gioco string) ([]LocresDaTradurre, string, error) {
	sorg := "en"
	files, err := leggiLocresLingua(paks, "en")
	if err != nil && gioco != "en" {
		sorg = gioco
		files, err = leggiLocresLingua(paks, gioco)
	}
	if err != nil {
		return nil, "", err
	}
	if gioco != sorg {
		// metto i file nella cartella della lingua del gioco, se il gioco ce l'ha
		if dest, ok := cartellaLinguaNelPak(paks, gioco); ok {
			for i := range files {
				parti := strings.Split(files[i].Nome, "/")
				parti[len(parti)-2] = dest
				files[i].Nome = strings.Join(parti, "/")
			}
		}
	}
	return files, sorg, nil
}

// GiocoHaLingua: il gioco ha già i suoi testi (non solo quelli del motore Unreal) nella lingua
// voluta. Allora non serve nessun file: basta sceglierla nelle impostazioni del gioco, ed è la
// traduzione fatta da persone (PC Cafe Simulator ha l'italiano ufficiale, 24/11).
func GiocoHaLingua(paks, lingua string) bool {
	for _, pp := range pakOriginali(paks) {
		p, err := unreal.ApriPak(pp)
		if err != nil {
			continue
		}
		for _, v := range p.Voci {
			// percorso intero: un pak montato su ../../../Engine/ ha nomi senza "Engine/" davanti
			n := "/" + strings.ToLower(strings.TrimLeft(strings.ReplaceAll(p.Montaggio+v.Nome, "\\", "/"), "./"))
			if strings.HasPrefix(n, "/engine/") || strings.Contains(n, "/engine/") || strings.Contains(n, "/localization/engine/") {
				continue
			}
			if locresDellaLingua(v.Nome, lingua) && v.Uncomp >= 4096 {
				p.Chiudi()
				return true
			}
		}
		p.Chiudi()
	}
	return false
}

// lingueUfficialiNote: giochi che hanno la lingua nei loro file ma non nei .locres di Unreal
// (testi in tabelle interne), quindi GiocoHaLingua non la vede. Dalla pagina Steam del gioco.
// Palworld (24/11): italiano ufficiale per interfaccia e sottotitoli, testi dentro il pak da 39 GB.
var lingueUfficialiNote = map[string][]string{
	"palworld": {"it", "fr", "de", "es", "pt", "ru", "ja", "ko", "zh", "zt", "pl", "tr", "vi", "th", "id"},
}

// LinguaUfficialeNota: il gioco in questa cartella ha di sicuro la lingua (elenco qui sopra).
func LinguaUfficialeNota(cartella, lingua string) bool {
	n := nomePiatto(filepath.Base(cartella))
	for g, l := range lingueUfficialiNote {
		if n == g {
			for _, x := range l {
				if x == lingua {
					return true
				}
			}
		}
	}
	return false
}

// cartellaLinguaNelPak: il nome esatto della cartella della lingua nei pak (es. "ko", "zh-Hans").
func cartellaLinguaNelPak(paks, lingua string) (string, bool) {
	for _, pp := range pakOriginali(paks) {
		p, err := unreal.ApriPak(pp)
		if err != nil {
			continue
		}
		for _, v := range p.Voci {
			if locresDellaLingua(v.Nome, lingua) {
				parti := strings.Split(v.Nome, "/")
				p.Chiudi()
				return parti[len(parti)-2], true
			}
		}
		p.Chiudi()
	}
	return "", false
}

func leggiLocresLingua(paks, da string) ([]LocresDaTradurre, error) {
	trovati := map[string]LocresDaTradurre{}
	var ultimoErr error
	for _, pp := range pakOriginali(paks) {
		p, err := unreal.ApriPak(pp)
		if err != nil {
			ultimoErr = err
			continue
		}
		for _, v := range p.Voci {
			if !locresDellaLingua(v.Nome, da) {
				continue
			}
			b, err := p.Leggi(v)
			if err != nil {
				ultimoErr = err
				continue
			}
			l, err := unreal.LeggiLocres(b)
			if err != nil {
				ultimoErr = err
				continue
			}
			trovati[strings.ToLower(v.Nome)] = LocresDaTradurre{Nome: strings.TrimPrefix(v.Nome, "/"), L: l}
		}
		p.Chiudi()
	}
	if len(trovati) == 0 {
		if ultimoErr != nil {
			return nil, ultimoErr
		}
		return nil, fmt.Errorf("nel gioco non c'è la lingua %s", nomeLingua(da))
	}
	var out []LocresDaTradurre
	for _, t := range trovati {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Nome < out[j].Nome })
	return out, nil
}

// locresDellaLingua: .../Localization/<qualcosa>/<lingua>/<file>.locres. La cartella della
// lingua può avere la regione (en-US, zh-Hans...).
func locresDellaLingua(nome, da string) bool {
	n := strings.ToLower(strings.ReplaceAll(nome, `\`, "/"))
	if !strings.HasSuffix(n, ".locres") || !strings.Contains(n, "/localization/") {
		return false
	}
	parti := strings.Split(n, "/")
	if len(parti) < 3 {
		return false
	}
	cart := parti[len(parti)-2]
	for _, c := range cartelleLinguaUnreal(da) {
		if cart == c {
			return true
		}
	}
	return false
}

func cartelleLinguaUnreal(da string) []string {
	switch da {
	case "zh":
		return []string{"zh-hans", "zh-cn", "zh"}
	case "zt":
		return []string{"zh-hant", "zh-tw", "zh-hk"}
	case "pt":
		return []string{"pt-br", "pt", "pt-pt"}
	case "es":
		return []string{"es", "es-es", "es-mx", "es-419"}
	case "nb":
		return []string{"nb", "no", "nb-no"}
	}
	return []string{da, da + "-" + strings.ToLower(regioneDi(da))}
}

func regioneDi(da string) string {
	m := map[string]string{"en": "us", "it": "it", "fr": "fr", "de": "de", "ja": "jp", "ko": "kr", "ru": "ru", "pl": "pl", "tr": "tr", "sv": "se", "fi": "fi", "da": "dk"}
	return m[da]
}

// ---- pezzi da non tradurre: {0}, {Nome}, <Tag>, </>, \n, %s, %d...

var rePezziFissi = regexp.MustCompile(`\{[^{}]*\}|<[^<>]*>|\r?\n|%[0-9]*[sdif]|\\n`)

// tradurreFrase: false per le frasi senza parole (numeri, simboli, codici interni).
func tradurreFrase(s string) bool {
	t := rePezziFissi.ReplaceAllString(s, " ")
	lettere := 0
	for _, r := range t {
		if unicode.IsLetter(r) {
			lettere++
		}
	}
	if lettere < 2 {
		return false
	}
	// codici interni tipo "BP_Item_01" o "None"
	if !strings.ContainsAny(strings.TrimSpace(t), " ") && strings.ContainsAny(t, "_") {
		return false
	}
	return true
}

// TraduciConPezziFissi: traduce s lasciando identici i pezzi fissi. Prima prova la frase intera
// (per la grammatica); se il traduttore ha perso o cambiato un pezzo fisso, traduce i tratti di
// testo uno per uno.
func TraduciConPezziFissi(s string, traduci func(string) (string, bool)) (string, bool) {
	pezzi := rePezziFissi.FindAllString(s, -1)
	if len(pezzi) == 0 {
		return traduci(s)
	}
	if !strings.Contains(s, "\n") {
		if t, ok := traduci(s); ok && stessiPezzi(pezzi, rePezziFissi.FindAllString(t, -1)) {
			return t, true
		}
	}
	idx := rePezziFissi.FindAllStringIndex(s, -1)
	var b strings.Builder
	prec := 0
	cambiato := false
	tratto := func(x string) {
		core := strings.TrimSpace(x)
		if core == "" || !tradurreFrase(core) {
			b.WriteString(x)
			return
		}
		t, ok := traduci(core)
		if !ok || t == "" {
			b.WriteString(x)
			return
		}
		cambiato = true
		b.WriteString(x[:len(x)-len(strings.TrimLeft(x, " \t"))])
		b.WriteString(t)
		b.WriteString(x[len(strings.TrimRight(x, " \t")):])
	}
	for _, p := range idx {
		tratto(s[prec:p[0]])
		b.WriteString(s[p[0]:p[1]])
		prec = p[1]
	}
	tratto(s[prec:])
	return b.String(), cambiato
}

func stessiPezzi(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

// ErrorePanico: una frase ha fatto andare in errore grave il traduttore (il lavoro si ferma, Tasso no).
type ErrorePanico struct {
	Valore any
	Stack  []byte
}

func (e *ErrorePanico) Error() string { return fmt.Sprint("errore interno: ", e.Valore) }

// TraduciLocres: traduce tutte le frasi (le uguali una volta sola). avanzamento(fatte, totale).
func TraduciLocres(files []LocresDaTradurre, traduci func(string) (string, bool), avanzamento func(int, int), stop func() bool) (tradotte int, err error) {
	uniche := map[string]string{}
	var ordine []string
	for _, f := range files {
		for _, s := range f.L.Frasi {
			if _, ok := uniche[s]; !ok && tradurreFrase(s) {
				uniche[s] = ""
				ordine = append(ordine, s)
			}
		}
	}
	// più frasi insieme (come la traduzione dello schermo): con migliaia di frasi una alla
	// volta servivano molti minuti (PC Cafe Simulator, 3.661 frasi, 24/11)
	const insieme = 6
	var (
		mu       sync.Mutex
		fatte    int
		ultimo   = time.Now()
		fermato  atomic.Bool
		panico   atomic.Pointer[ErrorePanico]
		prossima atomic.Int64
		wg       sync.WaitGroup
	)
	for w := 0; w < insieme; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				// un errore grave in una frase non deve chiudere Tasso: fermo il lavoro e lo dico
				if r := recover(); r != nil {
					panico.CompareAndSwap(nil, &ErrorePanico{Valore: r, Stack: debug.Stack()})
					fermato.Store(true)
				}
			}()
			for {
				if stop != nil && stop() {
					fermato.Store(true)
					return
				}
				i := int(prossima.Add(1) - 1)
				if i >= len(ordine) || fermato.Load() {
					return
				}
				s := ordine[i]
				t, ok := TraduciConPezziFissi(s, traduci)
				// sotto il lucchetto solo i conti: avanzamento si chiama fuori, così un suo errore
				// grave non lascia il lucchetto chiuso e gli altri fili non restano bloccati
				mu.Lock()
				if ok && t != "" {
					uniche[s] = t
					tradotte++
				}
				fatte++
				avvisa, n := false, fatte
				if avanzamento != nil && (time.Since(ultimo) > 500*time.Millisecond || fatte == len(ordine)) {
					avvisa, ultimo = true, time.Now()
				}
				mu.Unlock()
				if avvisa {
					avanzamento(n, len(ordine))
				}
			}
		}()
	}
	wg.Wait()
	if e := panico.Load(); e != nil {
		return tradotte, e
	}
	if fermato.Load() {
		return tradotte, errors.New("interrotto")
	}
	for _, f := range files {
		for k, s := range f.L.Frasi {
			if t := uniche[s]; t != "" {
				f.L.Frasi[k] = t
			}
		}
	}
	return tradotte, nil
}

// ScriviModUnreal: scrive Paks\~mods\zz_TassoTraduce_P.pak (prima in un file temporaneo).
// Restituisce il percorso relativo alla cartella del gioco e se ha creato lui ~mods.
func ScriviModUnreal(cartella, paks string, files []LocresDaTradurre) (rel string, creataMods bool, err error) {
	mods := filepath.Join(paks, "~mods")
	if r, e := filepath.Rel(cartella, filepath.Join(mods, nomeModUnreal)); e != nil || strings.HasPrefix(r, "..") || !senzaLink(cartella, r) || !senzaLink(cartella, r+".tmp") {
		return "", false, errors.New("nel percorso della mod c'è un link o una giunzione: per sicurezza non scrivo")
	}
	if _, e := os.Stat(mods); os.IsNotExist(e) {
		if err = os.MkdirAll(mods, 0o755); err != nil {
			return "", false, err
		}
		creataMods = true
	}
	var nuovi []unreal.FileNuovo
	for _, f := range files {
		nuovi = append(nuovi, unreal.FileNuovo{Nome: f.Nome, Dati: f.L.Scrivi()})
	}
	dest := filepath.Join(mods, nomeModUnreal)
	tmp := dest + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return "", creataMods, err
	}
	if err = unreal.ScriviPak(out, nuovi); err == nil {
		err = out.Sync()
	}
	if e := out.Close(); err == nil {
		err = e
	}
	if err == nil {
		// controllo: rileggo il pak appena scritto prima di metterlo al suo posto
		var p *unreal.Pak
		if p, err = unreal.ApriPak(tmp); err == nil {
			for _, f := range files {
				if _, ok := p.Trova(f.Nome); !ok {
					err = errors.New("il file scritto non è valido")
				}
			}
			p.Chiudi()
		}
	}
	if err == nil {
		err = os.Rename(tmp, dest)
	}
	if err != nil {
		_ = os.Remove(tmp)
		if creataMods {
			_ = os.Remove(mods)
		}
		return "", false, err
	}
	rel, _ = filepath.Rel(cartella, dest)
	return rel, creataMods, nil
}

// togliModUnreal: toglie il file aggiunto (e ~mods se l'ha creata Tasso ed è vuota).
func togliModUnreal(m ManifestoMod) error {
	for _, a := range m.Aggiunti {
		if filepath.IsAbs(a) || strings.Contains(a, "..") || !strings.EqualFold(filepath.Base(a), nomeModUnreal) {
			continue
		}
		if !senzaCollegamenti(m.Cartella, a) {
			return fmt.Errorf("non tolgo %s: la cartella passa da un collegamento", nomeModUnreal)
		}
		p := filepath.Join(m.Cartella, a)
		if !senzaLink(m.Cartella, a) || !senzaLink(m.Cartella, a+".tmp") {
			return fmt.Errorf("nel percorso di %s c'è un link o una giunzione: per sicurezza non lo tocco", nomeModUnreal)
		}
		_ = os.Remove(p + ".tmp") // rimasto se Tasso si era chiuso mentre lo scriveva
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return &errInUso{fmt.Sprintf("non sono riuscito a togliere %s (il gioco è aperto?)", nomeModUnreal)}
		}
		if m.CreataMods { // creata da Tasso
			_ = os.Remove(filepath.Dir(p)) // solo se vuota
		}
	}
	return nil
}
