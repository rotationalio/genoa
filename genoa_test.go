package genoa_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/genoa"
)

func TestLoad(t *testing.T) {
	// Register commands

	t.Run("JSON", func(t *testing.T) {
		genoa, err := genoa.Load("testdata/genoa.json")
		require.NoError(t, err)
		require.NotNil(t, genoa)
	})

	t.Run("YAML", func(t *testing.T) {
		genoa, err := genoa.Load("testdata/genoa.yaml")
		require.NoError(t, err)
		require.NotNil(t, genoa)
	})
}
