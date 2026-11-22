package main

// 3.1 pannellino a scelta: cosa mostrare e in quale dei 4 angoli.

var ovAllItems = []string{"cpu", "cputemp", "gpu", "gputemp", "ram", "ora"}

func ovLeft(pos string) bool   { return pos == "sx" || pos == "sxb" }
func ovBottom(pos string) bool { return pos == "sxb" || pos == "dxb" }

// ovItems: le voci scelte; se non hai mai scelto, tutte.
func ovItems() map[string]bool {
	setMu.Lock()
	list := settings.OverlayItems
	setMu.Unlock()
	m := map[string]bool{}
	if len(list) == 0 {
		for _, k := range ovAllItems {
			m[k] = true
		}
		return m
	}
	for _, k := range list {
		m[k] = true
	}
	return m
}

// setOverlayItems salva cosa mostrare (solo voci conosciute) e ridisegna il pannellino.
func setOverlayItems(items []string) string {
	ok := map[string]bool{}
	for _, k := range ovAllItems {
		ok[k] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, k := range items {
		if ok[k] && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	if len(out) == 0 {
		return "Scegli almeno una cosa da mostrare."
	}
	updateSettings(func(s *Settings) { s.OverlayItems = out })
	ovPost()
	return "ok"
}
