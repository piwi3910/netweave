package resolvers

import (
	"context"
	"testing"

	mockadapter "github.com/piwi3910/netweave/internal/adapters/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewResolver(t *testing.T) {
	adp := mockadapter.NewAdapter(false)
	logger := zap.NewNop()

	r := NewResolver(adp, nil, nil, logger)
	require.NotNil(t, r)
	assert.Equal(t, adp, r.adapter)
	assert.Equal(t, logger, r.logger)
}

func TestGetActiveAdapter_FallbackToDefault(t *testing.T) {
	adp := mockadapter.NewAdapter(false)
	logger := zap.NewNop()
	r := NewResolver(adp, nil, nil, logger)

	// Without context adapter, should return the default
	ctx := context.Background()
	result := r.getActiveAdapter(ctx)
	assert.Equal(t, adp, result)
}

func TestGetActiveAdapter_UsesContextAdapter(t *testing.T) {
	defaultAdp := mockadapter.NewAdapter(false)
	contextAdp := mockadapter.NewAdapterWithOCloudID("context-ocloud", false)
	logger := zap.NewNop()
	r := NewResolver(defaultAdp, nil, nil, logger)

	// With context adapter, should return the context one
	ctx := WithAdapter(context.Background(), contextAdp)
	result := r.getActiveAdapter(ctx)
	assert.Equal(t, contextAdp, result)
}

func TestGetActiveAdapter_NilContextValue(t *testing.T) {
	defaultAdp := mockadapter.NewAdapter(false)
	logger := zap.NewNop()
	r := NewResolver(defaultAdp, nil, nil, logger)

	// Explicitly setting nil in context should fall back to default
	ctx := context.WithValue(context.Background(), contextKeyIMSAdapter{}, nil)
	result := r.getActiveAdapter(ctx)
	assert.Equal(t, defaultAdp, result)
}

func TestWithAdapter(t *testing.T) {
	adp := mockadapter.NewAdapter(false)
	ctx := context.Background()

	newCtx := WithAdapter(ctx, adp)
	require.NotNil(t, newCtx)

	// Verify the adapter is retrievable from context
	val := newCtx.Value(contextKeyIMSAdapter{})
	require.NotNil(t, val)
	assert.Equal(t, adp, val)
}
