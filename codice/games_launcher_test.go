package main

import "testing"

func TestGameNameFromLauncher(t *testing.T) {
	for in, want := range map[string][2]string{
		"Hytale Launcher":    {"Hytale", "Hytale Launcher"},
		"Launcher di Hytale": {"Hytale", "Launcher di Hytale"},
		"Hytale - Launcher":  {"Hytale", "Hytale - Launcher"},
		"Launcher":           {"Launcher", ""},
		"Subnautica 2":       {"Subnautica 2", ""},
	} {
		n, l := gameNameFromLauncher(in)
		if n != want[0] || l != want[1] {
			t.Errorf("%q -> %q, %q; atteso %q", in, n, l, want)
		}
	}
}
