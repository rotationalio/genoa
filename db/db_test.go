package db

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQuoteEscape(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"test"`, `'test'`},
		{`"test's"`, `'test''s'`},
		{`'test'`, `'''test'''`},
		{`test`, `'test'`},
	}

	for i, test := range tests {
		require.Equal(t, test.expected, quoteEscape(test.input), "test %d failed", i)
	}
}
