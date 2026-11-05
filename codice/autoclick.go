package main

// Autoclick: clicca da solo ogni tot tempo, quante volte vuoi, dove sta il mouse o in un punto
// fisso, con piccoli movimenti facoltativi che restano dentro la finestra del gioco.
// Si avvia e si ferma con un tasto a scelta (di base F6) anche da dentro il gioco.

import (
	"math/rand"
	"sync"
	"time"
	"unsafe"
)

var (
	pSendInput    = user32.NewProc("SendInput")
	pSetCursorPos = user32.NewProc("SetCursorPos")
	pGetCursorPos = user32.NewProc("GetCursorPos")
	pGetFgWindow  = user32.NewProc("GetForegroundWindow")
	pGetAsyncKey  = user32.NewProc("GetAsyncKeyState")
)

type acConfig struct {
	IntervalMs int64  `json:"intervalMs"` // ogni quanto
	Count      int    `json:"count"`      // 0 = all'infinito
	Button     string `json:"button"`     // "sx", "dx", "centro"
	Double     bool   `json:"double"`
	Fixed      bool   `json:"fixed"` // punto fisso invece della posizione del mouse
	X          int32  `json:"x"`
	Y          int32  `json:"y"`
	Jiggle     int32  `json:"jiggle"`   // pixel di movimento casuale (0 = fermo)
	DelaySec   int    `json:"delaySec"` // attesa prima di partire (per andare nel gioco)
}

type mouseInput struct {
	typ       uint32
	_         uint32
	dx, dy    int32
	mouseData uint32
	flags     uint32
	time      uint32
	_         uint32
	extra     uintptr
}

type point struct{ X, Y int32 }

var (
	acMu      sync.Mutex
	acCfg     = acConfig{IntervalMs: 1000, Button: "sx", DelaySec: 3}
	acRunning bool
	acStop    chan struct{}
	acDone    int
)

func autoClickSet(c acConfig) string {
	if c.IntervalMs < 10 {
		c.IntervalMs = 10 // massimo 100 click al secondo
	}
	if c.IntervalMs > 3600_000 {
		c.IntervalMs = 3600_000
	}
	if c.Count < 0 {
		c.Count = 0
	}
	if c.Jiggle < 0 {
		c.Jiggle = 0
	}
	if c.Jiggle > 50 {
		c.Jiggle = 50
	}
	if c.DelaySec < 0 || c.DelaySec > 30 {
		c.DelaySec = 3
	}
	switch c.Button {
	case "sx", "dx", "centro":
	default:
		c.Button = "sx"
	}
	acMu.Lock()
	acCfg = c
	acMu.Unlock()
	return "ok"
}

// autoClickPick: dopo 3 secondi legge dove si trova il mouse (il tempo di portarlo sul punto giusto).
func autoClickPick() {
	for i := 3; i > 0; i-- {
		emit("onAutoClickPick", map[string]interface{}{"wait": i})
		time.Sleep(time.Second)
	}
	var p point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	acMu.Lock()
	acCfg.X, acCfg.Y, acCfg.Fixed = p.X, p.Y, true
	acMu.Unlock()
	emit("onAutoClickPick", map[string]interface{}{"x": p.X, "y": p.Y})
}

// Giochi con anti-cheat: un autoclick lì può far bannare l'account, quindi non parte.
var antiCheatExes = []string{"VALORANT-Win64-Shipping.exe", "EasyAntiCheat.exe", "EasyAntiCheat_EOS.exe", "BEService.exe", "BEService_x64.exe", "FortniteClient-Win64-Shipping.exe", "r5apex.exe", "r5apex_dx12.exe", "League of Legends.exe", "EAAntiCheat.GameService.exe", "cod.exe"}

const antiCheatMsg = "È aperto un gioco con anti-cheat (per esempio Valorant, League of Legends, Fortnite, Apex, Call of Duty o i giochi con BattlEye): lì l'autoclick può farti bannare l'account, quindi non lo uso."

func antiCheatRunning() bool { return exeRunning(antiCheatExes) }

func autoClickStart(fromHotkey bool) string {
	if antiCheatRunning() {
		if fromHotkey {
			emit("onAutoClick", map[string]interface{}{"running": false, "msg": antiCheatMsg})
		}
		return antiCheatMsg
	}
	setMu.Lock()
	paused := settings.Paused
	setMu.Unlock()
	if paused {
		return "Tasso è spento: riaccendilo per usare l'autoclick."
	}
	acMu.Lock()
	if acRunning {
		acMu.Unlock()
		return "busy"
	}
	acRunning, acDone = true, 0
	stop := make(chan struct{})
	acStop = stop
	cfg := acCfg
	acMu.Unlock()
	go func() {
		defer guard("autoclick.go:129")
		defer func() {
			acMu.Lock()
			acRunning = false
			n := acDone
			acMu.Unlock()
			emit("onAutoClick", map[string]interface{}{"running": false, "done": n})
		}()
		delay := cfg.DelaySec
		if fromHotkey {
			delay = 0 // con il tasto rapido sei già nel gioco
		}
		for i := delay; i > 0; i-- {
			emit("onAutoClick", map[string]interface{}{"running": true, "wait": i})
			select {
			case <-stop:
				return
			case <-time.After(time.Second):
			}
		}
		// Esc ferma sempre l'autoclick: lo controllo spesso, anche se i click sono lenti.
		escStop := make(chan struct{})
		runDone := make(chan struct{})
		defer close(runDone)
		go func() {
			defer guard("autoclick.go:153")
			pGetAsyncKey.Call(0x1B) // azzera lo stato "premuto da prima"
			t := time.NewTicker(25 * time.Millisecond)
			defer t.Stop()
			for {
				select {
				case <-stop:
					return
				case <-runDone:
					return
				case <-t.C:
					if r, _, _ := pGetAsyncKey.Call(0x1B); r&0x8001 != 0 {
						close(escStop)
						return
					}
				}
			}
		}()
		emit("onAutoClick", map[string]interface{}{"running": true, "done": 0})
		t := time.NewTicker(time.Duration(cfg.IntervalMs) * time.Millisecond)
		defer t.Stop()
		lastEmit := time.Now()
		lastAC := time.Time{} // il primo controllo subito, appena finito il conto alla rovescia
		for {
			if time.Since(lastAC) > 500*time.Millisecond {
				lastAC = time.Now()
				if antiCheatRunning() {
					emit("onAutoClick", map[string]interface{}{"running": false, "msg": antiCheatMsg})
					return
				}
			}
			acClickOnce(cfg)
			acMu.Lock()
			acDone++
			n := acDone
			acMu.Unlock()
			if time.Since(lastEmit) > 500*time.Millisecond {
				emit("onAutoClick", map[string]interface{}{"running": true, "done": n})
				lastEmit = time.Now()
			}
			if cfg.Count > 0 && n >= cfg.Count {
				return
			}
			select {
			case <-stop:
				return
			case <-escStop:
				return
			case <-t.C:
			}
		}
	}()
	return "ok"
}

func autoClickStop() string {
	acMu.Lock()
	if acRunning && acStop != nil {
		close(acStop)
		acStop = nil
	}
	acMu.Unlock()
	return "ok"
}

func acClickOnce(cfg acConfig) {
	if cfg.Fixed {
		pSetCursorPos.Call(uintptr(cfg.X), uintptr(cfg.Y))
	}
	if cfg.Jiggle > 0 {
		var p point
		pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
		bx, by := p.X, p.Y
		if cfg.Fixed {
			bx, by = cfg.X, cfg.Y
		}
		nx := bx + rand.Int31n(2*cfg.Jiggle+1) - cfg.Jiggle
		ny := by + rand.Int31n(2*cfg.Jiggle+1) - cfg.Jiggle
		// Resto dentro la finestra in primo piano (il gioco), con un margine di 8 pixel.
		if fg, _, _ := pGetFgWindow.Call(); fg != 0 {
			var r rect
			pGetWindowRect.Call(fg, uintptr(unsafe.Pointer(&r)))
			if r.Right-r.Left > 40 && r.Bottom-r.Top > 40 {
				nx = clamp32(nx, r.Left+8, r.Right-8)
				ny = clamp32(ny, r.Top+8, r.Bottom-8)
			}
		}
		pSetCursorPos.Call(uintptr(nx), uintptr(ny))
	}
	down, up := uint32(0x0002), uint32(0x0004)
	switch cfg.Button {
	case "dx":
		down, up = 0x0008, 0x0010
	case "centro":
		down, up = 0x0020, 0x0040
	}
	times := 1
	if cfg.Double {
		times = 2
	}
	for i := 0; i < times; i++ {
		in := [2]mouseInput{{typ: 0, flags: down}, {typ: 0, flags: up}}
		pSendInput.Call(2, uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
		if times == 2 && i == 0 {
			time.Sleep(40 * time.Millisecond)
		}
	}
}

func clamp32(v, lo, hi int32) int32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
