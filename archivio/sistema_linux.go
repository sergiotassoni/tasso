// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

//go:build linux

package archivio

import (
	"bufio"
	"errors"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// rinominaSeLibero sposta da -> a solo se "a" non esiste ancora (mai sovrascrivere).
// Serve per i test su Linux: usa RENAME_NOREPLACE quando c'è.
func rinominaSeLibero(da, a string) error {
	err := unix.Renameat2(unix.AT_FDCWD, da, unix.AT_FDCWD, a, unix.RENAME_NOREPLACE)
	if err == nil {
		return nil
	}
	if errors.Is(err, unix.EEXIST) {
		return errEsisteGia
	}
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOTSUP) {
		if _, e := os.Lstat(a); e == nil {
			return errEsisteGia
		}
		return os.Rename(da, a)
	}
	return err
}

func spazioLibero(cartella string) int64 {
	var st unix.Statfs_t
	if err := unix.Statfs(cartella, &st); err != nil {
		return -1
	}
	return int64(st.Bavail) * int64(st.Bsize)
}

func codePageOEM() int { return 437 }

// memoriaLibera: byte di RAM disponibili (0 se non si sa).
func memoriaLibera() int64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		c := strings.Fields(sc.Text())
		if len(c) >= 2 && c[0] == "MemAvailable:" {
			n, _ := strconv.ParseInt(c[1], 10, 64)
			return n << 10
		}
	}
	return 0
}

func leggiZonaSistema(string) []byte   { return nil }
func scriviZonaSistema(string, []byte) {}
func nascondi(string)                  {}
func mostra(string)                    {}
