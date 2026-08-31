package genoa_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/genoa"
	"go.rtnl.ai/genoa/config"
)

func TestLoad(t *testing.T) {
	// Register commands
	genoa.Register(func() genoa.Command { return new(Foo) })
	genoa.Register(func() genoa.Command { return new(Bar) })

	t.Run("JSON", func(t *testing.T) {
		t.Cleanup(func() {
			counts = nil
		})

		genoa, err := genoa.Load("testdata/genoa.json")
		require.NoError(t, err)
		require.NotNil(t, genoa)

		for cmd := range genoa.Iter() {
			require.NoError(t, cmd.Run(context.Background(), config.Config{}))
		}

		require.Equal(t, 2, counts["Foo"])
		require.Equal(t, 1, counts["Bar"])
	})

	t.Run("YAML", func(t *testing.T) {
		t.Cleanup(func() {
			counts = nil
		})

		genoa, err := genoa.Load("testdata/genoa.yaml")
		require.NoError(t, err)
		require.NotNil(t, genoa)

		for cmd := range genoa.Iter() {
			require.NoError(t, cmd.Run(context.Background(), config.Config{}))
		}

		require.Equal(t, 2, counts["Foo"])
		require.Equal(t, 1, counts["Bar"])
	})
}

//============================================================================
// Test commands
//============================================================================

var (
	mu     sync.Mutex
	counts map[string]int
)

type Foo struct {
	Name  string `json:"name" yaml:"name"`
	Color string `json:"color" yaml:"color"`
	Size  int    `json:"size" yaml:"size"`
}

func (f *Foo) Run(ctx context.Context, config config.Config) error {
	mu.Lock()
	defer mu.Unlock()

	if counts == nil {
		counts = make(map[string]int)
	}

	counts[f.Kind()]++
	return nil
}

func (f *Foo) Kind() string {
	return "Foo"
}

type Bar struct {
	Title     string    `json:"title" yaml:"title"`
	Timestamp time.Time `json:"timestamp" yaml:"timestamp"`
}

func (b *Bar) Run(ctx context.Context, config config.Config) error {
	mu.Lock()
	defer mu.Unlock()

	if counts == nil {
		counts = make(map[string]int)
	}

	counts[b.Kind()]++
	return nil
}

func (b *Bar) Kind() string {
	return "Bar"
}
