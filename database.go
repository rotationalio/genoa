package genoa

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"strings"

	"go.rtnl.ai/genoa/config"
	"go.rtnl.ai/genoa/db"
	"go.rtnl.ai/genoa/errors"
	"go.rtnl.ai/x/dsn"
	"go.rtnl.ai/x/rlog"
)

// Checks if a database exists and creates it with a new password if it does not. It
// stores the new password and the database connection string in a Kubernetes secret.
type EnsureDatabase struct {
	Release  string            `json:"release" yaml:"release"`   // The default name to use for the creation of resources.
	Database *DatabaseResource `json:"database" yaml:"database"` // Definition of the database to ensure.

	pg   *db.Admin     // Admin connection to the database.
	dsn  *dsn.DSN      // Target database DSN to ensure existence for.
	conf config.Config // Configuration for the operation.
}

func (e *EnsureDatabase) Run(ctx context.Context, conf config.Config) (err error) {
	// Ensure there is a release name to use for the creation of resources.
	e.conf = conf
	if name := e.Name(""); name == "" {
		return errors.Exit(errors.ExitFailure, errors.Join(errors.ErrMissingRelease, errors.New("could not identify release name from database or environment")))
	}

	// Ensure we have our database definition
	if e.Database == nil {
		e.Database = &DatabaseResource{}
	}

	// Fetch or create the secrets for our various resources.
	// NOTE: a release name is required for the creation of secrets.
	if err = e.FetchSecrets(ctx); err != nil {
		return err
	}

	// Connect to the admin database and ensure the connection is closed when done.
	if err := e.Connect(ctx); err != nil {
		return err
	}
	defer e.pg.Close()

	// Resolve the database resource from various secrets and sources.
	if err := e.Resolve(ctx); err != nil {
		return err
	}

	// After resolving the database resource, ensure that we can are in a valid state.
	if err := e.Validate(ctx); err != nil {
		return errors.Exit(errors.ExitFailure, fmt.Errorf("could not validate database resource: %w", err))
	}

	// Check if the database exists.
	var exists bool
	if exists, err = e.pg.DatabaseExists(ctx, e.Database.Name); err != nil {
		return err
	}

	// If the database does not exist, create it.
	if !exists {
		// If the database does not exist, create it.
		rlog.Debug("creating endeavor database", slog.String("database", e.dsn.Path), slog.String("role", e.dsn.User.Username), slog.String("host", e.dsn.Host), slog.Int("port", int(e.dsn.Port)))
		if err = e.pg.CreateDatabase(ctx, e.dsn); err != nil {
			return errors.Exit(errors.ExitFailure, fmt.Errorf("could not create database %s: %w", e.Database.Name, err))
		}
	}

	// Validate that we can connect to the database.
	if err = db.Check(ctx, e.Database.URL.Value); err != nil {
		return errors.Exit(errors.ExitFailure, fmt.Errorf("could not connect to database %s: %w", e.Database.Name, err))
	}

	// Save any secrets that may have been modified.
	if err = e.SaveSecrets(ctx); err != nil {
		return errors.Exit(errors.ExitFailure, fmt.Errorf("could not save secrets: %w", err))
	}

	rlog.Info("endeavor database exists and is accessible", slog.String("database", e.Database.Name), slog.String("secret", e.Database.URL.SecretName))
	return nil
}

func (e *EnsureDatabase) Kind() string {
	return "EnsureDatabase"
}

// Resolution order of the database resource is as follows:
// 1. If the database name is provided, it overrides all other values.
// 2. If the database username and passwords are provided, it overrides all other values.
// 3. If the database URL is provided, it is used to populate missing above and
// 4. The admin database DSN is used to populate missing above
//
// NOTE: resolve must run after FetchSecrets has been called.
func (e *EnsureDatabase) Resolve(ctx context.Context) (err error) {
	// Get the database URL from the secret and parse it into a DSN if it exists.
	e.dsn = &dsn.DSN{}
	if e.Database.URL.Value != "" {
		// If the database URL is specified in the resource, use it.
		if e.dsn, err = dsn.Parse(e.Database.URL.Value); err != nil {
			return fmt.Errorf("could not parse database URL from resource: %w", err)
		}
		rlog.Debug("using database URL from resource")
	} else {
		// Otherwise try to see if the database URL is specified in the secret.
		if databaseURL, err := e.Database.URL.SecretValue(ctx, false); err == nil && databaseURL != "" {
			if e.dsn, err = dsn.Parse(databaseURL); err != nil {
				return fmt.Errorf("could not parse database URL from secret: %w", err)
			}
			rlog.Debug("using database URL from secret")
		}
	}

	// If the database name is not set, use the database URL to populate it.
	if e.Database.Name == "" {
		// Determine if the database URL is specified in the DSN and use it if so.
		e.Database.Name = e.dsn.Path

		// If the database name is sill not set, use the release name.
		if e.Database.Name == "" {
			rlog.Debug("using release name as database name")
			e.Database.Name = e.Name("")
		} else {
			rlog.Debug("using databaseURL secret for database name")
		}
	} else {
		rlog.Debug("using database name from resource")
	}

	// Update the dsn with the database name (might not have any effect).
	e.dsn.Path = e.Database.Name

	// If the database username is not set, resolve it from the secret, the dsn, and
	// finally use the database name as the user name if not specified.
	if e.Database.Username.Value == "" {
		if e.Database.Username.Value, err = e.Database.Username.SecretValue(ctx, false); err != nil {
			return fmt.Errorf("could not get database username from secret: %w", err)
		}

		// If the username is still not set, use the dsn user name as the user name.
		if e.Database.Username.Value == "" {
			e.Database.Username.Value = e.dsn.User.Username
		}

		// If the username is still not set, use the database name as the user name.
		if e.Database.Username.Value == "" {
			e.Database.Username.Value = e.Database.Name
		}
	} else {
		rlog.Debug("using database username from resource")
	}

	// If the database password is not set, resolve it from the secret, the dsn, and
	// finally generate a new password if not specified.
	if e.Database.Password.Value == "" {
		if e.Database.Password.Value, err = e.Database.Password.SecretValue(ctx, false); err != nil {
			return fmt.Errorf("could not get database password from secret: %w", err)
		}

		// If the password is still not set, use the dsn password as the password.
		if e.Database.Password.Value == "" {
			e.Database.Password.Value = e.dsn.User.Password

			// If the password is still not set, generate a new password.
			// TODO: do we need to URL escape the database password?
			if e.Database.Password.Value == "" {
				rlog.Debug("generating a new strong database password")
				e.Database.Password.Value = db.Password()
			}
		} else {
			rlog.Debug("using database password from secret")
		}
	} else {
		rlog.Warn("using database password from resource (not recommended)")
	}

	// Update the dsn with the database username and password (may not have any effect).
	e.dsn.User = &dsn.UserInfo{
		Username: e.Database.Username.Value,
		Password: e.Database.Password.Value,
	}

	// At this point the database name (path), username, and password should all be populated
	// and set on to the target DSN. All other fields should come from the admin DSN.
	e.dsn.Provider = e.pg.DSN.Provider
	e.dsn.Driver = e.pg.DSN.Driver
	e.dsn.Host = e.pg.DSN.Host
	e.dsn.Port = e.pg.DSN.Port
	e.dsn.Options = maps.Clone(e.pg.DSN.Options)

	// Finally update the database URL secret with the new database DSN (may not have any effect).
	e.Database.URL.Value = e.dsn.String()

	// At this point, the database resource should be fully resolved and the target DSN should be populated.
	return nil
}

// Connects to the database and sets the admin connection.
func (e *EnsureDatabase) Connect(ctx context.Context) (err error) {
	if e.conf.DatabaseURL == "" {
		return errors.Exit(errors.ExitFailure, errors.ErrMissingDatabaseURL)
	}

	var adminDSN *dsn.DSN
	if adminDSN, err = dsn.Parse(e.conf.DatabaseURL); err != nil {
		return errors.Exit(errors.ExitFailure, errors.ErrMissingDatabaseURL)
	}

	if e.pg, err = db.OpenAdmin(ctx, adminDSN.String()); err != nil {
		return errors.Exit(errors.ExitFailure, err)
	}

	return nil
}

func (e *EnsureDatabase) FetchSecrets(ctx context.Context) (err error) {
	// Resolve all secrets with their defaults specified by this package.
	name := e.Name("")
	if e.Database.Username == nil {
		e.Database.Username = &SecretResource{}
	}
	e.Database.Username.Resolve(&SecretResource{SecretName: name, SecretKey: "databaseUsername", Value: name})

	if e.Database.Password == nil {
		e.Database.Password = &SecretResource{}
	}
	e.Database.Password.Resolve(&SecretResource{SecretName: name, SecretKey: "databasePassword"})

	if e.Database.URL == nil {
		e.Database.URL = &SecretResource{}
	}
	e.Database.URL.Resolve(&SecretResource{SecretName: name, SecretKey: "databaseURL"})

	// Fetch all secrets from the Kubernetes cluster.
	if _, err = e.Database.Username.Secret(ctx, false); err != nil {
		return err
	}

	if _, err = e.Database.Password.Secret(ctx, false); err != nil {
		return err
	}

	if _, err = e.Database.URL.Secret(ctx, false); err != nil {
		return err
	}

	return nil
}

func (e *EnsureDatabase) SaveSecrets(ctx context.Context) (err error) {
	if err = e.Database.Username.Save(ctx); err != nil {
		return err
	}

	if err = e.Database.Password.Save(ctx); err != nil {
		return err
	}

	if err = e.Database.URL.Save(ctx); err != nil {
		return err
	}

	return nil
}

func (e *EnsureDatabase) Name(suffix string) string {
	if e.Release == "" {
		if e.Database != nil && e.Database.Name != "" {
			e.Release = e.Database.Name
		} else if e.conf.Release != "" {
			e.Release = e.conf.Release
		}
	}

	suffix = strings.TrimSpace(suffix)
	if suffix == "" {
		return e.Release
	}
	return e.Release + "-" + suffix
}

func (e *EnsureDatabase) Validate(ctx context.Context) (err error) {
	if e.Database.Name == "" {
		err = errors.Join(err, errors.New("database resource name is required"))
	}

	if e.Database.Username == nil || !e.Database.Username.Complete() {
		err = errors.Join(err, errors.New("database resource username is required"))
	}

	if e.Database.Password == nil || !e.Database.Password.Complete() {
		err = errors.Join(err, errors.New("database resource password is required"))
	}

	if e.Database.URL == nil || !e.Database.URL.Complete() {
		err = errors.Join(err, errors.New("database resource URL is required"))
	}

	if e.dsn == nil || e.dsn.String() != e.Database.URL.Value {
		err = errors.Join(err, errors.New("target database DSN does not match resolved database resource"))
	}

	return err
}
