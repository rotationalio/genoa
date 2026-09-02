package genoa_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/genoa"
)

func TestCofferKeys_Parse(t *testing.T) {
	t.Run("Valid", func(t *testing.T) {
		tests := []string{
			"",
			":",
			":MC4CAQAwBQYDK2VuBCIEIGzkq/OyawyqFBsQcMD8HYIYM9HNEnpAATO8iWA5krQ5",
			"bar:MC4CAQAwBQYDK2VuBCIEIGzkq/OyawyqFBsQcMD8HYIYM9HNEnpAATO8iWA5krQ5,foo:dGhlZWFnbGVmbGllc2F0bWlkbmlnaHQ=",
		}

		for i, tc := range tests {
			keys := make(genoa.CofferKeys)
			require.NoError(t, keys.Load(tc), "could not parse test case %d", i)
			require.Equal(t, tc, keys.Dump())
		}
	})

}
