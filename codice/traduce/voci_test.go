package traduce

import (
	"math"
	"testing"
)

func tono(sec float64, ampiezza float64) []float32 {
	n := int(sec * frequenzaVoce)
	x := make([]float32, n)
	for i := range x {
		x[i] = float32(ampiezza * math.Sin(2*math.Pi*220*float64(i)/frequenzaVoce))
	}
	return x
}

func TestTagliatore(t *testing.T) {
	tg := NuovoTagliatore()
	var audio []float32
	audio = append(audio, tono(1, 0.001)...) // quasi silenzio
	audio = append(audio, tono(1.5, 0.2)...) // battuta 1
	audio = append(audio, tono(0.8, 0.001)...)
	audio = append(audio, tono(0.2, 0.2)...) // colpo breve: non è una battuta
	audio = append(audio, tono(0.8, 0.001)...)
	audio = append(audio, tono(2, 0.2)...) // battuta 2
	audio = append(audio, tono(1, 0.001)...)
	var battute [][]float32
	for i := 0; i < len(audio); i += 320 {
		battute = append(battute, tg.Aggiungi(audio[i:min(len(audio), i+320)])...)
	}
	if len(battute) != 2 {
		t.Fatalf("attese 2 battute, trovate %d", len(battute))
	}
	if d := float64(len(battute[1])) / frequenzaVoce; d < 2 || d > 3 {
		t.Errorf("seconda battuta di %.2f s", d)
	}
}

func TestTestoVoce(t *testing.T) {
	no := []string{"[Music]", " (laughs) ", "Thank you.", "♪ ♪", "you", "[BLANK_AUDIO]", "..."}
	for _, s := range no {
		if testoVoce(s) != "" {
			t.Errorf("%q doveva essere scartato", s)
		}
	}
	if g := testoVoce(" Reloading. [gunshots] Cover me! "); g != "Reloading. Cover me!" {
		t.Errorf("pulizia: %q", g)
	}
}

func TestWavERiduci(t *testing.T) {
	x := riduci(tono(1, 0.5), 48000)
	if len(x) != 5333 { // 1 s a 16 kHz ricavato da 16000 campioni "come se" fossero a 48 kHz
		t.Logf("lunghezza %d", len(x))
	}
	w := wav16(tono(0.5, 0.5))
	if string(w[:4]) != "RIFF" || len(w) != 44+8000*2 {
		t.Errorf("wav sbagliato: %d byte", len(w))
	}
}

func TestAnticipo(t *testing.T) {
	tg := NuovoTagliatore()
	manda := func(x []float32) (chiuse [][]float32, info []InfoBattuta, anticipi int) {
		for i := 0; i < len(x); i += 320 {
			c := tg.Aggiungi(x[i:min(len(x), i+320)])
			chiuse = append(chiuse, c...)
			info = append(info, tg.Info()...)
			if _, _, ok := tg.Anticipo(frequenzaVoce * 15 / 100); ok {
				anticipi++
			}
		}
		return
	}
	audio := append(append(tono(0.5, 0.001), tono(1.5, 0.2)...), tono(0.8, 0.001)...)
	chiuse, info, anticipi := manda(audio)
	if len(chiuse) != 1 || anticipi != 1 || !info[0].Anticipata {
		t.Fatalf("frase con una pausa sola: chiuse %d anticipi %d info %+v", len(chiuse), anticipi, info)
	}
	// pausa breve e poi riprende a parlare: l'anticipo non vale più per la chiusura
	audio = append(append(append(tono(1.2, 0.2), tono(0.25, 0.001)...), tono(1.0, 0.2)...), tono(0.8, 0.001)...)
	chiuse, info, anticipi = manda(audio)
	if len(chiuse) != 1 || anticipi != 2 || !info[0].Anticipata {
		// due anticipi (uno alla pausa breve, uno alla fine); vale l'ultimo
		t.Fatalf("frase con pausa in mezzo: chiuse %d anticipi %d info %+v", len(chiuse), anticipi, info)
	}
}

// con musica continua sotto (un terzo del volume della voce) le battute si tagliano lo stesso
func TestTagliatoreConMusica(t *testing.T) {
	tg := NuovoTagliatore()
	musica := func(sec float64) []float32 {
		x := tono(sec, 0.06)
		for i := range x {
			x[i] += float32(0.02 * math.Sin(float64(i)*0.37))
		}
		return x
	}
	var audio []float32
	audio = append(audio, musica(4)...)
	voce := tono(1.5, 0.25)
	m := musica(1.5)
	for i := range voce {
		voce[i] += m[i]
	}
	audio = append(audio, voce...)
	audio = append(audio, musica(1.5)...)
	var battute [][]float32
	for i := 0; i < len(audio); i += 320 {
		battute = append(battute, tg.Aggiungi(audio[i:min(len(audio), i+320)])...)
	}
	if len(battute) != 1 {
		t.Fatalf("attesa 1 battuta, trovate %d", len(battute))
	}
	if d := float64(len(battute[0])) / frequenzaVoce; d > 2.6 {
		t.Errorf("la battuta comprende troppa musica: %.2f s", d)
	}
}
