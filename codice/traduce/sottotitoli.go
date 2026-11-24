package traduce

// sottotitoloPronto: con la zona dei sottotitoli il testo si legge ogni mezzo secondo.
//   - Nei video lo sfondo si muove e l'OCR sbaglia una lettera ogni tanto: se è lo stesso
//     sottotitolo di prima non lo ritraduco (la traduzione cambiava o lampeggiava sotto gli occhi).
//   - Un sottotitolo che sta comparendo (dissolvenza, parole una alla volta) si legge a metà:
//     traduco solo quando due letture di fila dicono la stessa cosa.
//
// Restituisce se tradurre adesso e la lettura da tenere come candidata per il giro dopo.
func sottotitoloPronto(letto, ultimo, candidato string) (bool, string) {
	if ultimo != "" && (letto == ultimo || quasiUguale(letto, ultimo)) {
		return false, candidato
	}
	if letto != candidato && !quasiUguale(letto, candidato) {
		return false, letto
	}
	return true, letto
}
