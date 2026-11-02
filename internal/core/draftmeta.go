package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/vicgupta/dap/internal/manifest"
	"github.com/vicgupta/dap/internal/pluginauthor"
)

const (
	unchangedDraftMessage = "The coding agent finished without changing the starter plugin, so DAP did not propose an install."
	markerDraftMessage    = "The starter plugin is still in the draft. The agent did not replace it, so DAP did not propose an install."
	unnamedDraftMessage   = "The agent did not choose a short plugin name, so nothing was proposed."
)

type draftMeta struct {
	Name        string            `json:"name"`
	Provisional bool              `json:"provisional"`
	Files       map[string]string `json:"files"`
}

func (a *App) draftMetaPath(name string) string {
	return filepath.Join(a.DataDir, "drafts", ".meta", name+".json")
}

func (a *App) writeDraftMeta(meta draftMeta) error {
	dir := filepath.Join(a.DataDir, "drafts", ".meta")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(a.draftMetaPath(meta.Name), append(raw, '\n'), 0o644)
}

func (a *App) readDraftMeta(name string) (*draftMeta, error) {
	raw, err := os.ReadFile(a.draftMetaPath(name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var meta draftMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func (a *App) isDraft(abs string) bool {
	drafts := filepath.Clean(filepath.Join(a.DataDir, "drafts"))
	abs = filepath.Clean(abs)
	if abs == drafts || !strings.HasPrefix(abs, drafts+string(filepath.Separator)) {
		return false
	}
	rel, err := filepath.Rel(drafts, abs)
	if err != nil {
		return false
	}
	return rel != ".meta" && !strings.HasPrefix(rel, ".meta"+string(filepath.Separator))
}

// draftChanged reports whether the agent replaced the starter. A missing
// snapshot (an installed plugin, not a draft) counts as changed.
func (a *App) draftChanged(dir string) (bool, string) {
	if pluginauthor.MarkerPresent(dir) {
		return false, markerDraftMessage
	}
	meta, err := a.readDraftMeta(filepath.Base(dir))
	if err != nil {
		return false, err.Error()
	}
	if meta == nil {
		return true, ""
	}
	same, err := sameFiles(dir, meta.Files)
	if err != nil {
		return false, err.Error()
	}
	if same {
		return false, unchangedDraftMessage
	}
	return true, ""
}

// relocateDraft renames a draft folder to the name in plugin.yaml before a card is proposed.
func (a *App) relocateDraft(dir string) (string, error) {
	m, err := manifest.Load(dir)
	if err != nil {
		return "", err
	}
	if !pluginNameOK.MatchString(m.Name) {
		return "", errors.New("plugin name must be a short kebab-case name")
	}
	base := filepath.Base(dir)
	meta, err := a.readDraftMeta(base)
	if err != nil {
		return "", err
	}
	if meta != nil && meta.Provisional && strings.HasPrefix(m.Name, "draft-") {
		return "", errors.New(unnamedDraftMessage)
	}
	if m.Name == base {
		return dir, nil
	}
	next := filepath.Join(filepath.Dir(dir), m.Name)
	if _, err := os.Stat(next); err == nil {
		return "", errors.New(m.Name + " already exists")
	}
	if err := os.Rename(dir, next); err != nil {
		return "", err
	}
	if meta != nil {
		_ = os.Remove(a.draftMetaPath(base))
		meta.Name = m.Name
		meta.Provisional = false
		if err := a.writeDraftMeta(*meta); err != nil {
			return "", err
		}
	}
	return next, nil
}

func hashTree(dir string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		out[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	return out, err
}

func sameFiles(dir string, prev map[string]string) (bool, error) {
	cur, err := hashTree(dir)
	if err != nil {
		return false, err
	}
	if len(cur) != len(prev) {
		return false, nil
	}
	for k, v := range prev {
		if cur[k] != v {
			return false, nil
		}
	}
	return true, nil
}
