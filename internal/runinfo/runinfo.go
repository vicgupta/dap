// Package runinfo records how to reach a running `da serve` process.
package runinfo

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Info is written to run.json while the server is up. Mode 0600.
type Info struct {
	PID     int       `json:"pid"`
	Addr    string    `json:"addr"`
	Token   string    `json:"token"`
	Started time.Time `json:"started"`
}

// Path is the run file inside the data directory.
func Path(dir string) string { return filepath.Join(dir, "run.json") }

// Write stores the current server coordinates.
func Write(dir string, info Info) error {
	b, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(dir), append(b, '\n'), 0o600)
}

// Read loads run.json.
func Read(dir string) (Info, error) {
	b, err := os.ReadFile(Path(dir))
	if err != nil {
		return Info{}, err
	}
	var info Info
	if err := json.Unmarshal(b, &info); err != nil {
		return Info{}, err
	}
	if info.Addr == "" || info.Token == "" || info.PID == 0 {
		return Info{}, errors.New("run file is incomplete")
	}
	return info, nil
}

// Remove deletes the run file.
func Remove(dir string) error {
	err := os.Remove(Path(dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
