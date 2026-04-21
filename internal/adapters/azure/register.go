package azure

import (
	"encoding/json"
	"fmt"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/backend"
)

// init registers the Azure IMS adapter constructor with the backend factory.
// The constructor JSON-decodes backend.Instance.Config into azure.Config and
// falls back to the instance ID for the OCloudID when unspecified.
func init() {
	backend.RegisterIMSAdapter("azure", func(inst *backend.Instance) (adapter.Adapter, error) {
		var cfg Config
		if len(inst.Config) > 0 {
			if err := json.Unmarshal(inst.Config, &cfg); err != nil {
				return nil, fmt.Errorf("azure adapter config: %w", err)
			}
		}
		if cfg.OCloudID == "" {
			cfg.OCloudID = inst.ID
		}
		return New(&cfg)
	})
}
