package genoa_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/genoa"
	"go.rtnl.ai/x/semver"
)

func TestVersionCheck(t *testing.T) {
	t.Run("Compatible", func(t *testing.T) {
		// Add compatible versions here as necessary.
		tests := []string{
			"1.0.0",
		}

		for i, tc := range tests {
			vers, err := semver.Parse(tc)
			require.NoError(t, err, "could not parse version %q in test case %d", tc, i)
			require.NoError(t, genoa.VersionCheck(vers), "expected version %q in test case %d to be compatible with %s", tc, i, genoa.Version(true))
		}
	})

	t.Run("Incompatible", func(t *testing.T) {
		// Add/remove incompatible versions here as necessary.
		tests := []string{
			"0.37.1",
			"2.0.0",
			"1.52.0",
			"1.1.19",
		}

		for i, tc := range tests {
			vers, err := semver.Parse(tc)
			require.NoError(t, err, "could not parse version %q in test case %d", tc, i)
			require.Error(t, genoa.VersionCheck(vers), "expected version %q in test case %d to be incompatible with %s", tc, i, genoa.Version(true))
		}
	})
}
