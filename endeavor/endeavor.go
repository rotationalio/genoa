package endeavor

import (
	"context"
	"fmt"

	"go.rtnl.ai/genoa/config"
	"go.rtnl.ai/genoa/db"
	"go.rtnl.ai/genoa/errors"
	"go.rtnl.ai/genoa/k8s"
	"go.rtnl.ai/x/dsn"
	"go.rtnl.ai/x/slugify"
)

const (
	secretKeyDatabaseURL = "databaseURL"
)

func SecretName(name string) string {
	return slugify.Slugify(name) + "-endeavor"
}

// Checks if a database exists and creates it with a new password if it does not. It
// stores the new password and the database connection string in a Kubernetes secret.
func EnsureDatabase(ctx context.Context, name, secretName string) (err error) {
	if secretName == "" {
		secretName = SecretName(name)
	}

	// Connect to the admin database.
	var conf config.Config
	if conf, err = config.Get(); err != nil {
		return err
	}

	if conf.DatabaseURL == "" {
		return errors.ErrMissingDatabaseURL
	}

	var adminDSN *dsn.DSN
	if adminDSN, err = dsn.Parse(conf.DatabaseURL); err != nil {
		return errors.ErrMissingDatabaseURL
	}

	// Create a new database connection.
	var conn *db.Admin
	if conn, err = db.OpenAdmin(ctx, conf.DatabaseURL); err != nil {
		return err
	}
	defer conn.Close()

	// Check if the database exists.
	if exists, err := conn.DatabaseExists(ctx, name); err != nil {
		return err
	} else if !exists {
		// If the database does not exist, create it and the secret to access the DB.
		return CreateDatabaseAndSecret(ctx, conn, adminDSN, name, secretName)
	}

	// If the database exists, ensure there is a secret to access the DB.
	var secret k8s.Secrets
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
	return nil
}

func CreateDatabaseAndSecret(ctx context.Context, conn *db.Admin, adminDSN *dsn.DSN, name, secretName string) (err error) {
	// Attempt to fetch the secret to access the database.
	// If it doesn't exist, we will create it with a new strong password.
	var (
		createSecret bool
		secret       k8s.Secrets
	)
	if secret, err = k8s.FetchSecret(ctx, secretName); err != nil {
		if !errors.Is(err, errors.ErrNotFound) {
			return err
		}
		createSecret = true
	}

	if secret == nil {
		secret = make(k8s.Secrets, 1)
	}

	if _, ok := secret[secretKeyDatabaseURL]; !ok {
		secret[secretKeyDatabaseURL] = k8s.Secret(db.Password())
	}

	// Create the target database DSN.
	// TODO: do we need to URL escape the databae password?
	targetDSN := adminDSN.Clone()
	targetDSN.Path = name
	targetDSN.User = &dsn.UserInfo{
		Username: name,
		Password: string(secret[secretKeyDatabaseURL]),
	}

	if err = conn.CreateDatabase(ctx, targetDSN); err != nil {
		return fmt.Errorf("could not create database %s: %w", name, err)
	}

	// Create or update the secret to access the database.
	if createSecret {
		if err = k8s.CreateSecret(ctx, secretName, secret); err != nil {
			return fmt.Errorf("could not create secret %s: %w", secretName, err)
		}
	} else {
		if err = k8s.UpdateSecret(ctx, secretName, secret); err != nil {
			return fmt.Errorf("could not update secret %s: %w", secretName, err)
		}
	}

	// Success!
	return nil
}
