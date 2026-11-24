# Decompressore Oodle (ooz) per Tasso

`traduce/unreal/ooz.wasm` è il decompressore **ooz** (Kraken/Mermaid/Selkie/Leviathan, LZNA, BitKnit)
di Powzix, licenza **GNU GPL v3 o successiva** (come Tasso), compilato in WebAssembly.
Sorgente presa dal pacchetto pyooz 0.0.8 (PyPI), cartella `ooz/dep/ooz`.
Serve a leggere i file di lingua (.locres) dentro i .pak dei giochi Unreal compressi con Oodle.
Solo decompressione: Tasso non comprime mai con Oodle.

Ricompilare (clang 18 + wasm-ld):

    cd ooz
    n=$(grep -n "^// The decompressor will write outside" kraken.cpp | cut -d: -f1)
    head -n $((n-1)) kraken.cpp > kraken_dec.cpp
    for f in kraken_dec lzna bitknit; do clang++ --target=wasm32 -O2 -nostdlib -fno-exceptions -fno-rtti -I../inc -I. -Isimde -DSIMDE_NO_NATIVE -msimd128 -c $f.cpp -o ../$f.o; done
    cd .. && clang --target=wasm32 -O2 -nostdlib -msimd128 -c rt.c -o rt.o
    wasm-ld --no-entry --export=oz_reset --export=oz_alloc --export=oz_dec -z stack-size=1048576 --initial-memory=4194304 -o ooz.wasm kraken_dec.o lzna.o bitknit.o rt.o

`inc/` contiene intestazioni minime al posto della libreria C (niente file, niente stampa);
`rt.c` dà memoria, memcpy e le tre funzioni usate da Tasso (oz_reset, oz_alloc, oz_dec).
simde (MIT) è incluso in `ooz/simde`.
