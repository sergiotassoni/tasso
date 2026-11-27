package main

// Scena di prova DirectX 11 di Ottimizza tutto (3.2): Tasso disegna una sua scena (sfere e luci
// calcolate dalla scheda video, 1920×1080, fuori dallo schermo: niente finestra, niente giochi
// toccati) per circa 30 secondi e misura quanto ci mette ogni fotogramma. Ne escono gli FPS medi e
// gli FPS più bassi (1%). Usa la scheda video più potente (quella che userebbero i giochi).
// Tutto con le DLL di Windows (d3d11, dxgi, d3dcompiler_47), senza scaricare niente.
// Se qualcosa non parte (Windows senza DirectX 11, desktop remoto, driver di base) torna un
// errore e Ottimizza tutto continua senza FPS.

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	d3d11DLL         = windows.NewLazySystemDLL("d3d11.dll")
	pD3D11CreateDev  = d3d11DLL.NewProc("D3D11CreateDevice")
	d3dCompilerDLL   = windows.NewLazySystemDLL("d3dcompiler_47.dll")
	pD3DCompile      = d3dCompilerDLL.NewProc("D3DCompile")
	iidDXGIFactory6  = windows.GUID{Data1: 0xc1b6694f, Data2: 0xff09, Data3: 0x44a9, Data4: [8]byte{0xb0, 0x3c, 0x77, 0x90, 0x0a, 0x0a, 0x1d, 0x17}}
	iidDXGIAdapter1  = windows.GUID{Data1: 0x29038f61, Data2: 0x3839, Data3: 0x4626, Data4: [8]byte{0x91, 0xfd, 0x08, 0x68, 0x79, 0x01, 0x1a, 0x05}}
	errScenaNonParte = errors.New("scena di prova non partita")
)

// Il disegno: una stanza di sfere colorate fatta con il raymarching (tutto nella scheda video).
const scenaHLSL = `
cbuffer C : register(b0) { float T; float3 _p; };
float4 VS(uint id : SV_VertexID) : SV_Position {
  float2 p = float2((id << 1) & 2, id & 2);
  return float4(p * float2(2, -2) + float2(-1, 1), 0, 1);
}
float mappa(float3 p) {
  float3 q = p; q.xz = fmod(abs(q.xz) + 100.0, 2.0) - 1.0;
  float s = length(q - float3(0, 0.35 + 0.15 * sin(T + p.x), 0)) - 0.42;
  return min(s, p.y + 0.5);
}
float3 normale(float3 p) {
  float2 e = float2(0.002, 0);
  return normalize(float3(mappa(p + e.xyy) - mappa(p - e.xyy), mappa(p + e.yxy) - mappa(p - e.yxy), mappa(p + e.yyx) - mappa(p - e.yyx)));
}
float4 PS(float4 pos : SV_Position) : SV_Target {
  float2 uv = (pos.xy - float2(960, 540)) / 540.0;
  float3 ro = float3(sin(T * 0.3) * 3.0, 1.2, T * 0.8), rd = normalize(float3(uv.x, -uv.y - 0.25, 1.4));
  float t = 0.0; float3 c = float3(0.02, 0.03, 0.05);
  [loop] for (int i = 0; i < 96; i++) {
    float d = mappa(ro + rd * t);
    if (d < 0.001 || t > 40.0) break;
    t += d;
  }
  if (t < 40.0) {
    float3 p = ro + rd * t, n = normale(p), l = normalize(float3(0.6, 0.8, -0.4));
    float dif = saturate(dot(n, l)), amb = 0.25 + 0.25 * n.y;
    float3 base = 0.5 + 0.5 * cos(float3(0, 2, 4) + floor(p.x) + floor(p.z));
    float sp = pow(saturate(dot(reflect(-l, n), -rd)), 32.0);
    c = base * (dif + amb) + sp;
    c = lerp(c, float3(0.02, 0.03, 0.05), saturate(t / 40.0));
  }
  return float4(pow(c, 0.4545), 1);
}
`

// adattatorePotente: la scheda video ad alte prestazioni (IDXGIFactory6, Windows 10 1803+), se no
// quella con più memoria video. 0 = quella di base di Windows.
func adattatorePotente() uintptr {
	if pCreateDXGIFactory.Find() != nil {
		return 0
	}
	var fac uintptr
	if hr, _, _ := pCreateDXGIFactory.Call(uintptr(unsafe.Pointer(&iidDXGIFactory1)), uintptr(unsafe.Pointer(&fac))); !hrOK(hr) || fac == 0 {
		return 0
	}
	defer comCall(fac, 2)
	var f6 uintptr
	if hr := comCall(fac, 0, uintptr(unsafe.Pointer(&iidDXGIFactory6)), uintptr(unsafe.Pointer(&f6))); hrOK(hr) && f6 != 0 {
		defer comCall(f6, 2)
		var ad uintptr
		// EnumAdapterByGpuPreference(0, DXGI_GPU_PREFERENCE_HIGH_PERFORMANCE)
		if hr := comCall(f6, 29, 0, 2, uintptr(unsafe.Pointer(&iidDXGIAdapter1)), uintptr(unsafe.Pointer(&ad))); hrOK(hr) && ad != 0 {
			var d dxgiAdapterDesc1
			if hrOK(comCall(ad, 10, uintptr(unsafe.Pointer(&d)))) && d.Flags&2 == 0 {
				return ad
			}
			comCall(ad, 2)
		}
	}
	var best uintptr
	var bestMem uintptr
	for i := uintptr(0); i < 16; i++ {
		var ad uintptr
		if hr := comCall(fac, 12, i, uintptr(unsafe.Pointer(&ad))); !hrOK(hr) || ad == 0 {
			break
		}
		var d dxgiAdapterDesc1
		if hrOK(comCall(ad, 10, uintptr(unsafe.Pointer(&d)))) && d.Flags&2 == 0 && d.VendorID != 0x1414 && (best == 0 || d.DedicatedVideoMemory > bestMem) {
			if best != 0 {
				comCall(best, 2)
			}
			best, bestMem = ad, d.DedicatedVideoMemory
			continue
		}
		comCall(ad, 2)
	}
	return best
}

// compilaShader: HLSL → codice della scheda video (ID3DBlob). Restituisce il blob (da liberare).
func compilaShader(entry, target string) (uintptr, error) {
	src := []byte(scenaHLSL)
	e, _ := windows.BytePtrFromString(entry)
	t, _ := windows.BytePtrFromString(target)
	var code, errs uintptr
	hr, _, _ := pD3DCompile.Call(uintptr(unsafe.Pointer(&src[0])), uintptr(len(src)), 0, 0, 0,
		uintptr(unsafe.Pointer(e)), uintptr(unsafe.Pointer(t)), 1<<15 /* OPTIMIZATION_LEVEL3 */, 0,
		uintptr(unsafe.Pointer(&code)), uintptr(unsafe.Pointer(&errs)))
	if errs != 0 {
		comCall(errs, 2)
	}
	if !hrOK(hr) || code == 0 {
		return 0, fmt.Errorf("shader %s: 0x%x", entry, uint32(hr))
	}
	return code, nil
}

// scenaFPS disegna la scena per d (dopo 3 secondi di riscaldamento) e restituisce la durata di
// ogni fotogramma. onProg riceve l'avanzamento da 0 a 1.
func scenaFPS(ctx context.Context, d time.Duration, onProg func(float64)) (durate []time.Duration, err error) {
	defer func() {
		if r := recover(); r != nil {
			durate, err = nil, fmt.Errorf("%w: %v", errScenaNonParte, r)
		}
	}()
	if pD3D11CreateDev.Find() != nil || pD3DCompile.Find() != nil {
		return nil, errScenaNonParte
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	ad := adattatorePotente()
	if ad != 0 {
		defer comCall(ad, 2)
	}
	driver := uintptr(1) // D3D_DRIVER_TYPE_HARDWARE
	if ad != 0 {
		driver = 0 // con una scheda scelta: D3D_DRIVER_TYPE_UNKNOWN
	}
	livelli := [3]uint32{0xb000, 0xa100, 0xa000} // 11_0, 10_1, 10_0
	var dev, ctxD uintptr
	var livello uint32
	hr, _, _ := pD3D11CreateDev.Call(ad, driver, 0, 0, uintptr(unsafe.Pointer(&livelli[0])), 3, 7,
		uintptr(unsafe.Pointer(&dev)), uintptr(unsafe.Pointer(&livello)), uintptr(unsafe.Pointer(&ctxD)))
	if !hrOK(hr) || dev == 0 || ctxD == 0 {
		return nil, fmt.Errorf("%w: D3D11CreateDevice 0x%x", errScenaNonParte, uint32(hr))
	}
	defer comCall(dev, 2)
	defer comCall(ctxD, 2)

	target := "ps_5_0"
	vtarget := "vs_5_0"
	if livello < 0xb000 {
		target, vtarget = "ps_4_0", "vs_4_0"
	}
	vsB, err := compilaShader("VS", vtarget)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errScenaNonParte, err)
	}
	defer comCall(vsB, 2)
	psB, err := compilaShader("PS", target)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errScenaNonParte, err)
	}
	defer comCall(psB, 2)
	var vs, ps uintptr
	if !hrOK(comCall(dev, 12, comCall(vsB, 3), comCall(vsB, 4), 0, uintptr(unsafe.Pointer(&vs)))) || vs == 0 {
		return nil, errScenaNonParte
	}
	defer comCall(vs, 2)
	if !hrOK(comCall(dev, 15, comCall(psB, 3), comCall(psB, 4), 0, uintptr(unsafe.Pointer(&ps)))) || ps == 0 {
		return nil, errScenaNonParte
	}
	defer comCall(ps, 2)

	// la tela: 1920×1080, fuori dallo schermo
	tex := struct {
		W, H, Mip, Array, Format, SCount, SQuality, Usage, Bind, CPU, Misc uint32
	}{1920, 1080, 1, 1, 28 /* R8G8B8A8_UNORM */, 1, 0, 0, 0x20 /* RENDER_TARGET */, 0, 0}
	var tela, rtv uintptr
	if !hrOK(comCall(dev, 5, uintptr(unsafe.Pointer(&tex)), 0, uintptr(unsafe.Pointer(&tela)))) || tela == 0 {
		return nil, errScenaNonParte
	}
	defer comCall(tela, 2)
	if !hrOK(comCall(dev, 9, tela, 0, uintptr(unsafe.Pointer(&rtv)))) || rtv == 0 {
		return nil, errScenaNonParte
	}
	defer comCall(rtv, 2)
	// il tempo della scena (per farla muovere)
	cbd := struct{ Size, Usage, Bind, CPU, Misc, Stride uint32 }{16, 0, 4 /* CONSTANT_BUFFER */, 0, 0, 0}
	var cb uintptr
	if !hrOK(comCall(dev, 3, uintptr(unsafe.Pointer(&cbd)), 0, uintptr(unsafe.Pointer(&cb)))) || cb == 0 {
		return nil, errScenaNonParte
	}
	defer comCall(cb, 2)
	qd := struct{ Query, Misc uint32 }{0 /* EVENT */, 0}
	var q uintptr
	if !hrOK(comCall(dev, 24, uintptr(unsafe.Pointer(&qd)), uintptr(unsafe.Pointer(&q)))) || q == 0 {
		return nil, errScenaNonParte
	}
	defer comCall(q, 2)

	vp := struct{ X, Y, W, H, Min, Max float32 }{0, 0, 1920, 1080, 0, 1}
	comCall(ctxD, 33, 1, uintptr(unsafe.Pointer(&rtv)), 0) // OMSetRenderTargets
	comCall(ctxD, 44, 1, uintptr(unsafe.Pointer(&vp)))     // RSSetViewports
	comCall(ctxD, 24, 4)                                   // IASetPrimitiveTopology(TRIANGLELIST)
	comCall(ctxD, 11, vs, 0, 0)                            // VSSetShader
	comCall(ctxD, 9, ps, 0, 0)                             // PSSetShader
	comCall(ctxD, 16, 0, 1, uintptr(unsafe.Pointer(&cb)))  // PSSetConstantBuffers

	var tempo [4]float32
	fotogramma := func(n int) (time.Duration, error) {
		t0 := time.Now()
		tempo[0] = float32(n) / 60
		comCall(ctxD, 48, cb, 0, 0, uintptr(unsafe.Pointer(&tempo[0])), 0, 0) // UpdateSubresource
		comCall(ctxD, 13, 3, 0)                                               // Draw(3, 0)
		comCall(ctxD, 28, q)                                                  // End(query)
		for {
			r := comCall(ctxD, 29, q, 0, 0, 0) // GetData: S_OK quando la scheda ha finito
			if r == 0 {
				break
			}
			if !hrOK(r) {
				return 0, fmt.Errorf("%w: scheda video persa (0x%x)", errScenaNonParte, uint32(r))
			}
			if time.Since(t0) > 3*time.Second {
				return 0, fmt.Errorf("%w: fotogramma troppo lento", errScenaNonParte)
			}
		}
		return time.Since(t0), nil
	}
	// riscaldamento: la scheda video sale di frequenza
	inizio := time.Now()
	n := 0
	for time.Since(inizio) < 3*time.Second {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if _, err := fotogramma(n); err != nil {
			return nil, err
		}
		n++
	}
	inizio = time.Now()
	ultimo := time.Now()
	for time.Since(inizio) < d {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		dt, err := fotogramma(n)
		if err != nil {
			return nil, err
		}
		durate = append(durate, dt)
		n++
		if onProg != nil && time.Since(ultimo) > 500*time.Millisecond {
			ultimo = time.Now()
			onProg(time.Since(inizio).Seconds() / d.Seconds())
		}
	}
	comCall(ctxD, 110) // ClearState
	return durate, nil
}
