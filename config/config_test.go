package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/confire/contest"
	"go.rtnl.ai/genoa/config"
	"go.rtnl.ai/x/rlog"
)

// The values in the test environment variables should result a configuration that is
// equal to the validConfig variable when processed. Anytime a configuration value is
// added to this package, it should be added to this map and have a non-default value
// for testing and validation purposes.
var testEnv = contest.Env{
	"GENOA_LOG_LEVEL":                      "debug",
	"GENOA_CONSOLE_LOG":                    "true",
	"GENOA_LOCAL_ACCESS":                   "true",
	"GENOA_NAMESPACE":                      "genoa",
	"GENOA_DATABASE_URL":                   "postgres://postgres:postgres@localhost:5432/endeavor?sslmode=disable",
	"ENDEAVOR_ENSURE_DATABASE_ENABLED":     "true",
	"ENDEAVOR_ENSURE_DATABASE_NAME":        "rotational",
	"ENDEAVOR_ENSURE_DATABASE_SECRET_NAME": "rotational-endeavor-database-credentials",
}

// This config should always pass validation and should match the testEnv.
// For a minimal valid config for tests, use [conftest.Config] or [conftest.Unmarked].
var validConfig = config.Config{
	LogLevel:    rlog.LevelDecoder(rlog.LevelDebug),
	ConsoleLog:  true,
	LocalAccess: true,
	Namespace:   "genoa",
	DatabaseURL: "postgres://postgres:postgres@localhost:5432/endeavor?sslmode=disable",
	EnsureDatabase: config.EnsureDatabase{
		Enabled:    true,
		Name:       "rotational",
		SecretName: "rotational-endeavor-database-credentials",
	},
}

func TestConfig(t *testing.T) {
	t.Run("Valid", func(t *testing.T) {
		t.Cleanup(testEnv.Set())

		conf, err := config.New()
		require.NoError(t, err, "could not process config from environment")
		require.Equal(t, validConfig, *conf, "valid config should be equal to the expected valid config")
	})
}
