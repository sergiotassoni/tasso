// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package archivio

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// OpzioniVerifica: come verificare un archivio.
type OpzioniVerifica struct {
	Password    string
	Avanzamento FunzioneAvanzamento
}

// RisultatoVerifica: cosa è stato controllato.
type RisultatoVerifica struct {
	Tipo     Tipo  `json:"tipo"`
	File     int   `json:"file"`     // file letti e controllati
	Cartelle int   `json:"cartelle"` // cartelle
	Byte     int64 `json:"byte"`     // byte decompressi e controllati
}

// Verifica prova l'integrità dell'archivio senza estrarre niente: legge e decomprime tutti i
// dati e controlla le somme di controllo (CRC32 di zip, 7z, rar e gz, CRC di bzip2 e xz,
// zstd se l'archivio ce l'ha). Il tar semplice non ha somme dei dati: si controlla che sia
// leggibile fino in fondo. Non scrive niente sul disco.
func Verifica(ctx context.Context, percorso string, opz OpzioniVerifica) (ris RisultatoVerifica, err error) {
	percorso, err = filepath.Abs(percorso)
	if err != nil {
		return ris, err
	}
	fi, err := os.Stat(percorso)
	if err != nil {
		return ris, err
	}
	if !fi.Mode().IsRegular() {
		return ris, ErrFormato
	}
	l := &lavoroEstrai{ctx: ctx, opz: OpzioniEstrai{Password: opz.Password, Avanzamento: opz.Avanzamento}}
	percorso, tipo, err := l.apri(percorso)
	if err != nil {
		return ris, err
	}
	defer l.vol.Close()
	ris.Tipo = tipo
	limiti := LimitiPredefiniti()
	l.d = nuovaDestinazione(ctx, "", limiti, l.vol.Size())
	l.d.prova = true
	l.d.avanzamento = l.avanza

	switch tipo {
	case TipoZip:
		err = l.zip()
	case Tipo7z:
		err = l.setteZip()
	case TipoRar:
		err = l.rar(percorso)
	case TipoTar, TipoGz, TipoBz2, TipoXz, TipoZst:
		l.flusso = true
		err = l.flussoTar(tipo, percorso)
	default:
		err = ErrFormato
	}
	if err == nil {
		err = l.d.annullato()
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			err = ErrAnnullato
		}
		return ris, err
	}
	ris.File, ris.Cartelle, ris.Byte = l.d.file, l.d.cartelleProva, l.d.scritti
	return ris, nil
}
