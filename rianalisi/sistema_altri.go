// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

//go:build !windows

package rianalisi

// Fuori da Windows un file "irregolare" (socket, dispositivo) non va mai nell'archivio.
func collegamentoSistema(string) bool { return true }
