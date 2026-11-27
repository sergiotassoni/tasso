// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

//go:build windows

package main

// HKCU\Software\Tasso\Nome: Tasso PDF gira come l'utente collegato, quindi HKCU è quello giusto.

import "golang.org/x/sys/windows/registry"

type archivioNomeWin struct{}

func (archivioNomeWin) Leggi() (string, bool) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Tasso`, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer k.Close()
	v, tipo, err := k.GetStringValue("Nome")
	if err != nil || tipo != registry.SZ {
		return "", false
	}
	return v, true
}

func (archivioNomeWin) Scrivi(n string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Tasso`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue("Nome", n)
}

func (archivioNomeWin) Togli() error { return nil } // lo toglie "Togli Tasso dal PC" o il disinstallatore

func init() { archivioNom = archivioNomeWin{} }
