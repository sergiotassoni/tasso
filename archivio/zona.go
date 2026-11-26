// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package archivio

// Marchio del web (Mark-of-the-Web). Un archivio scaricato da internet ha il flusso
// ":Zone.Identifier". Ogni file estratto (anche da Estrai qui, tasto destro, procedura guidata,
// archivi riparati) riceve lo stesso marchio, così Windows SmartScreen e la Visualizzazione
// protetta di Office continuano ad avvisare (come chiede Microsoft ai programmi per archivi).

// Funzioni del sistema (variabili per le prove su Linux, dove i flussi non ci sono).
var (
	leggiZona  = leggiZonaSistema
	scriviZona = scriviZonaSistema
)

// zonaArchivio: il marchio della prima parte dell'archivio (o di una qualunque delle parti).
func zonaArchivio(parti []string) []byte {
	for _, p := range parti {
		if z := leggiZona(p); z != nil {
			return z
		}
	}
	return nil
}
