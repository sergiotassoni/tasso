package traduce

import "testing"

func TestLetturaAffidabile(t *testing.T) {
	for s, v := range map[string]bool{
		"The current season has been reset. From now on, a new season will begin on the first day of":     true,
		"Any player's Steam profile can now be opened from the pause menu.":                               true,
		"altri playgs thrling the paase mnu Repmts are sent drectty a":                                    false,
		"During ranked matches, players can now like, dislike, or report other players through the pause": true,
		"ENTER THE BAR": true,
	} {
		if letturaAffidabile(s) != v {
			t.Errorf("%q: atteso %v", s, v)
		}
	}
}

func TestSeparaAttaccate(t *testing.T) {
	for in, out := range map[string]string{
		"displays thetime remaining": "displays the time remaining",
		"Any player's Steam profile": "Any player's Steam profile",
		"LEGENDS leaderboard.":       "LEGENDS leader board.",
		"players fromeach completed": "players from each completed",
	} {
		if g := separaAttaccate(in); g != out {
			t.Errorf("%q -> %q, atteso %q", in, g, out)
		}
	}
	if !rigaAndataACapo("a new season will begin on the first day of", 470, Blocco{W: 60}) || rigaAndataACapo("Monthly Seasons", 80, Blocco{W: 470}) {
		t.Error("a capo")
	}
}
