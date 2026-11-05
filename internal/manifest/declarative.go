package manifest

import "github.com/vicgupta/dap/internal/declarative"

// projectDeclarative turns a checked declarative document into the manifest
// the installer, scheduler, and guardrails already understand.
// Permissions come from the services and actions. The YAML does not list them.
func projectDeclarative(p declarative.Plugin, dir string) (Manifest, error) {
	sched := p.EffectiveSchedule()
	m := Manifest{
		Name:          p.Name,
		Version:       p.Version,
		MinDAPVersion: p.MinDAPVersion,
		Type:          p.Type,
		Interface:     "dap.plugin.v1",
		Description:   p.Description,
		Runtime:       "declarative",
		Schedule: Schedule{
			Every:      sched.Every,
			Cron:       sched.Cron,
			At:         sched.At,
			Recurrence: sched.Recurrence,
			Day:        sched.Day,
		},
		Declarative: &p,
	}
	for _, perm := range p.Permissions() {
		m.Permissions = append(m.Permissions, Permission{
			Name:        perm.Name,
			Mode:        perm.Mode,
			Description: perm.Description,
		})
	}
	for _, s := range p.Settings {
		desc := s.Description
		if desc == "" {
			desc = s.Label
		}
		m.Config = append(m.Config, ConfigField{
			Key:         s.Key,
			Type:        s.Type,
			Description: desc,
			Required:    s.Required,
			Default:     s.Default,
		})
	}
	if p.Tile != nil {
		m.Tile = &TileSpec{Size: p.Tile.Size, Min: p.Tile.Min}
	}
	if err := m.Validate(dir); err != nil {
		return Manifest{}, err
	}
	return m, nil
}
