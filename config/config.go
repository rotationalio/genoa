package config

import (
	"log/slog"
	"sync"

	"go.rtnl.ai/confire"
	"go.rtnl.ai/x/rlog"
)

type Config struct {
	LogLevel       rlog.LevelDecoder `split_words:"true" default:"info" desc:"specify the verbosity of logging (trace, debug, info, warn, error, fatal, or panic)"`
	ConsoleLog     bool              `split_words:"true" default:"false" desc:"if true logs human readable text output instead of json"`
	LocalAccess    bool              `split_words:"true" default:"false" desc:"set to true to allow local access to the cluster without a service account"`
	Namespace      string            `split_words:"true" default:"endeavor" desc:"the namespace to use for the genoa operation"`
	DatabaseURL    string            `split_words:"true" default:"" desc:"the admin database dsn to use to manage roles and databases"`
	EnsureDatabase EnsureDatabase    `split_words:"true"`
}

type EnsureDatabase struct {
	Enabled    bool   `env:"ENDEAVOR_ENSURE_DATABASE_ENABLED" default:"false" desc:"set to true to run the ensure database operation"`
	Name       string `env:"ENDEAVOR_ENSURE_DATABASE_NAME" desc:"the name of the endeavor application to ensure the database for"`
	SecretName string `env:"ENDEAVOR_ENSURE_DATABASE_SECRET_NAME" desc:"the name of the secret to use to store the database credentials"`
}

const Prefix = "genoa"

// New creates a new Config instance and loads the configuration from the environment,
// validating the configuration and returning an error if the configuration is invalid
// or could not be parsed from environment variables.
//
// NOTE: New should only be used for testing, for module access to the config use Get().
func New() (conf *Config, err error) {
	// NOTE: confire.Process calls Validate() internally.
	conf = &Config{}
	if err = confire.Process(Prefix, conf); err != nil {
		return nil, err
	}
	return conf, nil
}

//============================================================================
// Quick Access Functions
//============================================================================

// Returns true if the GENOA_LOCAL_ACCESS environment variable is set to true, indicates
// that the local kubeconfig should be used instead of the cluster service account.
func LocalAccess() bool {
	conf, err := Get()
	if err != nil {
		return false
	}
	return conf.LocalAccess
}

// Returns the namespace to use for the genoa operation.
func Namespace() string {
	conf, err := Get()
	if err != nil {
		return ""
	}
	return conf.Namespace
}

// GetLogLevel returns the log level for the config.
func (c Config) GetLogLevel() slog.Level {
	return c.LogLevel.Level()
}

//============================================================================
// Config Package Management
//============================================================================

var (
	mu   sync.RWMutex
	err  error // only written by New() inside Get's sync.Once, and cleared on successful Set
	load sync.Once
	conf *Config
)

func Get() (Config, error) {
	load.Do(func() {
		mu.Lock()
		defer mu.Unlock()

		if conf == nil {
			conf, err = New()
		}
	})

	mu.RLock()
	defer mu.RUnlock()

	if conf != nil {
		return *conf, err
	}
	return Config{}, err
}

func MustGet() Config {
	conf, err := Get()
	if err != nil {
		panic(err)
	}
	return conf
}

func Set(c Config) error {
	mu.Lock()
	defer mu.Unlock()

	conf = &c
	err = nil

	return nil
}

func Reset() {
	mu.Lock()
	defer mu.Unlock()

	conf = nil
	err = nil
	load = sync.Once{}
}
