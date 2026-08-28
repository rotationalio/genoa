package endeavor

import (
	"context"
	"fmt"
	"log/slog"

	"go.rtnl.ai/genoa/config"
	"go.rtnl.ai/genoa/db"
	"go.rtnl.ai/genoa/errors"
	"go.rtnl.ai/genoa/k8s"
	"go.rtnl.ai/x/dsn"
	"go.rtnl.ai/x/rlog"
	"go.rtnl.ai/x/slugify"
)

const (
	secretKeyDatabaseURL = "databaseURL"
	secretKeyPassword    = "databasePassword"
)

func SecretName(name string) string {
	return slugify.Slugify(name) + "-endeavor"
}

// Checks if a database exists and creates it with a new password if it does not. It
// stores the new password and the database connection string in a Kubernetes secret.
func EnsureDatabase(ctx context.Context, conf config.Config) (err error) {
	if conf.DatabaseURL == "" {
		return errors.Exit(errors.ExitFailure, errors.ErrMissingDatabaseURL)
	}

	var adminDSN *dsn.DSN
	if adminDSN, err = dsn.Parse(conf.DatabaseURL); err != nil {
		return errors.Exit(errors.ExitFailure, errors.ErrMissingDatabaseURL)
	}

	// Create a new database connection.
	var conn *db.Admin
	if conn, err = db.OpenAdmin(ctx, conf.DatabaseURL); err != nil {
		return errors.Exit(errors.ExitFailure, err)
	}
	defer conn.Close()

	var (
		name       = "todo"
		secretName = "todo"
	)

	if secretName == "" {
		secretName = SecretName(name)
	}

	// Check if the database exists.
	rlog.Debug("checking if database exists", slog.String("database", name))
	if exists, err := conn.DatabaseExists(ctx, name); err != nil {
		return errors.Exit(errors.ExitFailure, err)
	} else if !exists {
		// If the database does not exist, create it and the secret to access the DB.
		return CreateDatabaseAndSecret(ctx, conn, adminDSN, name, secretName)
	}

	// If the database exists, ensure there is a secret to access the DB.
	var secret k8s.Secrets
	rlog.Debug("database exists, checking database credentials secret", slog.String("secret", secretName))
	if secret, err = k8s.FetchSecret(ctx, secretName); err != nil {
		if errors.Is(err, errors.ErrNotFound) {
			return fmt.Errorf("no database credentials: secret %s does not exist", secretName)
		}
		return err
	}

	// Validate that there is a database URL in the secret and that we can connect to the database.
	if databaseURL, ok := secret[secretKeyDatabaseURL]; ok {
		if err = db.Check(ctx, string(databaseURL)); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("no database credentials: secret %s does not contain key %s", secretName, secretKeyDatabaseURL)
	}

	// Success!
	rlog.Info("endeavor database exists and is accessible", slog.String("database", name), slog.String("secret", secretName))
	return nil
}

func CreateDatabaseAndSecret(ctx context.Context, conn *db.Admin, adminDSN *dsn.DSN, name, secretName string) (err error) {
	rlog.Info("creating endeavor database and credentials secret", slog.String("database", name), slog.String("secret", secretName))

	// Attempt to fetch the secret to access the database.
	// If it doesn't exist, we will create it with a new strong password.
	var (
		createSecret bool
		secret       k8s.Secrets
	)

	rlog.Debug("checking if a credentials secret already exists", slog.String("secret", secretName))
	if secret, err = k8s.FetchSecret(ctx, secretName); err != nil {
		if !errors.Is(err, errors.ErrNotFound) {
			return err
		}
		createSecret = true
	}

	if secret == nil {
		secret = make(k8s.Secrets, 1)
	}

	if _, ok := secret[secretKeyPassword]; !ok {
		rlog.Debug("generating a new strong database password", slog.Bool("create_secret", createSecret), slog.Bool("update_secret", !createSecret))
		secret[secretKeyPassword] = k8s.Secret(db.Password())
	}

	// Create the target database DSN.
	// TODO: do we need to URL escape the database password?
	targetDSN := adminDSN.Clone()
	targetDSN.Path = name
	targetDSN.User = &dsn.UserInfo{
		Username: name,
		Password: string(secret[secretKeyPassword]),
	}

	rlog.Debug("creating endeavor database", slog.String("database", targetDSN.Path), slog.String("role", targetDSN.User.Username), slog.String("host", targetDSN.Host), slog.Int("port", int(targetDSN.Port)))
	if err = conn.CreateDatabase(ctx, targetDSN); err != nil {
		return fmt.Errorf("could not create database %s: %w", name, err)
	}

	// Create or update the secret to access the database.
	secret[secretKeyDatabaseURL] = k8s.Secret(targetDSN.String())
	if createSecret {
		rlog.Debug("creating endeavor database credentials secret", slog.String("secret", secretName))
		if err = k8s.CreateSecret(ctx, secretName, secret); err != nil {
			return fmt.Errorf("could not create secret %s: %w", secretName, err)
		}
	} else {
		rlog.Debug("updating endeavor database credentials secret", slog.String("secret", secretName))
		if err = k8s.UpdateSecret(ctx, secretName, secret); err != nil {
			return fmt.Errorf("could not update secret %s: %w", secretName, err)
		}
	}

	// Success!
	rlog.Info("endeavor database and credentials secret created successfully", slog.String("database", name), slog.String("secret", secretName))
	return nil
}
