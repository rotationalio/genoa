package genoa_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/genoa"
)

func TestSecretResource_IsZero(t *testing.T) {
	tests := []struct {
		secret *genoa.SecretResource
		assert require.BoolAssertionFunc
	}{
		{
			nil, require.True,
		},
		{
			&genoa.SecretResource{}, require.True,
		},
		{
			&genoa.SecretResource{SecretName: "test"}, require.False,
		},
		{
			&genoa.SecretResource{SecretKey: "test"}, require.False,
		},
		{
			&genoa.SecretResource{Value: "test"}, require.False,
		},
	}

	for i, test := range tests {
		test.assert(t, test.secret.IsZero(), "unexpected result for test case %d", i)
	}
}

func TestSecretResource_Resolve(t *testing.T) {
}
