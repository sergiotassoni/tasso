package installa

// File ancora occupati subito dopo la chiusura di Tasso: Windows (e a volte l'antivirus) lascia
// andare i file un attimo dopo che il programma è uscito. Invece di fallire subito riprovo per
// qualche secondo; se un file resta occupato lo sposto accanto (Tasso.exe.vecchio-1: Windows
// permette di rinominare un exe o una dll in uso) e lo faccio cancellare al prossimo riavvio.

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

// Quanto riprovare con un file occupato: 60 tentativi a 250 ms = 15 secondi.
const (
	PausaRiprova     = 250 * time.Millisecond
	TentativiRiprova = 60
)

// Codici di Windows per "file occupato da un altro programma".
var codiciBloccato = map[uintptr]bool{
	5:    true, // ERROR_ACCESS_DENIED (anche un exe appena chiuso, per un attimo)
	32:   true, // ERROR_SHARING_VIOLATION
	33:   true, // ERROR_LOCK_VIOLATION
	1224: true, // ERROR_USER_MAPPED_FILE
}

// ErrFileBloccato: un file è ancora usato da un programma (anche dopo aver riprovato).
var ErrFileBloccato = errors.New("file ancora in uso da un programma")

// ErroreBloccato: quale file era occupato e l'errore vero di Windows.
type ErroreBloccato struct {
	File string
	Err  error
}

func (e *ErroreBloccato) Error() string {
	return fmt.Sprintf("%s è ancora usato da un programma (%v)", e.File, e.Err)
}
func (e *ErroreBloccato) Unwrap() []error { return []error{ErrFileBloccato, e.Err} }

// BloccatoWindows: err viene da Windows con uno dei codici di "file occupato".
func BloccatoWindows(err error) bool {
	var en syscall.Errno
	return errors.As(err, &en) && codiciBloccato[uintptr(en)]
}

func (in *Installatore) bloccato(err error) bool {
	if err == nil {
		return false
	}
	if in.Bloccato != nil {
		return in.Bloccato(err)
	}
	return runtime.GOOS == "windows" && BloccatoWindows(err)
}

func (in *Installatore) dormi(d time.Duration) {
	if in.Dormi != nil {
		in.Dormi(d)
		return
	}
	time.Sleep(d)
}

// Riprova esegue op; se l'errore è "file occupato" (bloccato) riprova fino a tentativi volte
// in tutto, con una pausa tra un tentativo e l'altro. Restituisce i tentativi fatti e l'ultimo errore.
func Riprova(op func() error, bloccato func(error) bool, dormi func(time.Duration), tentativi int, pausa time.Duration) (int, error) {
	if tentativi < 1 {
		tentativi = 1
	}
	var err error
	for i := 1; ; i++ {
		if err = op(); err == nil || !bloccato(err) || i >= tentativi {
			return i, err
		}
		dormi(pausa)
	}
}

// conRiprova: Riprova con le impostazioni dell'installatore, e scrive nel diario cosa è successo.
func (in *Installatore) conRiprova(cosa string, op func() error) error {
	n, err := Riprova(op, in.bloccato, in.dormi, TentativiRiprova, PausaRiprova)
	switch {
	case err == nil && n > 1:
		Diario("%s: riuscito al tentativo %d (il file era occupato)", cosa, n)
	case err != nil && n > 1:
		Diario("%s: ancora occupato dopo %d tentativi: %v", cosa, n, err)
	case err != nil:
		Diario("%s: errore: %v", cosa, err)
	}
	return err
}

// accantoLibero: dst.vecchio-1, dst.vecchio-2... il primo che non c'è.
func accantoLibero(dst string) string {
	for n := 1; ; n++ {
		p := dst + ".vecchio-" + strconv.Itoa(n)
		if _, err := os.Lstat(p); os.IsNotExist(err) {
			return p
		}
	}
}
