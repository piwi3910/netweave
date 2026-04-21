package tenant_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/tenant"
)

func TestFromContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ctx     context.Context //nolint:containedctx // test table
		wantID  string
		wantErr error
	}{
		{
			name:    "bare context returns ErrMissingTenant",
			ctx:     context.Background(),
			wantErr: tenant.ErrMissingTenant,
		},
		{
			name:    "empty tenant id returns ErrMissingTenant",
			ctx:     tenant.WithID(context.Background(), ""),
			wantErr: tenant.ErrMissingTenant,
		},
		{
			name:   "WithID carries the tenant id",
			ctx:    tenant.WithID(context.Background(), "tenant-1"),
			wantID: "tenant-1",
		},
		{
			name:   "WithAll carries the bypass sentinel",
			ctx:    tenant.WithAll(context.Background()),
			wantID: tenant.TenantAll,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := tenant.FromContext(tt.ctx)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.wantErr))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, got)
		})
	}
}

func TestIsAll(t *testing.T) {
	t.Parallel()
	assert.True(t, tenant.IsAll(tenant.TenantAll))
	assert.False(t, tenant.IsAll(""))
	assert.False(t, tenant.IsAll("tenant-1"))
}
