//go:build !windows && !provaui

// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

import "fmt"

// Su sistemi diversi da Windows il programma non ha finestra: qui servono solo i test.
func main() {
	fmt.Println("Tasso PDF funziona solo su Windows.")
}
