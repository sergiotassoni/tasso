package main

import "testing"

// Un gioco con il nome solo in caratteri non latini conta comunque come gioco aperto.
func TestNomeGiocoSicuro(t *testing.T) {
	if nomeGiocoSicuro(`原神.exe`) != "gioco" || nomeGiocoSicuro("Игра") != "gioco" {
		t.Fatal("nome non latino: voglio \"gioco\"")
	}
	if nomeGiocoSicuro("") != "" || nomeGiocoSicuro("  ") != "" {
		t.Fatal("nessun gioco: resta vuoto")
	}
	if nomeGiocoSicuro("Cyberpunk2077.exe") != "Cyberpunk2077.exe" {
		t.Fatal("nome latino: resta com'è")
	}
}
