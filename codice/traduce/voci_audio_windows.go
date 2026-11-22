//go:build windows

package traduce

// Ascolto dell'audio del PC (quello che esce dalle casse o dalle cuffie) con WASAPI in modo
// "loopback": è la stessa cosa che fa Windows per registrare lo schermo con l'audio. Non tocca
// il gioco: legge solo il suono già mixato dalla scheda audio.

import (
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ole32             = windows.NewLazySystemDLL("ole32.dll")
	pCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	pCoUninitialize   = ole32.NewProc("CoUninitialize")
	pCoCreateInstance = ole32.NewProc("CoCreateInstance")
	pCoTaskMemFree    = ole32.NewProc("CoTaskMemFree")
	clsidMMDeviceEnum = windows.GUID{Data1: 0xBCDE0395, Data2: 0xE52F, Data3: 0x467C, Data4: [8]byte{0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E}}
	iidIMMDeviceEnum  = windows.GUID{Data1: 0xA95664D2, Data2: 0x9614, Data3: 0x4F35, Data4: [8]byte{0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6}}
	iidIAudioClient   = windows.GUID{Data1: 0x1CB9AD4C, Data2: 0xDBFA, Data3: 0x4C32, Data4: [8]byte{0xB1, 0x78, 0xC2, 0xF5, 0x68, 0xA7, 0x03, 0xB2}}
	iidIAudioCapture  = windows.GUID{Data1: 0xC8ADBD64, Data2: 0xE71E, Data3: 0x48A0, Data4: [8]byte{0xA4, 0xDE, 0x18, 0x5C, 0x39, 0x5C, 0xD3, 0x17}}
	subtipoFloat      = windows.GUID{Data1: 0x00000003, Data2: 0x0000, Data3: 0x0010, Data4: [8]byte{0x80, 0x00, 0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71}}
	errAudioNonDispon = errors.New("audio del PC non disponibile")
	livelloAudio      atomic.Int64 // ultimo livello sentito, in millesimi (per il collaudo e l'interfaccia)
)

type waveFormatEx struct {
	FormatTag      uint16
	Channels       uint16
	SamplesPerSec  uint32
	AvgBytesPerSec uint32
	BlockAlign     uint16
	BitsPerSample  uint16
	CbSize         uint16
}

// chiamata a un metodo COM: obj è un puntatore a un oggetto il cui primo campo è la vtable.
func com(obj unsafe.Pointer, indice int, args ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(vtbl, uintptr(indice)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(obj)}, args...)...)
	return r
}

func rilascia(obj unsafe.Pointer) {
	if obj != nil {
		com(obj, 2)
	}
}

// ascoltaPC: chiama consegna con audio mono a 16 kHz ogni ~20 ms finché ferma non si chiude.
func ascoltaPC(ferma <-chan struct{}, consegna func([]float32)) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pCoInitializeEx.Call(0, 0) // COINIT_MULTITHREADED
	defer pCoUninitialize.Call()

	var enum, dev, client, capt unsafe.Pointer
	if r, _, _ := pCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidMMDeviceEnum)), 0, 0x17 /*CLSCTX_ALL*/, uintptr(unsafe.Pointer(&iidIMMDeviceEnum)), uintptr(unsafe.Pointer(&enum))); r != 0 {
		return fmt.Errorf("%w (0x%x)", errAudioNonDispon, r)
	}
	defer rilascia(enum)
	if r := com(enum, 4, 0 /*eRender*/, 0 /*eConsole*/, uintptr(unsafe.Pointer(&dev))); r != 0 {
		return fmt.Errorf("nessuna uscita audio trovata (0x%x)", r)
	}
	defer rilascia(dev)
	if r := com(dev, 3, uintptr(unsafe.Pointer(&iidIAudioClient)), 0x17, 0, uintptr(unsafe.Pointer(&client))); r != 0 {
		return fmt.Errorf("%w (0x%x)", errAudioNonDispon, r)
	}
	defer rilascia(client)
	var pf unsafe.Pointer
	if r := com(client, 8, uintptr(unsafe.Pointer(&pf))); r != 0 || pf == nil {
		return fmt.Errorf("formato audio sconosciuto (0x%x)", r)
	}
	defer pCoTaskMemFree.Call(uintptr(pf))
	f := *(*waveFormatEx)(pf)
	isFloat := f.FormatTag == 3 || (f.FormatTag == 0xFFFE && f.CbSize >= 22 && *(*windows.GUID)(unsafe.Add(pf, 24)) == subtipoFloat /* SubFormat: byte 24 della struttura C */)
	if !(isFloat && f.BitsPerSample == 32) && f.BitsPerSample != 16 {
		return fmt.Errorf("formato audio non gestito: %d bit", f.BitsPerSample)
	}
	const loopback = 0x00020000
	if r := com(client, 3, 0 /*shared*/, loopback, 10000000 /*1 s*/, 0, uintptr(pf), 0); r != 0 {
		return fmt.Errorf("non riesco ad ascoltare l'audio (0x%x)", r)
	}
	if r := com(client, 14, uintptr(unsafe.Pointer(&iidIAudioCapture)), uintptr(unsafe.Pointer(&capt))); r != 0 {
		return fmt.Errorf("%w (0x%x)", errAudioNonDispon, r)
	}
	defer rilascia(capt)
	if r := com(client, 10); r != 0 {
		return fmt.Errorf("avvio audio non riuscito (0x%x)", r)
	}
	defer com(client, 11)

	canali := int(f.Channels)
	ultimoPacchetto := time.Now()
	var mono []float32
	for {
		select {
		case <-ferma:
			return nil
		default:
		}
		time.Sleep(10 * time.Millisecond)
		for {
			var n uint32
			if r := com(capt, 5, uintptr(unsafe.Pointer(&n))); r != 0 {
				return fmt.Errorf("l'uscita audio è cambiata o non risponde (0x%x)", uint32(r))
			}
			if n == 0 {
				break
			}
			var dati unsafe.Pointer
			var frame, flags uint32
			if r := com(capt, 3, uintptr(unsafe.Pointer(&dati)), uintptr(unsafe.Pointer(&frame)), uintptr(unsafe.Pointer(&flags)), 0, 0); r != 0 {
				if uint32(r) == 0x08890001 { // AUDCLNT_S_BUFFER_EMPTY
					break
				}
				return fmt.Errorf("l'uscita audio è cambiata o non risponde (0x%x)", uint32(r))
			}
			silenzio := flags&2 != 0 || dati == nil
			for i := 0; i < int(frame); i++ {
				var s float32
				if !silenzio {
					for c := 0; c < canali; c++ {
						if isFloat {
							s += *(*float32)(unsafe.Add(dati, (i*canali+c)*4))
						} else {
							s += float32(*(*int16)(unsafe.Add(dati, (i*canali+c)*2))) / 32768
						}
					}
					s /= float32(canali)
				}
				mono = append(mono, s)
			}
			com(capt, 4, uintptr(frame))
			ultimoPacchetto = time.Now()
		}
		// quando non suona niente Windows non manda pacchetti: genero silenzio io, così le
		// battute si chiudono anche se dopo la voce non c'è più nessun suono
		if time.Since(ultimoPacchetto) > 100*time.Millisecond {
			mono = append(mono, make([]float32, int(f.SamplesPerSec)/10)...)
			ultimoPacchetto = time.Now()
		}
		if len(mono) >= int(f.SamplesPerSec)/50 {
			r := riduci(mono, int(f.SamplesPerSec))
			livelloAudio.Store(int64(math.Min(1, rms(r)) * 1000))
			consegna(r)
			mono = mono[:0]
		}
	}
}
