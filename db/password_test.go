package db

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPassword(t *testing.T) {
	runs := 1024
	if testing.Short() {
		runs = 16
	}

	for i := 0; i < runs; i++ {
		password := Password()
		require.True(t, IsStrongPassword(password), "%q failed strength check", password)
	}
}

func TestIsStrongPassword(t *testing.T) {
	tests := []struct {
		password string
		assert   require.BoolAssertionFunc
	}{
		{password: "password", assert: require.False},                         // too simple
		{password: "Password123!", assert: require.False},                     // too short
		{password: "NDiAfh5497QjA2B65KrZNqvVOaOBg8Uv", assert: require.False}, // no specials
		{password: "NDiAfh-F~aQjA_B.HKrZNqvVOaOBg~Uv", assert: require.False}, // no digits
		{password: "NDiAfh5497~jA2B65KrZNqvVOaOBg8Uv", assert: require.True},
		{password: "sNe.0bPQ8s7nxdt9nfHPthyVLafZKaav", assert: require.True},
		{password: "YH0rDgDfYjXZdVy50FZkXq6l-iuiPgao", assert: require.True},
		{password: "HHHhWkqei3jsMFlFy_YPhpUa6sWTud5K", assert: require.True},
	}

	for _, test := range tests {
		test.assert(t, IsStrongPassword(test.password), "%q failed strength check", test.password)
	}
}
