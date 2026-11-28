// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package archivio

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// Errori con un codice: l'interfaccia li mostra con un messaggio chiaro nella lingua scelta.
var (
	ErrPassword          = errors.New("serve la password")
	ErrPasswordSbagliata = errors.New("password sbagliata")
	ErrAnnullato         = errors.New("annullato")
	ErrBomba             = errors.New("l'archivio si espanderebbe troppo (possibile zip bomb)")
	ErrTroppeVoci        = errors.New("l'archivio contiene troppi file")
	ErrSpazio            = errors.New("spazio su disco insufficiente")
	ErrFormato           = errors.New("formato non riconosciuto")
	ErrZipCifrato        = errors.New("zip con password: non supportato")
	// ErrZipCifratoAES: zip con password AES (Esplora file di Windows di solito non lo apre).
	ErrZipCifratoAES = fmt.Errorf("%w (AES)", ErrZipCifrato)
	// ErrMetodo: un metodo di compressione che Tasso Zip non sa ancora aprire (l'archivio è sano).
	ErrMetodo = errors.New("metodo di compressione non supportato")
	// ErrMetaRimasta: "Estrai qui" non è riuscito a finire e non ha potuto rimettere tutto
	// com'era: qualche file è rimasto nella cartella.
	ErrMetaRimasta   = errors.New("estrazione non finita: qualche file è rimasto nella cartella")
	ErrZipDiviso     = errors.New("zip diviso in parti .z01: non supportato")
	ErrParteMancante = errors.New("manca una parte dell'archivio")
	ErrRovinato      = errors.New("archivio rovinato o incompleto")
	ErrNienteDaFare  = errors.New("nessun file da comprimere")
	ErrNonScrivibile = errors.New("non posso scrivere nella cartella")

	errEsisteGia = errors.New("esiste già")
)

// Codice restituisce un codice breve per l'errore (usato per i testi dell'interfaccia).
func Codice(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrAnnullato), errors.Is(err, context.Canceled):
		return "annullato"
	case errors.Is(err, ErrPasswordSbagliata):
		return "password_sbagliata"
	case errors.Is(err, ErrPassword):
		return "password"
	case errors.Is(err, ErrMemoria):
		return "memoria"
	case errors.Is(err, ErrBomba):
		return "bomba"
	case errors.Is(err, ErrTroppeVoci):
		return "troppe_voci"
	case errors.Is(err, ErrSpazio), errors.Is(err, syscall.ENOSPC), erroreDiscoPieno(err):
		return "spazio"
	case errors.Is(err, ErrFormato):
		return "formato"
	case errors.Is(err, ErrMetodo):
		return "metodo"
	case errors.Is(err, ErrMetaRimasta):
		return "meta_rimasta"
	case errors.Is(err, ErrZipCifratoAES):
		return "zip_cifrato_aes"
	case errors.Is(err, ErrZipCifrato):
		return "zip_cifrato"
	case errors.Is(err, ErrZipDiviso):
		return "zip_diviso"
	case errors.Is(err, ErrParteMancante):
		return "parte_mancante"
	case errors.Is(err, ErrRovinato):
		return "rovinato"
	case errors.Is(err, ErrNienteDaFare):
		return "niente"
	case errors.Is(err, ErrNonScrivibile), errors.Is(err, fs.ErrPermission):
		return "permessi"
	case errors.Is(err, fs.ErrNotExist):
		return "non_trovato"
	}
	return "generico"
}

// erroreDiscoPieno riconosce "disco pieno" anche su Windows (ERROR_DISK_FULL = 112, ERROR_HANDLE_DISK_FULL = 39).
func erroreDiscoPieno(err error) bool {
	if !suWindows {
		return false
	}
	var pe *os.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	var en syscall.Errno
	if errors.As(err, &en) {
		return uintptr(en) == 112 || uintptr(en) == 39
	}
	return false
}
