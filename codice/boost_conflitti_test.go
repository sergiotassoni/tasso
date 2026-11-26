package main

import (
	"testing"
	"time"
)

// Il Boost non va contro le scelte fatte in Windows o nel pannello della scheda video.

// "Ottimizzazioni per i giochi in finestra" spenta apposta (=0): il Boost non la riaccende.
func TestWinoptSceltoSpento(t *testing.T) {
	casi := []struct {
		m     map[string]string
		vuole bool
	}{
		{map[string]string{}, false},
		{map[string]string{"SwapEffectUpgradeEnable": "1"}, false},
		{map[string]string{"SwapEffectUpgradeEnable": "0"}, true},
		{map[string]string{"AutoHDREnable": "1", "SwapEffectUpgradeEnable": "0"}, true},
		{map[string]string{"VRROptimizeEnable": "0"}, false},
	}
	for _, c := range casi {
		if got := winoptSceltoSpento(c.m); got != c.vuole {
			t.Errorf("%v: %v, volevo %v", c.m, got, c.vuole)
		}
	}
}

// Una scheda video scelta in Windows (anche a risparmio energetico) è una scelta: niente "Sistemalo".
func TestGpuPrefScelta(t *testing.T) {
	casi := map[string]bool{
		"":                                 false,
		"GpuPreference=0;":                 false, // Lascia decidere a Windows
		"GpuPreference=1;":                 true,  // risparmio energetico, scelto da te
		"GpuPreference=2;":                 true,
		"SwapEffectUpgradeEnable=1;":       false,
		"AutoHDREnable=1;GpuPreference=1;": true,
		" GpuPreference = 2 ;":             true,
		"GpuPreference=;":                  false,
	}
	for v, vuole := range casi {
		if got := gpuPrefScelta(v); got != vuole {
			t.Errorf("%q: %v, volevo %v", v, got, vuole)
		}
	}
}

// La scheda potente si aggiunge alle altre scelte di Windows per quel gioco, senza cancellarle.
func TestGpuPrefConAltre(t *testing.T) {
	casi := []struct {
		old   string
		nuove []string
		vuole string
	}{
		{"", []string{"GpuPreference=2"}, "GpuPreference=2;"},
		{"GpuPreference=0;", []string{"GpuPreference=2"}, "GpuPreference=2;"},
		{"AutoHDREnable=1;GpuPreference=1;", []string{"GpuPreference=2"}, "GpuPreference=2;AutoHDREnable=1;"},
		{"VRROptimizeEnable=0;SwapEffectUpgradeEnable=0;", []string{"GpuPreference=2", "SwapEffectUpgradeEnable=1"}, "GpuPreference=2;SwapEffectUpgradeEnable=1;VRROptimizeEnable=0;"},
		{";;rotto;AutoHDREnable=1", []string{"GpuPreference=2"}, "GpuPreference=2;AutoHDREnable=1;"},
	}
	for _, c := range casi {
		if got := gpuPrefConAltre(c.old, c.nuove...); got != c.vuole {
			t.Errorf("%q: %q, volevo %q", c.old, got, c.vuole)
		}
	}
}

// Il controllo FPS non propone (e non fa) la scheda potente se in Windows ce n'è già una scelta.
func TestFpsFindingsGpuSoloSenzaScelta(t *testing.T) {
	f := pcFacts{Laptop: true}
	f.GPUs = append(f.GPUs, struct {
		Name string `json:"name"`
		Date string `json:"date"`
	}{Name: "NVIDIA GeForce RTX 4060 Laptop GPU"}, struct {
		Name string `json:"name"`
		Date string `json:"date"`
	}{Name: "Intel(R) UHD Graphics"})
	last := &fpsResult{Game: "Gioco", Avg: 80, path: `C:\Giochi\gioco.exe`}
	ha := func(scelta bool) bool {
		for _, x := range fpsFindings(f, nil, last, false, scelta, time.Now()) {
			if x.ID == "gpu" {
				return true
			}
		}
		return false
	}
	if !ha(false) {
		t.Error("senza scelta in Windows il consiglio della scheda potente deve esserci")
	}
	if ha(true) {
		t.Error("con una scheda scelta in Windows (anche a risparmio) niente consiglio")
	}
}

// La modalità messa per il gioco si rimette solo se è ancora quella (non l'hai cambiata tu).
func TestModoDaRimettereDopoGioco(t *testing.T) {
	casi := []struct {
		cur, target string
		vuole       bool
	}{
		{"gaming", "gaming", true},
		{"fps", "fps", true},
		{"gaming", "", true},
		{"normale", "gaming", false},
		{"film", "fps", false},
		{"fps", "", false},
	}
	for _, c := range casi {
		if got := modoDaRimettereDopoGioco(c.cur, c.target); got != c.vuole {
			t.Errorf("%s/%s: %v, volevo %v", c.cur, c.target, got, c.vuole)
		}
	}
}
