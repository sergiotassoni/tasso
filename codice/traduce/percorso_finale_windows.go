package traduce

import (
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

// finaleDiHandle: il percorso vero di un file o di una cartella aperti (GetFinalPathNameByHandle:
// collegamenti e giunzioni già risolti). "" se non si sa o se è in rete.
func finaleDiHandle(h windows.Handle) string {
	buf := make([]uint16, 1024)
	for i := 0; i < 2; i++ {
		n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
		if err != nil || n == 0 {
			return ""
		}
		if int(n) < len(buf) {
			s := strings.TrimPrefix(windows.UTF16ToString(buf[:n]), `\\?\`)
			if strings.HasPrefix(strings.ToUpper(s), `UNC\`) || strings.HasPrefix(s, `\\`) {
				return ""
			}
			return strings.TrimRight(s, `\`)
		}
		buf = make([]uint16, n+1)
	}
	return ""
}

// radiceVera: il percorso vero della cartella radice, calcolato UNA volta prima di guardarci dentro.
// La apro senza seguire collegamenti (FILE_FLAG_OPEN_REPARSE_POINT): se la radice stessa è un
// collegamento o una giunzione → errLink. Se Windows non sa dire il percorso → errNonVerificato.
// Chi la usa confronta poi ogni file con questa stringa fissa: uno scambio fatto dopo porta i file
// fuori da lei, e vengono rifiutati.
func radiceVera(p string) (string, error) {
	u, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return "", errNonVerificato
	}
	h, err := windows.CreateFile(u, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return "", err
		}
		return "", errNonVerificato
	}
	defer windows.CloseHandle(h)
	var fi windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(h, &fi) != nil {
		return "", errNonVerificato
	}
	if fi.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || fi.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return "", errLink
	}
	v := finaleDiHandle(h)
	if v == "" {
		return "", errNonVerificato
	}
	return v, nil
}

// controllaAperto: il file aperto f sta davvero sotto radice (percorso vero già calcolato con
// radiceVera). errNonVerificato se Windows non sa dire dove sta; errLink se sta fuori.
// unNome=true rifiuta anche i file con più nomi (hard link).
func controllaAperto(f *os.File, radice string, unNome bool) error {
	h := windows.Handle(f.Fd())
	p := finaleDiHandle(h)
	if p == "" {
		return errNonVerificato
	}
	if !dentroFinale(p, radice) {
		return errLink
	}
	if unNome {
		var fi windows.ByHandleFileInformation
		if windows.GetFileInformationByHandle(h, &fi) != nil {
			return errNonVerificato
		}
		if fi.NumberOfLinks > 1 {
			return errLink
		}
	}
	return nil
}
