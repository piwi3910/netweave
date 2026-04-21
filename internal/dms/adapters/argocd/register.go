package argocd

import (
	"encoding/json"
	"fmt"

	"github.com/piwi3910/netweave/internal/backend"
	dmsadapter "github.com/piwi3910/netweave/internal/dms/adapter"
)

// init registers the ArgoCD DMS adapter constructor with the backend factory.
func init() {
	backend.RegisterDMSAdapter("argocd", func(inst *backend.Instance) (dmsadapter.DMSAdapter, error) {
		var cfg Config
		if len(inst.Config) > 0 {
			if err := json.Unmarshal(inst.Config, &cfg); err != nil {
				return nil, fmt.Errorf("argocd adapter config: %w", err)
			}
		}
		return New(&cfg)
	})
}
