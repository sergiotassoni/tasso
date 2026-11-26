package main

import "testing"

// "Rimetti com'era" dei giochi in finestra rimette solo la parte di Tasso: HDR automatico o VRR
// accesi dall'utente nel frattempo restano.
func TestDxRimettiParte(t *testing.T) {
	adesso := map[string]string{"SwapEffectUpgradeEnable": "1", "AutoHDREnable": "1", "VRROptimizeEnable": "1"}
	// prima non c'era niente: tolgo solo SwapEffectUpgradeEnable
	m := dxRimettiParte(adesso, regPrev{Kind: "none"}, "SwapEffectUpgradeEnable")
	if _, ok := m["SwapEffectUpgradeEnable"]; ok || m["AutoHDREnable"] != "1" || m["VRROptimizeEnable"] != "1" {
		t.Fatalf("dopo il ripristino: %v", m)
	}
	// prima c'era "0": torna "0", il resto non si tocca
	adesso = map[string]string{"SwapEffectUpgradeEnable": "1", "AutoHDREnable": "1"}
	m = dxRimettiParte(adesso, regPrev{Kind: "sz", S: "VRROptimizeEnable=0;SwapEffectUpgradeEnable=0;"}, "SwapEffectUpgradeEnable")
	if m["SwapEffectUpgradeEnable"] != "0" || m["AutoHDREnable"] != "1" {
		t.Fatalf("dopo il ripristino: %v", m)
	}
	if _, ok := m["VRROptimizeEnable"]; ok {
		t.Fatal("le altre chiavi della copia vecchia non devono tornare")
	}
}
