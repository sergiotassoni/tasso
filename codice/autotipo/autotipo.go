// Package autotipo contiene le regole del "Compila" del Portachiavi che non dipendono da Windows,
// così si possono provare ovunque con go test:
//   - la sequenza dei tasti ({UTENTE}{TAB}{PASSWORD}{INVIO}, {ATTESA 2s}...);
//   - il riconoscimento del titolo della finestra ("Steam", "Accedi*Google");
//   - il confronto dei siti per dominio registrabile (accounts.google.com = google.com, ma
//     google.com.truffa.it NO): è la protezione contro le pagine false (phishing).
package autotipo

import (
	"crypto/rand"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// ---------------- Sequenza dei tasti ----------------

// Tipi di passo della sequenza.
const (
	Testo    = iota // testo fisso scritto così com'è
	Utente          // il nome utente della voce
	Password        // la password della voce
	Tab             // tasto Tab
	Invio           // tasto Invio
	Attesa          // pausa (Durata)
)

// Passo è un pezzo della sequenza.
type Passo struct {
	Tipo   int
	Testo  string
	Durata time.Duration
}

// SequenzaBase è quella usata quando la voce non ne ha una sua.
const SequenzaBase = "{UTENTE}{TAB}{PASSWORD}{INVIO}"

const (
	maxPassi      = 40
	maxAttesa     = 10 * time.Second
	maxAttesaTot  = 30 * time.Second
	maxTestoFisso = 200
)

var nomiPasso = map[string]int{
	"UTENTE": Utente, "USER": Utente, "USERNAME": Utente, "NOMEUTENTE": Utente,
	"PASSWORD": Password, "PW": Password,
	"TAB":   Tab,
	"INVIO": Invio, "ENTER": Invio,
}

// LeggiSequenza trasforma il testo della sequenza in passi. Vuota = SequenzaBase.
// Tra graffe vanno solo i nomi conosciuti; {{} e {}} scrivono le graffe.
func LeggiSequenza(s string) ([]Passo, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		s = SequenzaBase
	}
	var out []Passo
	var testo strings.Builder
	var totale time.Duration
	chiudiTesto := func() {
		if testo.Len() > 0 {
			out = append(out, Passo{Tipo: Testo, Testo: testo.String()})
			testo.Reset()
		}
	}
	nTesto := 0
	for i := 0; i < len(s); {
		c := s[i]
		if c == '}' {
			return nil, errors.New("Una graffa } è di troppo (per scriverla usa {}}).")
		}
		if c != '{' {
			testo.WriteByte(c)
			nTesto++
			i++
			continue
		}
		if strings.HasPrefix(s[i:], "{{}") {
			testo.WriteByte('{')
			nTesto++
			i += 3
			continue
		}
		if strings.HasPrefix(s[i:], "{}}") {
			testo.WriteByte('}')
			nTesto++
			i += 3
			continue
		}
		j := strings.IndexByte(s[i:], '}')
		if j < 0 {
			return nil, errors.New("Manca una graffa } di chiusura.")
		}
		dentro := strings.ToUpper(strings.TrimSpace(s[i+1 : i+j]))
		i += j + 1
		chiudiTesto()
		if t, ok := nomiPasso[strings.ReplaceAll(dentro, " ", "")]; ok {
			out = append(out, Passo{Tipo: t})
		} else if nome, arg, ok := strings.Cut(dentro, " "); ok && (nome == "ATTESA" || nome == "DELAY" || nome == "WAIT") {
			d, err := leggiDurata(arg)
			if err != nil {
				return nil, err
			}
			totale += d
			out = append(out, Passo{Tipo: Attesa, Durata: d})
		} else {
			return nil, errors.New("Nella sequenza c'è un nome che non conosco: {" + dentro + "}. Si possono usare {UTENTE}, {TAB}, {PASSWORD}, {INVIO} e {ATTESA 2s}.")
		}
		if len(out) > maxPassi {
			return nil, errors.New("La sequenza è troppo lunga.")
		}
	}
	chiudiTesto()
	if nTesto > maxTestoFisso {
		return nil, errors.New("La sequenza è troppo lunga.")
	}
	if totale > maxAttesaTot {
		return nil, errors.New("Le attese in tutto non possono superare 30 secondi.")
	}
	if len(out) == 0 {
		return nil, errors.New("La sequenza è vuota.")
	}
	return out, nil
}

// leggiDurata: "2", "2s", "1.5s", "500ms" (al massimo 10 secondi).
func leggiDurata(a string) (time.Duration, error) {
	a = strings.ToLower(strings.TrimSpace(a))
	molt := time.Second
	switch {
	case strings.HasSuffix(a, "ms"):
		molt, a = time.Millisecond, strings.TrimSuffix(a, "ms")
	case strings.HasSuffix(a, "s"):
		a = strings.TrimSuffix(a, "s")
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(a), ",", "."), 64)
	if err != nil || f <= 0 {
		return 0, errors.New("L'attesa va scritta così: {ATTESA 2s} (secondi) oppure {ATTESA 500ms}.")
	}
	d := time.Duration(f * float64(molt))
	if d > maxAttesa {
		return 0, errors.New("Un'attesa può durare al massimo 10 secondi.")
	}
	return d, nil
}

// HaInvio dice se la sequenza preme Invio (serve alla compilazione diretta dei campi).
func HaInvio(p []Passo) bool {
	for _, x := range p {
		if x.Tipo == Invio {
			return true
		}
	}
	return false
}

// SenzaUtente toglie {UTENTE} e il {TAB} subito dopo: per le voci senza nome utente.
func SenzaUtente(p []Passo) []Passo {
	var out []Passo
	for i := 0; i < len(p); i++ {
		if p[i].Tipo == Utente {
			if i+1 < len(p) && p[i+1].Tipo == Tab {
				i++
			}
			continue
		}
		out = append(out, p[i])
	}
	return out
}

// DalPassword: la parte della sequenza dal primo {PASSWORD} in poi (il campo davanti è già quello della password).
func DalPassword(p []Passo) []Passo {
	for i, x := range p {
		if x.Tipo == Password {
			return p[i:]
		}
	}
	return []Passo{{Tipo: Password}}
}

// ---------------- Titolo della finestra ----------------

// normTitolo: minuscole, trattini lunghi come quelli corti, spazi uniti.
func normTitolo(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		switch r {
		case '–', '—', '‒', '−':
			return '-'
		case '​', '‎', '‏':
			return -1
		}
		if unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// TitoloCorrisponde: il titolo della finestra va bene per lo schema salvato nella voce.
// Basta che il titolo contenga lo schema; dentro lo schema * vale come "qualsiasi testo";
// più schemi si separano con ; (basta che ne vada bene uno). Maiuscole e minuscole non contano.
func TitoloCorrisponde(schema, titolo string) bool {
	t := normTitolo(titolo)
	if t == "" {
		return false
	}
	for _, s := range strings.Split(schema, ";") {
		s = normTitolo(s)
		if strings.Trim(s, "* ") == "" {
			continue // "*" da solo andrebbe bene per tutte le finestre: non lo accetto
		}
		if !strings.Contains(s, "*") {
			if strings.Contains(t, s) {
				return true
			}
			continue
		}
		if conAsterischi("*"+s+"*", t) {
			return true
		}
	}
	return false
}

// conAsterischi: confronto con * = "qualsiasi testo" (anche vuoto) su tutto il titolo.
func conAsterischi(p, t string) bool {
	pi, ti, star, mark := 0, 0, -1, 0
	for ti < len(t) {
		switch {
		case pi < len(p) && p[pi] == '*':
			star, mark = pi, ti
			pi++
		case pi < len(p) && p[pi] == t[ti]:
			pi++
			ti++
		case star >= 0:
			pi = star + 1
			mark++
			ti = mark
		default:
			return false
		}
	}
	for pi < len(p) && p[pi] == '*' {
		pi++
	}
	return pi == len(p)
}

// ---------------- Siti e programmi ----------------

// Browser conosciuti: per loro conta l'indirizzo della pagina, mai il titolo o il nome del programma.
var Browser = map[string]bool{
	"chrome.exe": true, "msedge.exe": true, "firefox.exe": true, "brave.exe": true, "opera.exe": true,
	"vivaldi.exe": true, "iexplore.exe": true,
}

// Suffissi a due livelli più comuni (co.uk, com.br...): il dominio registrabile ha un pezzo in più.
var suffissiDoppi = map[string]bool{
	"co.uk": true, "org.uk": true, "ac.uk": true, "gov.uk": true, "me.uk": true, "ltd.uk": true, "plc.uk": true,
	"com.au": true, "net.au": true, "org.au": true, "co.nz": true, "co.jp": true, "ne.jp": true, "or.jp": true,
	"com.br": true, "com.mx": true, "com.ar": true, "com.tr": true, "com.cn": true, "com.hk": true, "com.tw": true,
	"com.sg": true, "co.in": true, "co.za": true, "co.kr": true, "com.pl": true, "com.ua": true, "co.il": true,
	"com.es": true, "com.pt": true, "com.ru": true, "gov.it": true, "edu.it": true,
	"org.br": true, "net.br": true, "gov.br": true, "com.co": true, "com.vn": true, "co.id": true,
	"com.my": true, "com.ph": true, "com.sa": true, "com.eg": true, "com.ng": true, "co.th": true,
	"ac.jp": true, "go.jp": true, "gob.mx": true, "gob.es": true, "org.es": true, "co.at": true,
	"or.at": true, "com.pe": true, "com.ve": true, "com.uy": true, "com.cl": true, "org.pl": true,
	"net.pl": true, "co.ke": true, "com.pk": true, "com.bd": true, "co.ve": true, "org.mx": true,
	// Siti di hosting dove ognuno ha il suo sottodominio (alice.github.io e truffa.github.io sono
	// due siti diversi): qui il sito vero è il pezzo prima del suffisso.
	"github.io": true, "gitlab.io": true, "blogspot.com": true, "wordpress.com": true,
	"herokuapp.com": true, "netlify.app": true, "vercel.app": true, "pages.dev": true,
	"workers.dev": true, "web.app": true, "firebaseapp.com": true, "azurewebsites.net": true,
	"cloudfront.net": true, "appspot.com": true, "glitch.me": true, "onrender.com": true,
	"fly.dev": true, "ngrok.io": true, "ngrok-free.app": true, "repl.co": true, "replit.app": true,
	"myshopify.com": true, "wixsite.com": true, "weebly.com": true, "altervista.org": true,
	"duckdns.org": true, "no-ip.org": true, "000webhostapp.com": true, "sharepoint.com": true,
	"s3.amazonaws.com": true, "azurestaticapps.net": true, "surge.sh": true, "neocities.org": true,
}

// NormHost: dall'indirizzo (anche senza https://) al nome del sito in minuscolo, senza porta né www.
func NormHost(addr string) string {
	a := strings.TrimSpace(strings.ToLower(addr))
	if a == "" || strings.ContainsAny(a, " \t") && !strings.Contains(a, "://") {
		return ""
	}
	// "://" conta solo se viene prima di "/", "?" o "#": Chrome mostra l'indirizzo senza
	// "https://", e in "finto-sito.com/login?r=https://www.amazon.it/" il sito è finto-sito.com,
	// non amazon.it (altrimenti una pagina falsa potrebbe farsi passare per un'altra).
	if i := strings.Index(a, "://"); i >= 0 {
		if j := strings.IndexAny(a, "/?#\\"); j < 0 || i <= j {
			a = a[i+3:]
		}
	}
	// "\" vale come "/" (i browser lo trattano così negli indirizzi web): in
	// "evil.com\@amazon.it" il sito è evil.com, non amazon.it.
	if i := strings.IndexAny(a, "/?#\\"); i >= 0 {
		a = a[:i]
	}
	if i := strings.LastIndex(a, "@"); i >= 0 {
		a = a[i+1:] // utente@sito: conta il sito
	}
	if strings.HasPrefix(a, "[") { // IPv6
		if j := strings.Index(a, "]"); j > 0 {
			return a[1:j]
		}
		return ""
	}
	if h, _, err := net.SplitHostPort(a); err == nil {
		a = h
	}
	a = strings.TrimSuffix(a, ".")
	a = strings.TrimPrefix(a, "www.")
	if a == "" || strings.ContainsAny(a, " \t\"'<>\\,;") {
		return ""
	}
	if net.ParseIP(a) != nil || a == "localhost" {
		return a
	}
	if !strings.Contains(a, ".") {
		return ""
	}
	for _, l := range strings.Split(a, ".") {
		if l == "" || len(l) > 63 || strings.HasPrefix(l, "-") || strings.HasSuffix(l, "-") {
			return ""
		}
		for _, r := range l {
			if !(r == '-' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 127) {
				return ""
			}
		}
	}
	// l'ultimo pezzo (it, com...) non può essere fatto solo di numeri
	if last := a[strings.LastIndex(a, ".")+1:]; strings.Trim(last, "0123456789") == "" {
		return ""
	}
	return a
}

// Registrabile: il dominio "vero" del sito (accounts.google.com -> google.com, www.bbc.co.uk -> bbc.co.uk).
func Registrabile(host string) string {
	h := NormHost(host)
	if h == "" || net.ParseIP(h) != nil || h == "localhost" {
		return h
	}
	l := strings.Split(h, ".")
	n := 2
	for _, k := range []int{3, 2} { // prima il suffisso più lungo (s3.amazonaws.com)
		if len(l) > k && suffissiDoppi[strings.Join(l[len(l)-k:], ".")] {
			n = k + 1
			break
		}
	}
	if len(l) <= n {
		return h
	}
	return strings.Join(l[len(l)-n:], ".")
}

// StessoSito: i due indirizzi sono dello stesso sito (stesso dominio registrabile).
func StessoSito(a, b string) bool {
	ra, rb := Registrabile(a), Registrabile(b)
	return ra != "" && ra == rb
}

// Nomi comuni scritti come "Sito o programma" (Steam, Gmail...): i siti e i programmi veri a cui
// corrispondono. Solo indirizzi ufficiali scritti qui a mano: mai indovinati dal nome.
var alias = map[string][]string{
	"google":    {"google.com", "google.it", "youtube.com"},
	"gmail":     {"google.com", "gmail.com"},
	"youtube":   {"google.com", "youtube.com"},
	"steam":     {"steampowered.com", "steamcommunity.com", "steam.exe", "steamwebhelper.exe"},
	"epic":      {"epicgames.com", "epicgameslauncher.exe"},
	"epicgames": {"epicgames.com", "epicgameslauncher.exe"},
	"microsoft": {"live.com", "microsoft.com", "microsoftonline.com", "xbox.com"},
	"outlook":   {"live.com", "microsoft.com", "outlook.com", "microsoftonline.com"},
	"hotmail":   {"live.com", "microsoft.com", "outlook.com"},
	"xbox":      {"live.com", "microsoft.com", "xbox.com"},
	"riot":      {"riotgames.com", "riotclientux.exe", "riotclientservices.exe"},
	"riotgames": {"riotgames.com", "riotclientux.exe", "riotclientservices.exe"},
	"battlenet": {"battle.net", "blizzard.com", "battle.net.exe"},
	"blizzard":  {"battle.net", "blizzard.com", "battle.net.exe"},
	"ubisoft":   {"ubisoft.com", "ubisoftconnect.com", "upc.exe", "ubisoftconnect.exe"},
	"ea":        {"ea.com", "eadesktop.exe"},
	"discord":   {"discord.com", "discord.exe"},
	"twitch":    {"twitch.tv"},
	"amazon":    {"amazon.it", "amazon.com", "amazon.de", "amazon.fr", "amazon.es", "amazon.co.uk"},
	"facebook":  {"facebook.com"},
	"instagram": {"instagram.com"},
	"netflix":   {"netflix.com"},
	"paypal":    {"paypal.com"},
	"apple":     {"apple.com", "icloud.com"},
	"icloud":    {"apple.com", "icloud.com"},
	"spotify":   {"spotify.com", "spotify.exe"},
	"twitter":   {"twitter.com", "x.com"},
	"x":         {"x.com", "twitter.com"},
	"tiktok":    {"tiktok.com"},
	"ebay":      {"ebay.it", "ebay.com"},
	"github":    {"github.com"},
	"gog":       {"gog.com", "galaxyclient.exe"},
	"rockstar":  {"rockstargames.com", "launcher.exe"},
}

func soloLettere(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Voce: i dati (non segreti) di una voce del portachiavi che servono per riconoscerla.
type Voce struct {
	Site string // "Sito o programma" (es. Steam, google.com)
	Web  string // indirizzi o programmi, separati da spazi (es. "accounts.google.com steam.exe")
	Win  string // titolo della finestra (schema)
}

// Chiavi: i siti (dominio registrabile) e i programmi (nome .exe) della voce.
func (v Voce) Chiavi() (siti []string, exe []string) {
	visto := map[string]bool{}
	agg := func(x string) {
		x = strings.ToLower(strings.TrimSpace(x))
		if x == "" {
			return
		}
		if strings.HasSuffix(x, ".exe") && !strings.ContainsAny(x, "/") {
			x = x[strings.LastIndexAny(x, `\`)+1:]
			if !visto["exe:"+x] && !Browser[x] {
				visto["exe:"+x] = true
				exe = append(exe, x)
			}
			return
		}
		if r := Registrabile(x); r != "" {
			if !visto[r] {
				visto[r] = true
				siti = append(siti, r)
			}
			return
		}
		for _, a := range alias[soloLettere(x)] {
			if strings.HasSuffix(a, ".exe") {
				if !visto["exe:"+a] {
					visto["exe:"+a] = true
					exe = append(exe, a)
				}
			} else if r := Registrabile(a); r != "" && !visto[r] {
				visto[r] = true
				siti = append(siti, r)
			}
		}
	}
	for _, w := range strings.FieldsFunc(v.Web, func(r rune) bool { return r == ' ' || r == ',' || r == ';' || r == '\n' || r == '\t' }) {
		agg(w)
	}
	agg(v.Site)
	return siti, exe
}

// Contesto: dove sta per compilare Tasso.
type Contesto struct {
	Host  string // indirizzo della pagina (solo nei browser)
	Exe   string // nome del programma, in minuscolo (es. steam.exe)
	Title string // titolo della finestra
}

// Corrisponde: la voce va bene per questo contesto?
// Nei browser conta solo il dominio della pagina (protezione dalle pagine false); un browser di
// cui non riesco a leggere l'indirizzo non corrisponde a niente. Nei programmi contano il nome
// del programma o il titolo della finestra salvato nella voce.
func Corrisponde(v Voce, c Contesto) bool {
	exe := strings.ToLower(c.Exe)
	siti, exes := v.Chiavi()
	if Browser[exe] || c.Host != "" {
		h := Registrabile(c.Host)
		if h == "" {
			return false
		}
		for _, s := range siti {
			if s == h {
				return true
			}
		}
		return false
	}
	for _, e := range exes {
		if e == exe {
			return true
		}
	}
	return strings.TrimSpace(v.Win) != "" && TitoloCorrisponde(v.Win, c.Title)
}

// TogliBrowser: dal titolo di una finestra del browser toglie " - Google Chrome" e simili.
func TogliBrowser(t string) string {
	n := normTitolo(t)
	for _, s := range []string{" - google chrome", " - microsoft edge", " - microsoft​ edge", " - mozilla firefox", " - brave", " - opera", " - vivaldi"} {
		if i := strings.LastIndex(n, s); i >= 0 {
			n = n[:i]
			break
		}
	}
	// Edge: "Pagina e altre 3 pagine - Personale"
	if i := strings.Index(n, " e altre "); i > 0 {
		n = n[:i]
	}
	if i := strings.Index(n, " and "); i > 0 && strings.Contains(n[i:], " more page") {
		n = n[:i]
	}
	return strings.TrimSpace(n)
}

// CampoAccesso: dal nome o dall'identificativo di un campo di testo capisce se è un campo di accesso
// (utente, email, account). Il campo password si riconosce a parte (Windows lo segnala).
func CampoAccesso(nome, id string) bool {
	s := strings.ToLower(nome + " " + id)
	for _, k := range []string{"email", "e-mail", "mail", "user", "utente", "login", "account", "accedi", "sign in", "signin", "username", "identifier", "telefono", "phone", "nickname", "id utente"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// NomeDaSito: il nome proposto per un account nuovo, preso dal sito ("accedi.esempio.it" → "Esempio").
func NomeDaSito(host string) string {
	r := Registrabile(host)
	if r == "" || net.ParseIP(r) != nil {
		return r
	}
	nome := r
	if i := strings.Index(r, "."); i > 0 {
		nome = r[:i]
	}
	rs := []rune(nome)
	rs[0] = unicode.ToUpper(rs[0])
	return string(rs)
}

// CampoRegistrazione: il campo password è di una pagina per creare un account (e non di un
// accesso)? Sì se nella pagina ci sono almeno due campi password (password e conferma) o se il
// nome del campo o il titolo della pagina parlano di account nuovo.
func CampoRegistrazione(nome, id, titolo string, campiPassword int) bool {
	campo := strings.ToLower(nome + " " + id)
	for _, k := range []string{"attuale", "current", "vecchia", "old password", "oldpassword", "actual", "actuel", "aktuell"} {
		if strings.Contains(campo, k) {
			return false // "password attuale" in un cambio password: quella si compila, non si crea
		}
	}
	if campiPassword >= 2 {
		return true
	}
	s := strings.ToLower(nome + " " + id + " " + TogliBrowser(titolo))
	for _, k := range []string{
		"new-password", "new password", "newpassword", "nuova password", "crea password", "crea una password",
		"conferma", "confirm", "registra", "registrati", "registrazione", "iscriviti", "crea account",
		"crea un account", "crea il tuo account", "sign up", "signup", "register", "create account",
		"create your account", "create password", "nueva contraseña", "crear cuenta", "regístrate",
		"nouveau mot de passe", "créer un compte", "inscription", "neues passwort", "konto erstellen", "registrieren",
	} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// GeneraPassword crea una password casuale vera (crypto/rand) di n caratteri, con almeno una
// minuscola, una maiuscola, un numero (e un simbolo se richiesto), senza caratteri che si
// confondono (l, I, O, 0, 1). Torna UTF-16 con lo zero finale, così le copie si possono azzerare.
func GeneraPassword(n int, simboli bool) []uint16 {
	if n < 8 {
		n = 8
	}
	if n > 64 {
		n = 64
	}
	gruppi := []string{"abcdefghijkmnopqrstuvwxyz", "ABCDEFGHJKLMNPQRSTUVWXYZ", "23456789"}
	if simboli {
		gruppi = append(gruppi, "!#$%&*?-_+=@")
	}
	tutti := strings.Join(gruppi, "")
	caso := func(m int) int {
		var b [1]byte
		lim := 256 - 256%m // niente sbilanciamenti
		for {
			if _, err := rand.Read(b[:]); err != nil {
				panic("crypto/rand non disponibile")
			}
			if int(b[0]) < lim {
				return int(b[0]) % m
			}
		}
	}
	out := make([]uint16, n+1)
	for {
		for i := 0; i < n; i++ {
			out[i] = uint16(tutti[caso(len(tutti))])
		}
		ok := true
		for _, g := range gruppi {
			c := false
			for i := 0; i < n && !c; i++ {
				c = strings.IndexByte(g, byte(out[i])) >= 0
			}
			ok = ok && c
		}
		if ok {
			return out
		}
	}
}
