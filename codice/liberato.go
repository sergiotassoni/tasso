package main

// Spazio liberato da una pulizia, onesto: i passaggi sommano i byte dei file che hanno tolto, ma
// alcune cose (DISM, Ottimizzazione recapito, file che Windows libera dopo) non si contano bene.
// Per questo si misura anche lo spazio libero dei dischi prima e dopo.

// diffSpazioLibero: quanto spazio libero in più c'è adesso sui dischi misurati sia prima che dopo
// (può essere negativo se nel frattempo altri programmi hanno scritto, es. Windows Update).
func diffSpazioLibero(prima, dopo map[string]uint64) int64 {
	var d int64
	for k, a := range prima {
		b, ok := dopo[k]
		if !ok {
			continue
		}
		d += int64(b) - int64(a)
	}
	return d
}

// spazioOnesto: il numero da mostrare. La differenza vera dei dischi quando è più grande della somma
// dei passaggi (spazio liberato che i passaggi non sapevano contare); la somma quando i dischi
// dicono meno (altri programmi hanno scritto nel frattempo). Mai sotto zero.
func spazioOnesto(somma, reale int64) int64 {
	if somma < 0 {
		somma = 0
	}
	if reale > somma {
		return reale
	}
	return somma
}
