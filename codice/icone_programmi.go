package main

// Icone vere nelle liste "Aggiornamenti dei programmi" (winget) e "App preinstallate" (Store).
//
// L'interfaccia non manda percorsi: è Go a sapere dove guardare.
//   - Programmi winget: la voce del programma nelle chiavi Uninstall del registro (HKLM 64 e 32 bit,
//     HKCU), poi DisplayIcon ("file,indice") o il programma principale nella cartella InstallLocation.
//   - App dello Store: il logo scritto nel manifest del pacchetto (Square44x44Logo), cercato SOLO
//     dentro la cartella del pacchetto, senza seguire collegamenti o giunzioni.
//
// La lista compare subito; le icone arrivano dopo, una alla volta in background, con onIcons.
// Se un'icona non c'è, l'interfaccia mostra l'iniziale del nome nel cerchietto colorato.

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	pPrivateExtractIcons = user32.NewProc("PrivateExtractIconsW")
	pGetDpiForSystem     = user32.NewProc("GetDpiForSystem")

	icoProgMu     sync.Mutex
	icoProgCache  = map[string]string{} // chiave della riga -> data URL ("" = niente icona: iniziale)
	icoProgLavoro sync.Mutex            // una ricerca di icone alla volta
)

const (
	icoMaxLogo  = 256 << 10 // un logo PNG più grande di così non è un logo
	icoMaxIco   = 1 << 20   // file .ico
	icoMaxManif = 1 << 20   // AppxManifest.xml
	icoMaxFile  = 2000      // file letti in una cartella del logo
)

// rigaIcona: una riga della lista con la chiave che usa l'interfaccia (data-ik).
type rigaIcona struct {
	chiave, nome, id string
}

// iconeAggiornamenti: dopo onUpdates, cerca in background le icone dei programmi della lista.
func iconeAggiornamenti(res map[string]interface{}) {
	items, _ := res["items"].([]interface{})
	var righe []rigaIcona
	for _, it := range items {
		m, _ := it.(map[string]interface{})
		nome, _ := m["name"].(string)
		id, _ := m["id"].(string)
		if nome == "" && id == "" {
			continue
		}
		righe = append(righe, rigaIcona{chiaveAggiornamento(id, nome), nome, id})
	}
	avviaIcone(righe, false)
}

// iconeAppPreinstallate: dopo onBloat, i loghi delle app trovate (id = nome del pacchetto, anche con *).
func iconeAppPreinstallate(list []interface{}) {
	var righe []rigaIcona
	for _, it := range list {
		m, _ := it.(map[string]interface{})
		id, _ := m["id"].(string)
		if id == "" {
			continue
		}
		nome, _ := m["name"].(string)
		righe = append(righe, rigaIcona{"b:" + id, nome, id})
	}
	avviaIcone(righe, true)
}

func avviaIcone(righe []rigaIcona, appx bool) {
	if len(righe) == 0 {
		return
	}
	go func() {
		defer guard("icone_programmi.go:avviaIcone")
		icoProgLavoro.Lock()
		defer icoProgLavoro.Unlock()
		cercaIcone(righe, appx)
	}()
}

func cercaIcone(righe []rigaIcona, appx bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// SHGetFileInfo (riserva di iconFrom) vuole COM inizializzato sul thread.
	_ = windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)
	defer windows.CoUninitialize()

	grande := latoIcona() > 32
	out := map[string]string{}
	invia := func() {
		if len(out) > 0 {
			emitRaw("onIcons", out)
			out = map[string]string{}
		}
	}
	var voci []voceDisinst
	var pacchetti []pacchettoAppx
	letti := false
	for _, r := range righe {
		icoProgMu.Lock()
		c, ok := icoProgCache[r.chiave]
		icoProgMu.Unlock()
		if !ok {
			if !letti {
				letti = true
				if appx {
					pacchetti = leggiPacchettiAppx()
				} else {
					for _, e := range readUninstall() {
						voci = append(voci, voceDisinst{chiave: e.key, nome: e.name, loc: e.loc, icona: e.icon})
					}
				}
			}
			c = func() (s string) {
				defer func() {
					if recover() != nil {
						s = ""
					}
				}()
				if appx {
					return iconaAppx(r.id, pacchetti, grande)
				}
				return iconaWinget(r.nome, r.id, voci, grande)
			}()
			icoProgMu.Lock()
			if len(icoProgCache) > 2000 {
				icoProgCache = map[string]string{}
			}
			icoProgCache[r.chiave] = c
			icoProgMu.Unlock()
		}
		out[r.chiave] = c
		if len(out) >= 8 {
			invia()
		}
	}
	invia()
}

// latoIcona: 32 pixel, 64 sugli schermi con il ridimensionamento sopra il 100% (l'icona resta nitida).
func latoIcona() int {
	if pGetDpiForSystem.Find() == nil {
		if dpi, _, _ := pGetDpiForSystem.Call(); dpi > 96 {
			return 64
		}
	}
	return 32
}

// ---------------- programmi winget ----------------

// iconaWinget: per le app MSIX/Store (Id `MSIX\<FullName>`) prima il logo del pacchetto; per gli altri
// prima la voce Uninstall e, se non dà un'icona (niente voce o niente DisplayIcon), il logo del
// pacchetto Appx con lo stesso nome.
func iconaWinget(nome, id string, voci []voceDisinst, grande bool) string {
	_, _, msix := pacchettoDaIdMSIX(id)
	if msix {
		if s := iconaAppxWinget(nome, id, grande); s != "" {
			return s
		}
	}
	if v, ok := trovaVoceWinget(nome, id, voci); ok {
		lato := 32
		if grande {
			lato = 64
		}
		for _, f := range fontiIcona(v) {
			if s := estraiIcona(f.percorso, f.indice, lato); s != "" {
				return s
			}
		}
	}
	if !msix {
		return iconaAppxWinget(nome, id, grande)
	}
	return ""
}

type fonteIcona struct {
	percorso string
	indice   int
}

// fontiIcona: prima DisplayIcon, poi il programma principale della cartella di installazione.
func fontiIcona(v voceDisinst) []fonteIcona {
	var out []fonteIcona
	if p, i, ok := dividiIconaReg(expandEnv(v.icona)); ok && percorsoIconaOk(p) {
		out = append(out, fonteIcona{p, i})
	}
	if loc := strings.Trim(strings.TrimSpace(expandEnv(v.loc)), `"`); percorsoDiscoLocale(loc) {
		if st, err := os.Lstat(loc); err == nil && st.IsDir() && !èLink(st) {
			if e := dirExe(loc); e != "" && percorsoIconaOk(e) {
				out = append(out, fonteIcona{e, 0})
			}
		}
	}
	return out
}

// percorsoIconaOk: un file vero su un disco del PC (niente rete, niente collegamenti), di tipo icona.
func percorsoIconaOk(p string) bool {
	if !percorsoDiscoLocale(p) {
		return false
	}
	low := strings.ToLower(p)
	ext := filepath.Ext(low)
	if ext != ".exe" && ext != ".dll" && ext != ".ico" {
		return false
	}
	base := filepath.Base(low)
	if base == "msiexec.exe" || base == "rundll32.exe" || base == "explorer.exe" || base == "cmd.exe" {
		return false // icona di Windows, non del programma
	}
	st, err := os.Lstat(p)
	if err != nil || !st.Mode().IsRegular() || èLink(st) {
		return false
	}
	if ext == ".ico" && st.Size() > icoMaxIco {
		return false
	}
	return true
}

// estraiIcona: l'icona alla misura giusta (PrivateExtractIcons), con riserva su iconFrom (ExtractIconEx e
// SHGetFileInfo). Sempre un PNG fatto da Go, mai il file così com'è.
func estraiIcona(p string, indice, lato int) string {
	p16, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return ""
	}
	var h uintptr
	var idIcona uint32
	n, _, _ := pPrivateExtractIcons.Call(uintptr(unsafe.Pointer(p16)), uintptr(int32(indice)), uintptr(lato), uintptr(lato),
		uintptr(unsafe.Pointer(&h)), uintptr(unsafe.Pointer(&idIcona)), 1, 0)
	if n != 0 && n != 0xFFFFFFFF && h != 0 {
		s := iconPNG(h)
		pDestroyIcon.Call(h)
		if s != "" {
			return s
		}
	}
	return iconFrom(p + "," + strconv.Itoa(indice))
}

// ---------------- app dello Store ----------------

const chiaveRepoAppx = `Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppModel\Repository\Packages`

var pGetPackagePathByFullName = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetPackagePathByFullName")

// leggiPacchettiAppx: i pacchetti dell'utente come li vede Get-AppxPackage (registro dell'utente),
// con la loro cartella. Senza PowerShell.
func leggiPacchettiAppx() []pacchettoAppx {
	k, err := registry.OpenKey(registry.CURRENT_USER, chiaveRepoAppx, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil
	}
	nomi, _ := k.ReadSubKeyNames(-1)
	k.Close()
	var out []pacchettoAppx
	for _, full := range nomi {
		nome, ver, risorsa, ok := dividiNomeCompleto(full)
		if !ok || risorsa {
			continue // i pacchetti di sole risorse (lingue, misure) non hanno il manifest con il logo
		}
		dir, vis := "", ""
		if sk, err := registry.OpenKey(registry.CURRENT_USER, chiaveRepoAppx+`\`+full, registry.QUERY_VALUE); err == nil {
			dir, _, _ = sk.GetStringValue("PackageRootFolder")
			vis, _, _ = sk.GetStringValue("DisplayName")
			sk.Close()
		}
		if dir == "" {
			dir = cartellaPacchetto(full)
		}
		if dir == "" {
			continue
		}
		out = append(out, pacchettoAppx{full, nome, filepath.Clean(dir), ver, vis})
	}
	return out
}

func cartellaPacchetto(full string) string {
	if pGetPackagePathByFullName.Find() != nil {
		return ""
	}
	f16, err := windows.UTF16PtrFromString(full)
	if err != nil {
		return ""
	}
	n := uint32(windows.MAX_PATH)
	buf := make([]uint16, n)
	if r, _, _ := pGetPackagePathByFullName.Call(uintptr(unsafe.Pointer(f16)), uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&buf[0]))); r != 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

// Pacchetti Appx per gli aggiornamenti: letti dal registro una volta sola e tenuti qualche minuto,
// insieme ai nomi visibili già risolti (SHLoadIndirectString, niente PowerShell).
var (
	pSHLoadIndirectString = windows.NewLazySystemDLL("shlwapi.dll").NewProc("SHLoadIndirectString")

	appxAggMu     sync.Mutex
	appxAggLista  []pacchettoAppx
	appxAggQuando time.Time
	appxAggNomi   = map[string]string{} // nome completo -> nome visibile risolto
)

func pacchettiAggiornamenti() []pacchettoAppx {
	appxAggMu.Lock()
	defer appxAggMu.Unlock()
	if appxAggLista == nil || time.Since(appxAggQuando) > 5*time.Minute {
		appxAggLista = leggiPacchettiAppx()
		if appxAggLista == nil {
			appxAggLista = []pacchettoAppx{}
		}
		appxAggQuando = time.Now()
	}
	return appxAggLista
}

// nomeVisibileAppx: il DisplayName del pacchetto nella lingua dell'utente ("Programma di installazione app").
func nomeVisibileAppx(pk pacchettoAppx) string {
	appxAggMu.Lock()
	v, ok := appxAggNomi[pk.nomeCompleto]
	appxAggMu.Unlock()
	if ok {
		return v
	}
	v = strings.TrimSpace(pk.nomeVisibile)
	if strings.HasPrefix(v, "@") {
		v = ""
		if pSHLoadIndirectString.Find() == nil {
			if src, err := windows.UTF16PtrFromString(strings.TrimSpace(pk.nomeVisibile)); err == nil {
				buf := make([]uint16, 512)
				if r, _, _ := pSHLoadIndirectString.Call(uintptr(unsafe.Pointer(src)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0); r == 0 {
					v = strings.TrimSpace(windows.UTF16ToString(buf))
				}
			}
		}
		if strings.HasPrefix(v, "@") || strings.HasPrefix(strings.ToLower(v), "ms-resource:") {
			v = ""
		}
	}
	appxAggMu.Lock()
	if len(appxAggNomi) > 4000 {
		appxAggNomi = map[string]string{}
	}
	appxAggNomi[pk.nomeCompleto] = v
	appxAggMu.Unlock()
	return v
}

// iconaAppxWinget: il logo del manifest del pacchetto Appx che corrisponde a una riga di winget.
func iconaAppxWinget(nome, id string, grande bool) string {
	for _, pk := range pacchettiPerWinget(nome, id, pacchettiAggiornamenti(), nomeVisibileAppx) {
		if s := logoPacchetto(pk.cartella, grande); s != "" {
			return s
		}
	}
	return ""
}

func iconaAppx(id string, pacchetti []pacchettoAppx, grande bool) string {
	for _, pk := range pacchettiPerID(id, pacchetti) {
		if s := logoPacchetto(pk.cartella, grande); s != "" {
			return s
		}
	}
	return ""
}

// logoPacchetto: legge il manifest e il file del logo, tutto dentro la cartella del pacchetto.
func logoPacchetto(root string, grande bool) string {
	if !percorsoDiscoLocale(root) {
		return ""
	}
	if st, err := os.Lstat(root); err != nil || !st.IsDir() || èLink(st) {
		return ""
	}
	man, ok := leggiDentro(root, filepath.Join(root, "AppxManifest.xml"), icoMaxManif)
	if !ok {
		return ""
	}
	for _, rel := range logoDaManifest(string(man)) {
		dir := filepath.Join(root, filepath.Dir(rel))
		if !dentroDi(dir, root) || !senzaLinkFino(root, dir) {
			continue
		}
		ents, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		var nomi []string
		for i, e := range ents {
			if i >= icoMaxFile {
				break
			}
			if e.Type().IsRegular() {
				nomi = append(nomi, e.Name())
			}
		}
		base := filepath.Base(rel)
		ext := filepath.Ext(base)
		for _, scelto := range scegliLogo(nomi, strings.TrimSuffix(base, ext), ext, grande) {
			b, ok := leggiDentro(root, filepath.Join(dir, scelto), icoMaxLogo)
			if !ok {
				continue
			}
			if s := pngSicuro(b); s != "" {
				return s
			}
		}
	}
	return ""
}

// leggiDentro: legge p solo se sta dentro root, è un file normale (niente collegamenti, né lui né le
// cartelle in mezzo) e non supera max byte.
func leggiDentro(root, p string, max int64) ([]byte, bool) {
	root, p = filepath.Clean(root), filepath.Clean(p)
	if !dentroDi(p, root) || strings.EqualFold(p, root) || !senzaLinkFino(root, p) {
		return nil, false
	}
	st, err := os.Lstat(p)
	if err != nil || !st.Mode().IsRegular() || èLink(st) || st.Size() > max {
		return nil, false
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil || len(b) == 0 || int64(len(b)) > max {
		return nil, false
	}
	// Controllo finale: il file aperto è davvero lì dentro (non portato altrove da una giunzione) e non
	// è in rete.
	fin := percorsoFinale(windows.Handle(f.Fd()))
	if fin == "" {
		return nil, false
	}
	if !dentroDi(fin, root) {
		rf, err := filepath.EvalSymlinks(root)
		if err != nil || !dentroDi(fin, filepath.Clean(rf)) {
			return nil, false
		}
	}
	return b, true
}

// senzaLinkFino: nessuna cartella tra root e p (p compreso) è un collegamento o una giunzione.
func senzaLinkFino(root, p string) bool {
	pezzi, ok := sottoApp(root, p)
	if !ok {
		return false
	}
	cur := filepath.Clean(root)
	for _, s := range pezzi {
		cur = filepath.Join(cur, s)
		fi, err := os.Lstat(cur)
		if err != nil || èLink(fi) {
			return false
		}
	}
	return true
}

// pngSicuro: solo un PNG vero e piccolo; lo ridisegno con Go, così all'interfaccia arriva un'immagine pulita.
func pngSicuro(b []byte) string {
	cfg, err := png.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 512 || cfg.Height > 512 {
		return ""
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil || vuota(img) {
		return ""
	}
	var o bytes.Buffer
	if png.Encode(&o, img) != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(o.Bytes())
}

// vuota: tutta trasparente (si vedrebbe un buco al posto dell'icona).
func vuota(img image.Image) bool {
	r := img.Bounds()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0x1000 {
				return false
			}
		}
	}
	return true
}

// ---------------- logica pura (provata nei test) ----------------

// chiaveAggiornamento: la stessa che usa l'interfaccia in onUpdates.
func chiaveAggiornamento(id, nome string) string {
	if id != "" {
		return "u:" + id
	}
	return "u:n:" + nome
}

// voceDisinst: una voce delle chiavi Uninstall del registro.
type voceDisinst struct {
	chiave, nome, loc, icona string
}

// percorsoDiscoLocale: "C:\..." su un disco del PC. Niente rete (\\server), niente \\?\, niente flussi
// nascosti (file:flusso), niente "..".
func percorsoDiscoLocale(p string) bool {
	if len(p) < 4 || p[1] != ':' || (p[2] != '\\' && p[2] != '/') {
		return false
	}
	c := p[0] | 0x20
	if c < 'a' || c > 'z' {
		return false
	}
	if strings.ContainsAny(p[2:], ":*?\"<>|") {
		return false
	}
	for _, s := range strings.FieldsFunc(p, func(r rune) bool { return r == '\\' || r == '/' }) {
		if s == ".." {
			return false
		}
	}
	return true
}

// dividiIconaReg: DisplayIcon ("C:\x\a.exe,-101", "\"C:\x\a.exe\"", "C:\x\a.ico") -> percorso e indice.
func dividiIconaReg(s string) (string, int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", 0, false
	}
	idx := 0
	if i := strings.LastIndex(s, ","); i > 0 {
		if n, err := strconv.Atoi(strings.TrimSpace(s[i+1:])); err == nil {
			idx, s = n, strings.TrimSpace(s[:i])
		}
	}
	s = strings.Trim(s, `"`)
	if s == "" {
		return "", 0, false
	}
	return s, idx, true
}

var (
	versioneRe = regexp.MustCompile(`^(v|ver\.?)?\d+([.\-+_]\d\w*)*$`)
	archRe     = regexp.MustCompile(`(?i)^\(?(x64|x86|64-bit|32-bit|amd64|arm64|64 bit|32 bit)\)?$`)
)

// normaNome: minuscole, solo lettere e cifre ("Corsair iCUE5 Software" -> "corsairicue5software").
func normaNome(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// nomeSenzaVersione: "Bionic 1.2.0+2" -> "bionic", "7-Zip 24.08 (x64)" -> "7zip".
func nomeSenzaVersione(s string) string {
	s = strings.TrimRight(strings.TrimSpace(s), "…")
	var keep []string
	for _, t := range strings.Fields(s) {
		if versioneRe.MatchString(t) || archRe.MatchString(t) {
			continue
		}
		keep = append(keep, t)
	}
	return normaNome(strings.Join(keep, " "))
}

// trovaVoceWinget: la voce del registro che corrisponde a una riga di winget (nome e ID).
// Dal più sicuro al meno sicuro; a parità vince la voce che ha un'icona.
func trovaVoceWinget(nome, id string, voci []voceDisinst) (voceDisinst, bool) {
	scegli := func(ok func(v voceDisinst) bool) (voceDisinst, bool) {
		var trovate []voceDisinst
		for _, v := range voci {
			if ok(v) {
				trovate = append(trovate, v)
			}
		}
		if len(trovate) == 0 {
			return voceDisinst{}, false
		}
		sort.SliceStable(trovate, func(i, j int) bool { return trovate[i].icona != "" && trovate[j].icona == "" })
		return trovate[0], true
	}
	// 1. ID di winget per i programmi senza pacchetto: "ARP\Machine\X64\{GUID}" = nome della chiave.
	if strings.HasPrefix(strings.ToUpper(id), `ARP\`) {
		ult := id[strings.LastIndex(id, `\`)+1:]
		if v, ok := scegli(func(v voceDisinst) bool {
			return ult != "" && strings.EqualFold(v.chiave[strings.LastIndex(v.chiave, `\`)+1:], ult)
		}); ok {
			return v, true
		}
	}
	tronco := strings.HasSuffix(strings.TrimSpace(nome), "…")
	n := normaNome(strings.TrimRight(strings.TrimSpace(nome), "…"))
	if n != "" && !tronco {
		// 2. Stesso nome.
		if v, ok := scegli(func(v voceDisinst) bool { return normaNome(v.nome) == n }); ok {
			return v, true
		}
	}
	// 3. Stesso nome senza numeri di versione e architettura.
	if nv := nomeSenzaVersione(nome); len(nv) >= 3 && !tronco {
		if v, ok := scegli(func(v voceDisinst) bool { return nomeSenzaVersione(v.nome) == nv }); ok {
			return v, true
		}
	}
	// 4. Nome accorciato da winget ("Microsoft Visual C++ 2015-2…"): la voce che comincia così.
	if tronco && len(n) >= 4 {
		if v, ok := scegli(func(v voceDisinst) bool { return strings.HasPrefix(normaNome(v.nome), n) }); ok {
			return v, true
		}
	}
	// 5. L'ultima parte dell'ID ("Git.Git" -> "git", "BleachBit.BleachBit" -> "bleachbit").
	if i := strings.LastIndex(id, "."); i >= 0 && !strings.Contains(id, `\`) {
		if u := normaNome(id[i+1:]); len(u) >= 3 {
			if v, ok := scegli(func(v voceDisinst) bool { return nomeSenzaVersione(v.nome) == u }); ok {
				return v, true
			}
		}
	}
	return voceDisinst{}, false
}

type pacchettoAppx struct {
	nomeCompleto, nome, cartella string
	versione                     []int
	nomeVisibile                 string // DisplayName del registro, anche "@{...ms-resource:...}"
}

// dividiNomeCompleto: "Nome_Versione_Arch_Risorsa_Editore" -> nome, versione, se è di sole risorse.
func dividiNomeCompleto(full string) (string, []int, bool, bool) {
	p := strings.Split(full, "_")
	if len(p) != 5 || p[0] == "" {
		return "", nil, false, false
	}
	var ver []int
	for _, s := range strings.Split(p[1], ".") {
		n, err := strconv.Atoi(s)
		if err != nil {
			return "", nil, false, false
		}
		ver = append(ver, n)
	}
	return p[0], ver, p[3] != "", true
}

// pacchettiPerID: i pacchetti con il nome giusto ("Microsoft.BingWeather" o "*CandyCrush*"),
// dal più nuovo al più vecchio.
func pacchettiPerID(id string, tutti []pacchettoAppx) []pacchettoAppx {
	pat := strings.ToLower(id)
	var out []pacchettoAppx
	for _, pk := range tutti {
		if ok, err := path.Match(pat, strings.ToLower(pk.nome)); err == nil && ok {
			out = append(out, pk)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return versionePiuNuova(out[i].versione, out[j].versione) })
	return out
}

// pacchettoDaIdMSIX: `MSIX\MSTeams_24295.605.3225.8804_x64__8wekyb3d8bbwe` (anche con `\\` o "/",
// o con il solo family name "MSTeams_8wekyb3d8bbwe") -> nome del pacchetto ed editore (PublisherId).
func pacchettoDaIdMSIX(id string) (nome, editore string, ok bool) {
	s := strings.TrimSpace(id)
	if len(s) < 6 || !strings.EqualFold(s[:4], "MSIX") || (s[4] != '\\' && s[4] != '/') {
		return "", "", false
	}
	s = strings.TrimLeft(s[4:], `\/`)
	if i := strings.IndexAny(s, `\/`); i >= 0 {
		s = s[:i]
	}
	p := strings.Split(s, "_")
	switch {
	case len(p) == 5 && p[0] != "":
		return p[0], strings.ToLower(p[4]), true
	case len(p) == 2 && p[0] != "" && p[1] != "":
		return p[0], strings.ToLower(p[1]), true
	case len(p) == 1 && p[0] != "":
		return p[0], "", true
	}
	return "", "", false
}

// editoreAppx: l'ultima parte del nome completo (PublisherId, es. "8wekyb3d8bbwe").
func editoreAppx(full string) string {
	if i := strings.LastIndex(full, "_"); i >= 0 {
		return strings.ToLower(full[i+1:])
	}
	return ""
}

// appxNotiWinget: Id di winget (o nomi normalizzati) di app MSIX che non somigliano al nome del pacchetto.
var appxNotiWinget = map[string]string{
	"microsoft.teams":             "MSTeams",
	"microsoft.teams.free":        "MicrosoftTeams",
	"microsoft.appinstaller":      "Microsoft.DesktopAppInstaller",
	"microsoft.outlookforwindows": "Microsoft.OutlookForWindows",
	"9nrx63209r7b":                "Microsoft.OutlookForWindows",
	"9nblggh4nns1":                "Microsoft.DesktopAppInstaller",
	"microsoftteams":              "MSTeams",
	"microsoftteamspersonal":      "MicrosoftTeams",
	"microsoftteamsfree":          "MicrosoftTeams",
	"outlookforwindows":           "Microsoft.OutlookForWindows",
	"appinstaller":                "Microsoft.DesktopAppInstaller",
	"programmadiinstallazioneapp": "Microsoft.DesktopAppInstaller",
}

// pacchettiPerWinget: i pacchetti Appx di una riga di winget, dal più sicuro al meno sicuro:
// Id MSIX\<FullName>, Id o nome noti, nome del pacchetto uguale al nome, nome visibile uguale (o che
// comincia così, se winget l'ha accorciato con "…"). visibile può essere nil.
func pacchettiPerWinget(nome, id string, tutti []pacchettoAppx, visibile func(pacchettoAppx) string) []pacchettoAppx {
	filtra := func(ok func(pk pacchettoAppx) bool) []pacchettoAppx {
		var out []pacchettoAppx
		for _, pk := range tutti {
			if ok(pk) {
				out = append(out, pk)
			}
		}
		sort.SliceStable(out, func(i, j int) bool { return versionePiuNuova(out[i].versione, out[j].versione) })
		return out
	}
	if n, ed, ok := pacchettoDaIdMSIX(id); ok {
		if out := filtra(func(pk pacchettoAppx) bool {
			return strings.EqualFold(pk.nome, n) && (ed == "" || editoreAppx(pk.nomeCompleto) == ed)
		}); len(out) > 0 {
			return out
		}
	}
	tronco := strings.HasSuffix(strings.TrimSpace(nome), "…")
	n := normaNome(strings.TrimRight(strings.TrimSpace(nome), "…"))
	for _, k := range []string{strings.ToLower(strings.TrimSpace(id)), n} {
		if pn, ok := appxNotiWinget[k]; ok && k != "" && pn != "" && !(k == n && tronco) {
			if out := filtra(func(pk pacchettoAppx) bool { return strings.EqualFold(pk.nome, pn) }); len(out) > 0 {
				return out
			}
		}
	}
	if len(n) < 4 {
		return nil
	}
	// Nome del pacchetto: "Microsoft.OutlookForWindows" -> "outlookforwindows" (o tutto il nome).
	if !tronco {
		if out := filtra(func(pk pacchettoAppx) bool {
			ult := pk.nome[strings.LastIndex(pk.nome, ".")+1:]
			return normaNome(pk.nome) == n || normaNome(ult) == n
		}); len(out) > 0 {
			return out
		}
	}
	if visibile == nil {
		return nil
	}
	return filtra(func(pk pacchettoAppx) bool {
		v := normaNome(visibile(pk))
		if v == "" {
			return false
		}
		if tronco {
			return strings.HasPrefix(v, n)
		}
		return v == n
	})
}

func versionePiuNuova(a, b []int) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return len(a) > len(b)
}

var logoAttrRe = regexp.MustCompile(`(?s)\b(Square44x44Logo|Square150x150Logo)\s*=\s*"([^"]+)"|<Logo>\s*([^<]+?)\s*</Logo>`)

// logoDaManifest: i loghi del manifest, prima il piccolo (44x44), poi il medio, poi quello dello Store.
// Solo percorsi relativi dentro il pacchetto.
func logoDaManifest(xml string) []string {
	var s44, s150, store []string
	for _, m := range logoAttrRe.FindAllStringSubmatch(xml, 20) {
		v := m[2]
		if m[3] != "" {
			v = m[3]
		}
		v = strings.ReplaceAll(strings.TrimSpace(v), "/", `\`)
		if v == "" || strings.HasPrefix(v, `\`) || strings.Contains(v, ":") || strings.Contains(v, "%") {
			continue
		}
		bad := false
		for _, s := range strings.Split(v, `\`) {
			if s == ".." {
				bad = true
			}
		}
		if bad || !strings.EqualFold(filepath.Ext(strings.ReplaceAll(v, `\`, "/")), ".png") {
			continue
		}
		switch {
		case m[1] == "Square44x44Logo":
			s44 = append(s44, v)
		case m[1] == "Square150x150Logo":
			s150 = append(s150, v)
		default:
			store = append(store, v)
		}
	}
	var out []string
	seen := map[string]bool{}
	for _, v := range append(append(s44, s150...), store...) {
		if !seen[strings.ToLower(v)] {
			seen[strings.ToLower(v)] = true
			out = append(out, v)
		}
	}
	return out
}

// scegliLogo: tra i file della cartella ("Logo.targetsize-32_altform-unplated.png", "Logo.scale-100.png"…)
// quelli del logo giusto, dal migliore per una riga da 28 pixel. Niente varianti ad alto contrasto se
// c'è altro.
func scegliLogo(nomi []string, base, ext string, grande bool) []string {
	lb, le := strings.ToLower(base), strings.ToLower(ext)
	pref32 := []string{"targetsize-32_altform-unplated", "targetsize-32", "targetsize-48_altform-unplated", "targetsize-48", "targetsize-36_altform-unplated", "targetsize-40_altform-unplated", "scale-100", "targetsize-24_altform-unplated", "scale-125", "scale-150", "scale-200", "targetsize-64_altform-unplated", "targetsize-64"}
	pref64 := []string{"targetsize-64_altform-unplated", "targetsize-64", "targetsize-48_altform-unplated", "targetsize-48", "scale-200", "scale-150", "targetsize-96_altform-unplated", "targetsize-32_altform-unplated", "targetsize-32", "scale-125", "scale-100"}
	pref := pref32
	if grande {
		pref = pref64
	}
	type cand struct {
		nome string
		voto int
	}
	var cs []cand
	for _, n := range nomi {
		l := strings.ToLower(n)
		if !strings.HasSuffix(l, le) {
			continue
		}
		mid := strings.TrimSuffix(l, le)
		if mid != lb && !strings.HasPrefix(mid, lb+".") {
			continue
		}
		q := strings.TrimPrefix(strings.TrimPrefix(mid, lb), ".")
		voto := 500
		if q == "" {
			voto = 400 // il file senza qualificatori, se c'è
		}
		for i, p := range pref {
			if q == p {
				voto = i
				break
			}
		}
		if voto >= 400 && strings.Contains(q, "targetsize-") && !strings.Contains(q, "contrast") {
			voto = 300
		}
		if strings.Contains(q, "contrast-") {
			voto = 900
		}
		if strings.Contains(q, "lightunplated") {
			voto += 50
		}
		cs = append(cs, cand{n, voto})
	}
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].voto < cs[j].voto })
	var out []string
	for i, c := range cs {
		if i >= 4 {
			break
		}
		out = append(out, c.nome)
	}
	return out
}

// ---------------- fine logica pura ----------------
