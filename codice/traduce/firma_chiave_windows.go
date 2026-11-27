//go:build windows

package traduce

import "golang.org/x/sys/windows"

// La chiave delle firme: solo SYSTEM e Administrators (permessi non ereditati da Programmi,
// dove i file sono leggibili da tutti gli utenti e dalle app dello Store).
func init() {
	proteggiChiave = func(p string) error {
		sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;SY)(A;;FA;;;BA)")
		if err != nil {
			return err
		}
		dacl, _, err := sd.DACL()
		if err != nil {
			return err
		}
		return windows.SetNamedSecurityInfo(p, windows.SE_FILE_OBJECT,
			windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
	}
}
