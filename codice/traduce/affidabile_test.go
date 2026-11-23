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
