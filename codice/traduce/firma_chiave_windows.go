//go:build windows

package traduce

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// sdChiave: solo SYSTEM e Administrators, permessi non ereditati da Programmi (dove i file sono
// leggibili da tutti gli utenti e dalle app dello Store).
const sdChiave = "D:P(A;;FA;;;SY)(A;;FA;;;BA)"

func init() {
	proteggiChiave = func(p string) error {
		sd, err := windows.SecurityDescriptorFromString(sdChiave)
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
	creaFileChiave = func(p string) (*os.File, error) {
		sd, err := windows.SecurityDescriptorFromString(sdChiave)
		if err != nil {
			return nil, err
		}
		sa := windows.SecurityAttributes{SecurityDescriptor: sd}
		sa.Length = uint32(unsafe.Sizeof(sa))
		n, err := windows.UTF16PtrFromString(p)
		if err != nil {
			return nil, err
		}
		h, err := windows.CreateFile(n, windows.GENERIC_WRITE, 0, &sa, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
		if err != nil {
			return nil, err
		}
		return os.NewFile(uintptr(h), p), nil
	}
}
