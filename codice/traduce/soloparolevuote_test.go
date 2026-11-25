package traduce

import "testing"

func TestSoloParoleVuote(t *testing.T) {
	for s, atteso := range map[string]bool{
		"the": true, "to a": true, "and,": true, "The": true,
		"Back": false, "Resume": false, "the end": false, "Press E to interact": false, "": false,
	} {
		if soloParoleVuote(s) != atteso {
			t.Errorf("soloParoleVuote(%q) = %v, atteso %v", s, !atteso, atteso)
		}
	}
}
