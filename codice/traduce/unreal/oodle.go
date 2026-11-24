package unreal

// Decompressione Oodle con ooz (GPL-3, vedi strumenti/ooz-wasm) compilato in WebAssembly ed
// eseguito da wazero: niente DLL esterne, niente codice nativo scaricato.

import (
	"context"
	_ "embed"
	"errors"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

//go:embed ooz.wasm
var oozWasm []byte

var (
	muOodle  sync.Mutex
	oodleMod api.Module
	oodleErr error
	oodleRt  wazero.Runtime
)

func caricaOodle() (api.Module, error) {
	if oodleMod != nil || oodleErr != nil {
		return oodleMod, oodleErr
	}
	ctx := context.Background()
	oodleRt = wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter().WithMemoryLimitPages(16384)) // 1 GB al massimo
	m, err := oodleRt.Instantiate(ctx, oozWasm)
	if err != nil {
		oodleErr = err
		return nil, err
	}
	oodleMod = m
	return m, nil
}

// spazioSicuro: ooz scrive fino a 64 byte oltre la fine del buffer di uscita.
const spazioSicuro = 64

// DecomprimiOodle: src compresso con Oodle, n = grandezza decompressa.
func DecomprimiOodle(src []byte, n int) ([]byte, error) {
	if n < 0 || n > 512<<20 || len(src) > 512<<20 {
		return nil, errors.New("blocco Oodle troppo grande")
	}
	muOodle.Lock()
	defer muOodle.Unlock()
	m, err := caricaOodle()
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	reset, alloc, dec := m.ExportedFunction("oz_reset"), m.ExportedFunction("oz_alloc"), m.ExportedFunction("oz_dec")
	if _, err := reset.Call(ctx); err != nil {
		return nil, err
	}
	r, err := alloc.Call(ctx, uint64(len(src)+spazioSicuro))
	if err != nil || r[0] == 0 {
		return nil, errors.New("memoria Oodle esaurita")
	}
	pSrc := uint32(r[0])
	r, err = alloc.Call(ctx, uint64(n+spazioSicuro))
	if err != nil || r[0] == 0 {
		return nil, errors.New("memoria Oodle esaurita")
	}
	pDst := uint32(r[0])
	mem := m.Memory()
	if !mem.Write(pSrc, src) {
		return nil, errors.New("memoria Oodle esaurita")
	}
	r, err = dec.Call(ctx, uint64(pSrc), uint64(len(src)), uint64(pDst), uint64(n))
	if err != nil {
		return nil, errors.New("dati Oodle non validi")
	}
	if int32(uint32(r[0])) != int32(n) {
		return nil, errors.New("dati Oodle non validi")
	}
	out, ok := mem.Read(pDst, uint32(n))
	if !ok {
		return nil, errors.New("dati Oodle non validi")
	}
	return append([]byte(nil), out...), nil
}
