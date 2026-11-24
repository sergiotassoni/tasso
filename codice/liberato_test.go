package main

import "testing"

func TestSpazioOnesto(t *testing.T) {
	gb := int64(1 << 30)
	mb := int64(1 << 20)
	prima := map[string]uint64{`C:\`: uint64(10 * gb), `D:\`: uint64(50 * gb)}
	dopo := map[string]uint64{`C:\`: uint64(10*gb + 1536*mb), `D:\`: uint64(50 * gb)}
	if d := diffSpazioLibero(prima, dopo); d != 1536*mb {
		t.Fatalf("diff %d", d)
	}
	if v := spazioOnesto(409*mb, 1536*mb); v != 1536*mb {
		t.Fatalf("deve vincere il disco: %d", v)
	}
	if v := spazioOnesto(409*mb, -2*gb); v != 409*mb {
		t.Fatalf("mai negativo, resta la somma: %d", v)
	}
	if v := spazioOnesto(0, -5); v != 0 {
		t.Fatalf("zero: %d", v)
	}
	if d := diffSpazioLibero(prima, map[string]uint64{`C:\`: uint64(11 * gb)}); d != gb {
		t.Fatalf("disco sparito ignorato: %d", d)
	}
}
