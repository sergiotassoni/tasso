package traduce

// Settimo giro: gli stessi 9 video, 5 minuti più avanti (r3.txt, build bd5a49e, 1337 fotogrammi).
// Casi veri (testo letto dall'OCR e posizione nel fotogramma a 1920x1080):
//   - 0083 "…While I'm" / "here." → "Qui." da solo; 0771 "area." da solo; 1189 "elect"; 0657
//     "press to sprint." → "premere per sprint."; "Press" da solo;
//   - 1147 "/Back" → "/Indietto", "/SUITS /SETTINGS", 1044 "'Close", 1145 "[Shipwreck";
//   - traduttore veloce: "Inestare il negozio all'angolo.", "PUNCI DI SVOGLIAMENTO", "Propriettario
//     di negozi", "zatteri: (urlente)" (e "Raffs" corretto in "Rafts" dalla correzione dell'OCR);
//   - 0441 "Gamer's Litüe Playgrmjnd" (logo del canale) tradotto;
//   - 0363 "baking the cake" letto con una parolaccia e tradotto volgare.

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// rigaLettere: riga con le parole strette sulle lettere come le dà Windows: alte 0,74 em con le
// lettere alte, 0,53 em senza (+0,21 em con le gambe sotto), larghe circa 0,48 em a lettera.
func rigaLettere(testo string, x, y, em float64) RigaOCR {
	var r RigaOCR
	for _, p := range strings.Fields(testo) {
		sopra, sotto := false, false
		for _, c := range p {
			if strings.ContainsRune("bdfhklt'/[0123456789", c) || (c >= 'A' && c <= 'Z') {
				sopra = true
			}
			if strings.ContainsRune("gjpqy", c) {
				sotto = true
			}
		}
		yy, h := y+0.21*em, 0.53*em
		if sopra {
			yy, h = y, 0.74*em
		}
		if sotto {
			h += 0.21 * em
		}
		w := float64(len([]rune(p))) * 0.48 * em
		r.Parole = append(r.Parole, ParolaOCR{T: p, X: x, Y: yy, W: w, H: h})
		x += w + 0.25*em
	}
	return r
}

func disegnate(righe ...RigaOCR) ([]Blocco, []Blocco) {
	bl := blocchiLetti(nil, righe, 1, nil, time.Now())
	pronti, _ := decisione(nil, bl)
	return bl, pronti
}

// 0083: sottotitolo centrato, la seconda riga è solo "here." (lettere basse: il riquadro di
// Windows è molto più basso della riga sopra). Si unisce alla prima; e se non si unisse, le due
// metà non si disegnano mai ognuna con la sua traduzione.
func TestPezzoSottoNonDaSolo(t *testing.T) {
	sopra := rigaLettere("Thinkin' 'bout it, I could probably get it tuned up for ya While I'm", 631, 928, 22)
	sotto := rigaLettere("here.", 934, 964, 22)
	bl, pronti := disegnate(sopra, sotto)
	if len(bl) != 1 || !strings.HasSuffix(bl[0].Testo, "While I'm here.") {
		t.Errorf("sottotitolo non unito: %q", chiaviBlocchi(bl))
	}
	for _, p := range pronti {
		if strings.TrimSpace(p.Testo) == "here." {
			t.Errorf("\"here.\" disegnata da sola")
		}
	}
	// le due metà già divise e tutte e due pronte: nessuna si disegna
	a := bloccoDaParole(sopra.Parole, 1)
	a.Testo = "Thinkin' 'bout it, I could probably get it tuned up for ya While I'm"
	b := bloccoDaParole(sotto.Parole, 1)
	b.Testo = "here."
	if tenute, _ := senzaMezzeRighe([]Blocco{a, b}, nil); len(tenute) != 0 {
		t.Errorf("metà di frase tradotte a parte: %q", testiBlocchi(tenute))
	}
	// 0771: "area." sotto "…collected in each" (rimasta in inglese) non si disegna da sola
	c := bloccoDaParole(rigaLettere("chart to see how many you've collected in each", 1300, 631, 24).Parole, 1)
	d := bloccoDaParole(rigaLettere("area.", 1300, 658, 24).Parole, 1)
	if tenute, _ := senzaMezzeRighe([]Blocco{d}, []Blocco{c}); len(tenute) != 0 {
		t.Errorf("\"area.\" disegnata da sola sotto la sua frase in inglese")
	}
	// e il paragrafo intero si unisce ("…to track" / "Secrets. Look for…" / … / "area.")
	bl, _ = disegnate(rigaLettere("You can also use Turning Points to track", 1300, 577, 24),
		rigaLettere("Secrets. Look for the Secret symbols on the", 1300, 604, 24),
		rigaLettere("chart to see how many you've collected in each", 1300, 631, 24), rigaLettere("area.", 1300, 658, 24))
	if len(bl) != 1 || !strings.HasSuffix(bl[0].Testo, "in each area.") {
		t.Errorf("paragrafo non unito: %q", chiaviBlocchi(bl))
	}
}

// 1189 "elect", 0657 "press to sprint.": pezzo minuscolo senza la sua testa; "Press" da solo.
func TestPezzoMinuscoloSenzaTesta(t *testing.T) {
	for _, r := range []RigaOCR{rigaLettere("elect", 1738, 1025, 22), rigaLettere("press to sprint.", 853, 785, 26), rigaLettere("Press", 792, 823, 26)} {
		if _, pronti := disegnate(r); len(pronti) != 0 {
			t.Errorf("%q disegnata da sola", testiBlocchi(pronti))
		}
	}
	// "Press [icona] to fire your weapon.": i due pezzi insieme vanno bene (l'icona è in mezzo)
	_, pronti := disegnate(rigaLettere("Press", 792, 823, 26), rigaLettere("to fire your weapon.", 912, 822, 26))
	if len(pronti) == 0 {
		t.Errorf("comando con l'icona in mezzo: %q", testiBlocchi(pronti))
	}
	// 0653: "Q to aim." dopo "Tap Q to quickly throw the item. Hold" rimasta in inglese: mai da sola
	a := bloccoDaParole(rigaLettere("Tap Q to quickly throw the item. Hold", 670, 823, 26).Parole, 1)
	q := bloccoDaParole(rigaLettere("Q to aim.", float64(a.X+a.W+8), 823, 26).Parole, 1)
	if tenute, _ := senzaMezzeRighe([]Blocco{q}, []Blocco{a}); len(tenute) != 0 {
		t.Errorf("\"Q to aim.\" disegnata da sola dopo la prima parte in inglese")
	}
	// le voci di menu esatte scritte minuscole restano tradotte
	m := Blocco{Testo: "options", X: 100, Y: 100, W: 80, H: 20, Righe: 1, Affid: letturaCatalog}
	if pezzoSenzaTesta(m, []Blocco{m}) {
		t.Error("voce di menu minuscola del catalogo tolta")
	}
}

// 1147, 1044, 1145: il segno letto dall'icona non si traduce e la traduzione non copre l'icona.
func TestSegnoDellIcona(t *testing.T) {
	casi := []struct {
		riga  RigaOCR
		testi []string
	}{
		{rigaLettere("/Back", 1784, 1017, 30), []string{"Back"}},
		{rigaLettere("'Close", 1215, 965, 28), []string{"Close"}},
		{rigaLettere("[Shipwreck", 185, 206, 28), []string{"Shipwreck"}},
		{rigaLettere("'SAVE", 137, 367, 28), []string{"SAVE"}},
		{rigaLettere("/MAp", 828, 57, 26), []string{"MAP"}},
		{rigaLettere("/SUITS /SETTINGS", 909, 57, 26), []string{"SUITS", "SETTINGS"}},
	}
	for _, c := range casi {
		bl, _ := disegnate(c.riga)
		if got := testiBlocchi(bl); strings.Join(got, "|") != strings.Join(c.testi, "|") {
			t.Errorf("%q: letto %q, voluto %q", testoParole(c.riga.Parole), got, c.testi)
			continue
		}
		if bl[0].X <= int(c.riga.Parole[0].X) || bl[0].IconaSx != bl[0].X {
			t.Errorf("%q: il riquadro copre ancora il segno dell'icona (x %d, icona %d)", c.testi[0], bl[0].X, bl[0].IconaSx)
		}
	}
	// restano: le contrazioni ("'bout"), le parentesi chiuse, le frasi lunghe
	for _, s := range []string{"'bout time you showed up", "(screaming)"} {
		bl, _ := disegnate(rigaLettere(s, 500, 500, 24))
		if len(bl) != 1 || bl[0].Testo != s {
			t.Errorf("%q cambiata: %q", s, testiBlocchi(bl))
		}
	}
}

// 0441, 0363, 1147: logo letto male e parolaccia dubbia restano in inglese; il nome con i numeri
// in una frase buona no ("The place where S4Vl resides.").
func TestLogoEParolacce(t *testing.T) {
	_, pronti := disegnate(rigaLettere("Gamer's Litüe Playgrmjnd", 77, 38, 22))
	if len(pronti) != 0 {
		t.Errorf("logo letto male tradotto: %q", testiBlocchi(pronti))
	}
	_, pronti = disegnate(rigaLettere("Caz: Yeah, you were so drunk you didn't even remember baking the cunt!", 521, 953, 30))
	if len(pronti) != 0 {
		t.Errorf("parolaccia dell'OCR tradotta: %q", testiBlocchi(pronti))
	}
	for _, s := range []string{"Caz: And happy new year to you too, you cunt.", "Shit! They found us!",
		"Hadir: Keep those fucking dogs in check!", "The place where S4Vl resides. Home sweet home.",
		"Barkov: There will be no victory for our enemies... only death."} {
		if _, pronti := disegnate(rigaLettere(s, 600, 900, 28)); len(pronti) != 1 {
			t.Errorf("%q non tradotta", s)
		}
	}
}

// Il controllo dell'italiano: le traduzioni sbagliate vere del traduttore veloce e quelle buone.
func TestItalianoInventato(t *testing.T) {
	cattive := [][2]string{
		{"Investigate the corner store.", "Inestare il negozio all'angolo."},
		{"TURNING POINTS", "PUNCI DI SVOGLIAMENTO"},
		{"Shop Owner", "Propriettario di negozi"},
		{"Rafts: (screaming)", "zatteri: (urlente)"},
		{"/Back", "/Indietto"},
		{"Autosave in progress. Do not turn off your computer.", "Autosvegliere in corso. Non spegnere il computer."},
		{"Skip the intro video", "Salta il video introdutrò"},
		{"The fans are going wild!", "I fan stanno impazzindo!"},
		{"I need ten iron ingots to finish the gate.", "Mi servono dieci lingitti di ferro per finire il cancello."},
	}
	for _, c := range cattive {
		if si, _ := traduzioneInventata(c[0], c[1]); !si {
			t.Errorf("%q → %q: doveva essere scartata", c[0], c[1])
		}
	}
	buone := [][2]string{
		{"The place where S4Vl resides. Home sweet home.", "Il luogo in cui S4Vl risiede. Casa dolce casa."},
		{"Items can be thrown at enemies or used to distract them.", "Gli oggetti possono essere lanciati contro i nemici o usati per distrarli."},
		{"Barkov: There will be no victory for our enemies... o only death.", "Barkov: Non ci sarà vittoria per i nostri nemici... o solo la morte."},
		{"Koby's bullets recharge over time and deal significant stun damage to enemies.", "I proiettili di Koby si ricaricano nel tempo e infliggono danni significativi ai nemici."},
		{"Yeah, but I thought I saw a shadow down there", "Si', ma pensavo di aver visto un'ombra laggiù"},
		{"I lost my brother in that battle. I won't lose you too.", "Ho perso mio fratello in quella battaglia. Non ti perdero' anche io."},
		{"Press to sprint.", "Premi per scattare."},
		{"Let's look around and see what we can find.", "Guardiamoci intorno e vediamo cosa possiamo trovare."},
		{"Hadir: I set up shop on the edge of Barkov's base.", "Hadiro: Ho allestito un negozio ai margini della base di Barkov."},
		{"Upgrade your suspension to handle the dirt tracks.", "Aggiorna la sospensione per gestire le tracce sterrate."},
		{"You can customize your character at any time.", "Puoi personalizzare il tuo personaggio in qualsiasi momento."},
		{"Russian Soldier: Well, shit, I guess they were terrorists...", "Soldato russo: Beh, merda, credo fossero terroristi..."},
	}
	for _, c := range buone {
		if si, w := traduzioneInventata(c[0], c[1]); si {
			t.Errorf("%q → %q: scartata per %q", c[0], c[1], w)
		}
	}
}

// motoreMappa: traduzioni finte decise dal test.
type motoreMappa map[string]string

func (m motoreMappa) Nome() string { return "Mozilla" }
func (m motoreMappa) Traduci(_ context.Context, r RichiestaTraduzione) (string, error) {
	if v, ok := m[r.Testo]; ok {
		return v, nil
	}
	return "", errSenzaMotore{}
}

// Il traduttore: con le parole inventate riprova con le maiuscole cambiate, se no niente
// traduzione (resta l'inglese) e niente memoria. Il nome di chi parla resta com'è.
func TestTraduttoreParoleInventate(t *testing.T) {
	m := motoreMappa{
		"TURNING POINTS":                "PUNCI DI SVOGLIAMENTO",
		"Turning points":                "Punti di svolta",
		"Investigate the corner store.": "Inestare il negozio all'angolo.",
		"Shop Owner":                    "Propriettario di negozi",
		"Shop owner":                    "Proprietario del negozio",
		"(screaming)":                   "(urla)",
	}
	tr := NuovoTraduttore(filepath.Join(t.TempDir(), "mem.json"), glossarioBase(), m)
	ctx := context.Background()
	if r := tr.TraduciScritta(ctx, "TURNING POINTS"); r.Tradotto != "PUNTI DI SVOLTA" {
		t.Errorf("TURNING POINTS → %+v", r)
	}
	if r := tr.TraduciScritta(ctx, "Shop Owner"); r.Tradotto != "Proprietario del negozio" {
		t.Errorf("Shop Owner → %+v", r)
	}
	r := tr.TraduciScritta(ctx, "Investigate the corner store.")
	if r.Tradotto != "" || !strings.Contains(r.Errore, errParoleInventate) {
		t.Errorf("traduzione inventata mostrata: %+v", r)
	}
	if m := motivoScartaTraduzione(Blocco{Testo: "Investigate the corner store."}, r); m == "" {
		t.Error("la scritta doveva restare in inglese")
	}
	if r := tr.TraduciScritta(ctx, "Rafts: (screaming)"); r.Tradotto != "Rafts: (urla)" {
		t.Errorf("nome di chi parla: %+v", r)
	}
	// una traduzione inventata rimasta in memoria da una versione di prima non si usa più
	tr2 := NuovoTraduttore(filepath.Join(t.TempDir(), "mem.json"), glossarioBase(), m)
	tr2.cache["\x00TURNING POINTS"] = "PUNCI DI SVOGLIAMENTO"
	if r := tr2.TraduciScritta(ctx, "TURNING POINTS"); r.Tradotto != "PUNTI DI SVOLTA" || r.Motore == "memoria" {
		t.Errorf("memoria con parole inventate usata: %+v", r)
	}
	// la correzione dell'OCR non cambia il nome di chi parla ("Raffs" non diventa "Rafts")
	if s, _ := correggiRigaOCR("Raffs: (screaming)"); !strings.HasPrefix(s, "Raffs:") {
		t.Errorf("nome di chi parla corretto: %q", s)
	}
}
