package settezip

import "os/exec"

func lookPath(p string) (string, error) { return exec.LookPath(p) }

func comando(p string, a ...string) ([]byte, error) { return exec.Command(p, a...).CombinedOutput() }
