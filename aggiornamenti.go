// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

// Aggiornamenti: Tasso PDF non usa internet. Gli aggiornamenti dell'Ecosistema Tasso li gestisce
// Tasso (Impostazioni → Aggiornamenti): "Controlla aggiornamenti" (finestra Informazioni) apre
// Tasso con --controlla-aggiornamenti. Se Tasso non c'è, la pagina delle versioni si apre solo
// dopo che l'utente ha detto sì (lo chiede la pagina).

// paginaVersioni: le versioni di Tasso (con l'installatore dell'ecosistema).
const paginaVersioni = "https://github.com/sergiotassoni/tasso/releases/latest"

// goControllaAggiornamenti: "tasso" (Tasso aperto su Aggiornamenti), "nessuno" (Tasso non è
// installato) o "errore" (Windows non l'ha aperto, per esempio hai detto no al permesso).
func goControllaAggiornamenti() string {
	if piattaforma.apriTassoAgg == nil {
		return "nessuno"
	}
	return piattaforma.apriTassoAgg()
}

// goPaginaVersioni: la pagina delle versioni nel browser (la pagina la chiama solo dopo un sì).
func goPaginaVersioni() error {
	if piattaforma.apriLink == nil {
		return nil
	}
	return perPagina(piattaforma.apriLink(paginaVersioni))
}
