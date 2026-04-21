package osm

import (
	"encoding/json"
	"fmt"

	"github.com/piwi3910/netweave/internal/backend"
	"github.com/piwi3910/netweave/internal/smo"
)

// init registers the OSM SMO plugin constructor with the backend factory.
func init() {
	backend.RegisterSMOAdapter("osm", func(inst *backend.Instance) (smo.Plugin, error) {
		cfg := DefaultConfig()
		if len(inst.Config) > 0 {
			if err := json.Unmarshal(inst.Config, cfg); err != nil {
				return nil, fmt.Errorf("osm plugin config: %w", err)
			}
		}
		plugin, err := New(cfg)
		if err != nil {
			return nil, err
		}
		return NewSMOPluginAdapter(plugin), nil
	})
}
