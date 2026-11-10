package cloudbackup

import (
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/testutil/restorefake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRestore_PICWhileDualFlight(t *testing.T) {
	store := restorefake.New()
	p := samplePayload()
	f := p.Flights[0]
	f.TotalTime = 96
	f.IsPIC = true
	f.IsDual = true
	f.PICTime = 96
	f.DualTime = 96

	res, err := restoreSample(t, store, p, "")
	require.NoError(t, err)
	assert.Empty(t, res.Warnings)

	got := store.State.Flights[0]
	assert.True(t, got.IsPIC)
	assert.True(t, got.IsDual)
	assert.Equal(t, 96, got.PICTime)
	assert.Equal(t, 96, got.DualTime)
}
