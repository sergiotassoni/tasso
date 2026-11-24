package main

// 3.1 "Non chiudere mai anche": scegli tu quali programmi il Boost lascia sempre aperti.

import (
	"sort"
	"strings"
)

// boostKeepSet: i programmi da non chiudere mai, scelti in Giochi > Impostazioni.
func boostKeepSet() map[string]bool {
	setMu.Lock()
	defer setMu.Unlock()
	m := map[string]bool{}
	for _, k := range settings.BoostKeep {
		m[strings.ToLower(k)] = true
	}
	return m
}

// boostKeepInfo: i programmi che il Boost può chiudere e quelli che hai deciso di tenere aperti.
func boostKeepInfo() map[string]interface{} {
	var all []string
	for exe := range boostClosable {
		all = append(all, exe)
	}
	sort.Strings(all)
	setMu.Lock()
	keep := append([]string{}, settings.BoostKeep...)
	setMu.Unlock()
	return map[string]interface{}{"closable": all, "keep": keep}
}

// setBoostKeep salva la scelta: accetta solo i programmi che il Boost conosce.
func setBoostKeep(list []string) string {
	seen := map[string]bool{}
	var out []string
	for _, k := range list {
		k = strings.ToLower(strings.TrimSpace(k))
		if !strings.HasSuffix(k, ".exe") {
			k += ".exe"
		}
		if boostClosable[k] && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	sort.Strings(out)
	updateSettings(func(s *Settings) { s.BoostKeep = out })
	return "ok"
}
