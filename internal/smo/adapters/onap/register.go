package onap

import (
	"encoding/json"
	"fmt"

	"github.com/piwi3910/netweave/internal/backend"
	"github.com/piwi3910/netweave/internal/smo"
)

// init registers the ONAP SMO plugin constructor with the backend factory.
func init() {
	backend.RegisterSMOAdapter("onap", func(inst *backend.Instance) (smo.Plugin, error) {
		cfg := DefaultConfig()
		if len(inst.Config) > 0 {
			if err := json.Unmarshal(inst.Config, cfg); err != nil {
				return nil, fmt.Errorf("onap plugin config: %w", err)
			}
		}
		return New(cfg)
	})
}
