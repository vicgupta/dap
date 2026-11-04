package pluginauthor

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/vicgupta/dap/internal/manifest"
	"github.com/vicgupta/dap/internal/pluginhost"
)

// Dev runs one plugin, reloads it when a file in the directory changes, and
// writes one JSON object per line to w.
func Dev(ctx context.Context, dir, dataDir string, w io.Writer) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	emit := func(event string, fields map[string]any) {
		if fields == nil {
			fields = map[string]any{}
		}
		fields["event"] = event
		fields["ts"] = time.Now().UTC().Format(time.RFC3339)
		_ = enc.Encode(fields)
	}
	host := pluginhost.New()
	defer host.StopAll()
	var stamp time.Time
	start := func() error {
		m, err := manifest.Load(abs)
		if err != nil {
			emit("error", map[string]any{"detail": err.Error()})
			return err
		}
		argv, err := m.Entrypoint.Command(abs)
		if err != nil {
			emit("error", map[string]any{"detail": err.Error()})
			return err
		}
		host.Stop(m.Name)
		if err := host.Start(pluginhost.Options{Name: m.Name, Dir: abs, Argv: argv, Env: pluginhost.PluginEnvIn(dataDir, "DAP_PLUGIN="+m.Name)}); err != nil {
			emit("error", map[string]any{"detail": err.Error()})
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		var init pluginhost.InitResult
		if err := host.Call(ctx, m.Name, "initialize", pluginhost.InitParams{Interface: "dap.plugin.v1", DAPVersion: "0.1.0", PluginDir: abs, DataDir: abs, Config: map[string]any{}}, &init); err != nil {
			emit("error", map[string]any{"detail": err.Error(), "stderr": host.Stderr(m.Name)})
			return err
		}
		emit("ready", map[string]any{"name": m.Name, "health": init.Health.Message})
		stamp = newest(abs)
		return nil
	}
	if err := start(); err != nil {
		return err
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	var lastErr string
	for {
		select {
		case <-ctx.Done():
			emit("stop", nil)
			return nil
		case <-tick.C:
			m, err := manifest.Load(abs)
			if err != nil {
				continue
			}
			tail := host.Stderr(m.Name)
			if tail != "" && tail != lastErr {
				emit("stderr", map[string]any{"text": tail})
				lastErr = tail
			}
			if next := newest(abs); next.After(stamp) {
				emit("reload", map[string]any{"name": m.Name})
				_ = start()
			}
		}
	}
}

func newest(dir string) time.Time {
	var max time.Time
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if info.ModTime().After(max) {
			max = info.ModTime()
		}
		return nil
	})
	return max
}
