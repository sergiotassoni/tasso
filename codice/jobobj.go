package main

import (
	"sync"

	"golang.org/x/sys/windows"
)

// killJob raggruppa un processo e tutti i suoi figli: kill() li chiude tutti insieme.
type killJob struct {
	mu sync.Mutex
	h  windows.Handle
}

func newKillJob(pid int) *killJob {
	j := &killJob{}
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return j
	}
	// Niente KILL_ON_JOB_CLOSE: i programmi aperti apposta (Esplora file, il browser)
	// devono restare aperti. Il gruppo si chiude solo con kill() (tempo scaduto o Ferma).
	ph, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		windows.CloseHandle(h)
		return j
	}
	defer windows.CloseHandle(ph)
	if err := windows.AssignProcessToJobObject(h, ph); err != nil {
		windows.CloseHandle(h)
		return j
	}
	j.h = h
	return j
}

func (j *killJob) kill() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.h != 0 {
		_ = windows.TerminateJobObject(j.h, 1)
	}
}

// close libera l'handle senza chiudere niente.
func (j *killJob) close() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.h != 0 {
		windows.CloseHandle(j.h)
		j.h = 0
	}
}
