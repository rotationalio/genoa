package genoa_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/genoa"
	"go.rtnl.ai/ulid"
)

func TestJWKSKeyRegex(t *testing.T) {
	tests := []struct {
		input  string
		assert require.BoolAssertionFunc
	}{
		{
			input:  "01KTCVE4BRVQQCR7R3M2RYVHJE.pem",
			assert: require.True,
		},
		{
			input:  "01KTCVE4BRVQQCR7R3M2RYVHJE",
			assert: require.False,
		},
		{
			input:  "01M1EH6WGEFH803NAA9MZQ7NAP.pem",
			assert: require.True,
		},
		{
			input:  "01M1EH6WGEFH803NAA9MZQ7NAP.json",
			assert: require.False,
		},
		{
			input:  "5537323f-9dc7-4bda-801e-a70b134d505a.pem",
			assert: require.False,
		},
	}

	for i, tc := range tests {
		tc.assert(t, genoa.JWKSKeyRegex.MatchString(tc.input), "test %d failed", i)
	}
}

func TestJWKSKeysSort(t *testing.T) {
	keys := genoa.JWKSKeys{
		{Key: "01KTCVE4BRVQQCR7R3M2RYVHJE.pem"},
		{Key: "01M1EKKVH35SB6RVJQ7VK68HDM.pem"},
		{Key: "01M1EH6WGEFH803NAA9MZQ7NAP.pem"},
	}
	keys.Sort()
	require.Equal(t, keys[0].Key, "01M1EKKVH35SB6RVJQ7VK68HDM.pem")
	require.Equal(t, keys[1].Key, "01M1EH6WGEFH803NAA9MZQ7NAP.pem")
	require.Equal(t, keys[2].Key, "01KTCVE4BRVQQCR7R3M2RYVHJE.pem")

	var prev time.Time
	for i, key := range keys {
		ts := ulid.MustParse(key.Key[:26]).Timestamp()
		if i == 0 {
			prev = ts
			continue
		}

		require.True(t, ts.Before(prev), "key %d is not before key %d", i, i-1)
		prev = ts
	}
}

func TestJWKSKeysString(t *testing.T) {
	keys := genoa.JWKSKeys{
		{Key: "01KTCVE4BRVQQCR7R3M2RYVHJE.pem"},
		{Key: "01M1EKKVH35SB6RVJQ7VK68HDM.pem"},
	}
	require.Equal(
		t, keys.String("/data/keys"),
		"01KTCVE4BRVQQCR7R3M2RYVHJE:/data/keys/01KTCVE4BRVQQCR7R3M2RYVHJE.pem,01M1EKKVH35SB6RVJQ7VK68HDM:/data/keys/01M1EKKVH35SB6RVJQ7VK68HDM.pem",
	)
}
