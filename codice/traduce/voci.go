package traduce

// Voci dei personaggi → sottotitoli italiani (idea presa da LagoFast e Live Translate).
// Parte che non dipende da Windows: divide l'audio in battute, prepara il WAV per whisper,
// scarta le "allucinazioni" tipiche del riconoscimento vocale su musica e rumore.

import (
	"bytes"
	"encoding/binary"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

const frequenzaVoce = 16000 // whisper vuole 16 kHz mono

// Tagliatore: riceve audio mono a 16 kHz a pezzetti e restituisce le battute intere, tagliate
// nelle pause. Soglia adattiva: segue il rumore di fondo (musica bassa, vento) e considera
// "voce" quello che sta chiaramente sopra.
type Tagliatore struct {
	fondo      float64   // livello del rumore di fondo
	inCorso    []float32 // battuta che si sta registrando
	silenzio   int       // campioni di silenzio consecutivi dentro la battuta
	prima      []float32 // ultimi 300 ms prima dell'inizio (per non tagliare la prima sillaba)
	MinVoce    int       // campioni minimi di una battuta
	MaxVoce    int       // oltre questo la battuta viene chiusa comunque
	PausaFine  int       // silenzio che chiude la battuta
	SogliaMin  float64
	Moltiplica float64
	id         int64         // numero della battuta in corso
	anticipata bool          // ne ho già mandato una copia appena è iniziata la pausa
	forti      int           // pezzi forti di fila dentro una pausa (un colpo di musica non è voce)
	livelli    []float64     // livelli degli ultimi 3 s (per il rumore di fondo)
	ultima     []float32     // ultima battuta chiusa (per riattaccarla se la frase continua)
	dopo       int           // campioni passati dalla sua chiusura
	base       int           // lunghezza del pezzo riattaccato all'inizio della battuta in corso
	info       []InfoBattuta // per ogni battuta chiusa nell'ultima Aggiungi
}

// InfoBattuta: per ogni battuta chiusa, se era già stata mandata in anticipo e da allora il
// personaggio non ha più parlato (quindi il sottotitolo anticipato vale già come finale).
type InfoBattuta struct {
	ID         int64
	Anticipata bool
}

// Anticipo: appena dentro una battuta c'è una pausa di almeno minSil campioni, restituisce
// (una volta sola) la battuta fin lì: si comincia a capirla subito invece di aspettare che la
// pausa diventi abbastanza lunga da chiuderla. Se il personaggio riprende a parlare, l'anticipo
// non vale più e la battuta intera verrà capita alla chiusura.
func (t *Tagliatore) Anticipo(minSil int) ([]float32, int64, bool) {
	if t.inCorso == nil || t.anticipata || t.silenzio < minSil || len(t.inCorso)-t.silenzio-t.base < t.MinVoce {
		return nil, 0, false
	}
	t.anticipata = true
	return append([]float32(nil), t.inCorso[:len(t.inCorso)-t.silenzio]...), t.id, true
}

// Info: le informazioni delle battute chiuse nell'ultima Aggiungi (stesso ordine).
func (t *Tagliatore) Info() []InfoBattuta { return t.info }

func NuovoTagliatore() *Tagliatore {
	return &Tagliatore{
		fondo:      0.003,
		MinVoce:    frequenzaVoce * 6 / 10,   // 0,6 s
		MaxVoce:    frequenzaVoce * 7,        // 7 s (whisper lavora con un contesto di 7,7 s)
		PausaFine:  frequenzaVoce * 45 / 100, // 0,45 s: con 0,35 le frasi si spezzavano alle virgole e si capivano peggio (collaudo 22/11)
		SogliaMin:  0.008,
		Moltiplica: 2.5,
	}
}

func percentile(x []float64, p float64) float64 {
	c := append([]float64(nil), x...)
	sort.Float64s(c)
	return c[int(float64(len(c)-1)*p)]
}

func rms(x []float32) float64 {
	if len(x) == 0 {
		return 0
	}
	var s float64
	for _, v := range x {
		s += float64(v) * float64(v)
	}
	return math.Sqrt(s / float64(len(x)))
}

// InCorso: quanto è lunga la battuta che si sta registrando (0 se nessuna).
func (t *Tagliatore) InCorso() time.Duration {
	return time.Duration(len(t.inCorso)) * time.Second / frequenzaVoce
}

// CopiaInCorso: copia della battuta in corso (per il sottotitolo provvisorio).
func (t *Tagliatore) CopiaInCorso() []float32 {
	return append([]float32(nil), t.inCorso...)
}

// Aggiungi: pezzi da 20 ms. Restituisce le battute chiuse.
func (t *Tagliatore) Aggiungi(campioni []float32) [][]float32 {
	var fuori [][]float32
	t.info = t.info[:0]
	const passo = frequenzaVoce / 50 // 20 ms
	for len(campioni) > 0 {
		n := min(passo, len(campioni))
		pezzo := campioni[:n]
		campioni = campioni[n:]
		livello := rms(pezzo)
		// rumore di fondo = il livello "basso" degli ultimi 3 secondi (20° percentile), anche
		// mentre qualcuno parla: tra una parola e l'altra si sente solo la musica del gioco.
		// Così con musica continua la soglia sale da sola in pochi secondi.
		t.livelli = append(t.livelli, livello)
		if len(t.livelli) > 150 {
			t.livelli = t.livelli[len(t.livelli)-150:]
		}
		if len(t.livelli) >= 25 {
			t.fondo = math.Max(0.0015, percentile(t.livelli, 0.2))
		}
		soglia := math.Max(t.SogliaMin, t.fondo*t.Moltiplica)
		voce := livello > soglia
		if t.inCorso == nil {
			t.dopo += n
			if voce {
				t.inCorso = append(append([]float32{}, t.prima...), pezzo...)
				// la frase riprende entro 0,8 s (era solo una virgola): riattacco il pezzo di prima,
				// così whisper capisce la frase intera e il sottotitolo la mostra tutta
				t.base = 0
				if t.ultima != nil && t.dopo <= frequenzaVoce*8/10 && len(t.ultima)+len(t.inCorso) < t.MaxVoce {
					t.base = len(t.ultima)
					t.inCorso = append(append([]float32{}, t.ultima...), t.inCorso...)
				}
				t.ultima = nil
				t.silenzio = 0
				t.id++
				t.anticipata = false
			} else {
				t.prima = append(t.prima, pezzo...)
				if max300 := frequenzaVoce * 3 / 10; len(t.prima) > max300 {
					t.prima = t.prima[len(t.prima)-max300:]
				}
			}
			continue
		}
		t.inCorso = append(t.inCorso, pezzo...)
		if voce {
			t.forti++
		} else {
			t.forti = 0
		}
		// dentro una pausa, un solo pezzo forte (colpo di batteria, passo) non conta come voce:
		// la voce vera dura almeno 40 ms di fila
		if voce && (t.silenzio == 0 || t.forti >= 2) {
			t.silenzio = 0
			t.anticipata = false // ha ripreso a parlare: l'anticipo non vale più
		} else {
			t.silenzio += n
		}
		if t.silenzio >= t.PausaFine || len(t.inCorso) >= t.MaxVoce {
			// conta solo la parte nuova: un colpo di rumore dopo una frase non la fa ripetere
			if len(t.inCorso)-t.silenzio-t.base >= t.MinVoce {
				fuori = append(fuori, t.inCorso)
				t.info = append(t.info, InfoBattuta{t.id, t.anticipata})
				t.ultima, t.dopo = t.inCorso, 0
			}
			t.inCorso, t.silenzio, t.prima = nil, 0, nil
		}
	}
	return fuori
}

// wav16: WAV PCM 16 bit mono a 16 kHz, quello che whisper-server legge senza ffmpeg.
func wav16(x []float32) []byte {
	var b bytes.Buffer
	dati := len(x) * 2
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(36+dati))
	b.WriteString("WAVEfmt ")
	binary.Write(&b, binary.LittleEndian, uint32(16))
	binary.Write(&b, binary.LittleEndian, uint16(1)) // PCM
	binary.Write(&b, binary.LittleEndian, uint16(1)) // mono
	binary.Write(&b, binary.LittleEndian, uint32(frequenzaVoce))
	binary.Write(&b, binary.LittleEndian, uint32(frequenzaVoce*2))
	binary.Write(&b, binary.LittleEndian, uint16(2))
	binary.Write(&b, binary.LittleEndian, uint16(16))
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, uint32(dati))
	for _, v := range x {
		v = max(-1, min(1, v))
		binary.Write(&b, binary.LittleEndian, int16(v*32767))
	}
	return b.Bytes()
}

// riduci: da frequenza f (es. 48000) a 16 kHz, mono già fatto. Media sui campioni di ogni
// intervallo (filtro semplice contro l'aliasing).
func riduci(x []float32, f int) []float32 {
	if f == frequenzaVoce || f <= 0 {
		return x
	}
	passo := float64(f) / frequenzaVoce
	n := int(float64(len(x)) / passo)
	out := make([]float32, n)
	for i := range out {
		a := int(float64(i) * passo)
		b := min(len(x), int(float64(i+1)*passo))
		var s float32
		for _, v := range x[a:b] {
			s += v
		}
		if b > a {
			out[i] = s / float32(b-a)
		}
	}
	return out
}

var reParentesi = regexp.MustCompile(`[\[\(][^\]\)]{0,40}[\]\)]`)

// frasi che whisper "sente" nella musica o nel silenzio anche quando nessuno parla
var reNonVoce = regexp.MustCompile(`(?i)^[\s\[\(\*♪.♫-]*(music|musica|applause|laughter|silence|blank_audio|inaudible|sound|noise|wind|footsteps|gunshots?|explosions?|thank you\.?|thanks for watching!?|you|bye\.?|subtitles by.*|\.\.\.)[\s\]\)\*♪.♫-]*$`)

// testoVoce: pulisce il risultato di whisper; "" se non è una battuta vera.
func testoVoce(s string) string {
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	// tolgo le indicazioni tra parentesi quadre o tonde ([music], (laughs))
	s = reParentesi.ReplaceAllString(s, "")
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	if s == "" || reNonVoce.MatchString(s) {
		return ""
	}
	lettere := 0
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			lettere++
		}
	}
	if lettere < 3 {
		return ""
	}
	// whisper a volte scrive la battuta tutta minuscola: maiuscola all'inizio
	if r := []rune(s); r[0] >= 'a' && r[0] <= 'z' {
		r[0] -= 'a' - 'A'
		s = string(r)
	}
	return s
}

// durataSottotitolo: quanto tenere a schermo una battuta (tempo di lettura, 2,5-8 s).
func durataSottotitolo(testo string) float64 {
	return math.Max(2.5, math.Min(8, 1.2+float64(len(strings.Fields(testo)))*0.32))
}
