package traduce

import "testing"

func TestCorreggiOCRInglese(t *testing.T) {
	casi := map[string]string{
		"If you're reading this, itis already too late for me.":   "If you're reading this, it's already too late for me.",
		"Captain, welre being hailed by an unknown vessel.":       "Captain, we're being hailed by an unknown vessel.",
		"Don't sweat it. Ilve got your back.":                     "Don't sweat it. I've got your back.",
		"Don't sweat it. l've got your back.":                     "Don't sweat it. I've got your back.",
		"lim not Iost. lim exploring with confidence.":            "I'm not lost. I'm exploring with confidence.",
		"Ilm not Iost. Ilm exploring with confidence.":            "I'm not lost. I'm exploring with confidence.",
		"l'm not last. l'm exploring with confidence.":            "I'm not last. I'm exploring with confidence.",
		"Watch your step. These ruins are Older than the kingdom": "Watch your step. These ruins are older than the kingdom",
		"Waiting for Other players...":                            "Waiting for other players...",
		"The king hasn•t left the castle in three Years.":         "The king hasn't left the castle in three years.",
		"Combine items in Your inventory to craft new gear.":      "Combine items in your inventory to craft new gear.",
		"Did you hear that? Something•s moving upstairs.":         "Did you hear that? Something's moving upstairs.",
		// da NON toccare
		"I am the King of the North.": "I am the King of the North.",
		"Talk to Victor in Oslo.":     "Talk to Victor in Oslo.",
		"I will kill him.":            "I will kill him.",
		"Ill tidings, my lord.":       "Ill tidings, my lord.",
	}
	for in, atteso := range casi {
		if got := correggiOCRInglese(in); got != atteso {
			t.Errorf("%q\n  -> %q\n  atteso %q", in, got, atteso)
		}
	}
}

func TestCorrezioniNuove(t *testing.T) {
	for in, atteso := range map[string]string{
		"l'Il be there.":       "I'll be there.",
		"Weill see.":           "We'll see.",
		"live seen it before.": "I've seen it before.",
		"lt's late.":           "It's late.",
		"Then l ran away.":     "Then I ran away.",
		"I live in the city.":  "I live in the city.",
		"It is late.":          "It is late.",
	} {
		if got := pulisci(in); got != atteso {
			t.Errorf("%q -> %q, atteso %q", in, got, atteso)
		}
	}
}

func TestFrasiFisse(t *testing.T) {
	for in, atteso := range map[string]string{
		"Da not go there.": "Do not go there.",
		"It's tao late.":   "It's too late.",
		"Wait far me.":     "Wait for me.",
	} {
		if got := pulisci(in); got != atteso {
			t.Errorf("%q -> %q, atteso %q", in, got, atteso)
		}
	}
}
