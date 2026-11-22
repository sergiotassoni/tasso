package traduce

import "testing"

func TestQuasiUguale(t *testing.T) {
	si := [][2]string{
		{"Hello there, traveller. Welcome to our village.", "Hel1o there, traveller. Welcome to our village."},
		{"The bridge to the north is guarded day and night.", "The bridge to the north is guarded day and niqht."},
	}
	no := [][2]string{
		{"You have 150 gold coins left in your purse.", "You have 160 gold coins left in your purse."},
		{"I can help you find the lost sword of the king.", "I can't help you find the lost sword of the king."},
		{"Short line", "Shart line"},
		{"Open the door to the left of the big tower.", "Open the door to the right of the big tower."},
		{"You must find him before the sun goes down.", "You must find her before the sun goes down."},
	}
	for _, c := range si {
		if !quasiUguale(c[0], c[1]) {
			t.Errorf("dovevano essere uguali: %q / %q", c[0], c[1])
		}
	}
	for _, c := range no {
		if quasiUguale(c[0], c[1]) {
			t.Errorf("non dovevano essere uguali: %q / %q", c[0], c[1])
		}
	}
}
