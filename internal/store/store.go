// Package store persists plugins, cards, guardrail overrides, and per-plugin scratch state.
// The activity ledger is separate and append-only.
package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vicgupta/dap/internal/guardrails"
	"github.com/vicgupta/dap/internal/manifest"
	"github.com/vicgupta/dap/internal/model"
	"github.com/vicgupta/dap/internal/scheduler"
)

// State is the whole mutable document.
type State struct {
	Plugins     map[string]*PluginRecord   `json:"plugins"`
	Cards       []model.Card               `json:"cards"`
	Rules       []guardrails.Rule          `json:"rules"`
	PluginState map[string]json.RawMessage `json:"plugin_state,omitempty"`
}

// PluginRecord is one installed plugin and its schedule cursor.
type PluginRecord struct {
	Name        string                `json:"name"`
	Dir         string                `json:"dir"`
	Config      map[string]any        `json:"config,omitempty"`
	Schedule    manifest.Schedule     `json:"schedule"`
	Run         scheduler.State       `json:"run"`
	Tile        model.Tile            `json:"tile"`
	Metrics     []model.Metric        `json:"metrics,omitempty"`
	InstalledAt time.Time             `json:"installed_at"`
	Manifest    manifest.Manifest     `json:"manifest"`
	Approved    []manifest.Permission `json:"approved_permissions"`
}

// Store is a mutex-guarded JSON file.
type Store struct {
	path  string
	mu    sync.Mutex
	state State
}

// Open loads state.json, creating an empty document if needed.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{
		path: filepath.Join(dir, "state.json"),
		state: State{
			Plugins:     map[string]*PluginRecord{},
			PluginState: map[string]json.RawMessage{},
		},
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := s.saveLocked(); err != nil {
				return nil, err
			}
			return s, nil
		}
		return nil, err
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &s.state); err != nil {
			return nil, err
		}
	}
	if s.state.Plugins == nil {
		s.state.Plugins = map[string]*PluginRecord{}
	}
	if s.state.PluginState == nil {
		s.state.PluginState = map[string]json.RawMessage{}
	}
	return s, nil
}

// Update mutates a copy and, on success, swaps it in and saves.
// A failing fn does not change memory or disk. fn must not keep pointers.
func (s *Store) Update(fn func(*State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.copyLocked()
	if err := fn(&next); err != nil {
		return err
	}
	s.state = next
	return s.saveLocked()
}

// View runs fn against a deep copy.
func (s *Store) View(fn func(State)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(s.copyLocked())
}

// Snapshot returns a deep copy.
func (s *Store) Snapshot() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.copyLocked()
}

func (s *Store) copyLocked() State {
	b, err := json.Marshal(s.state)
	if err != nil {
		return State{Plugins: map[string]*PluginRecord{}}
	}
	var out State
	if err := json.Unmarshal(b, &out); err != nil {
		return State{Plugins: map[string]*PluginRecord{}}
	}
	if out.Plugins == nil {
		out.Plugins = map[string]*PluginRecord{}
	}
	if out.PluginState == nil {
		out.PluginState = map[string]json.RawMessage{}
	}
	return out
}

func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
