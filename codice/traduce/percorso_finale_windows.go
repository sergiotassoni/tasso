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

// finaleCartella: il percorso vero della cartella radice (aperta con BACKUP_SEMANTICS).
func finaleCartella(p string) string {
	u, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return ""
	}
	h, err := windows.CreateFile(u, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	return finaleDiHandle(h)
}

// apertoDentro: il file aperto f sta davvero sotto la cartella radice (anche se una cartella in
// mezzo è stata cambiata in una giunzione dopo i controlli).
func apertoDentro(f *os.File, radice string) bool {
	r := finaleCartella(radice)
	p := finaleDiHandle(windows.Handle(f.Fd()))
	return dentroFinale(p, r)
}
