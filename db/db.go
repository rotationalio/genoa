package db

import (
	"context"
	"database/sql"
	"fmt"

	"go.rtnl.ai/genoa/errors"
	"go.rtnl.ai/x/dsn"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Checks that the database credentials are valid and that the database is reachable.
func Check(ctx context.Context, databaseURL string) (err error) {
	var conn *sql.DB
	if conn, err = Open(ctx, databaseURL); err != nil {
		if uri, _ := dsn.Parse(databaseURL); uri != nil {
			return fmt.Errorf("failed to connect to database %s at %s: %w", uri.Path, uri.Host, err)
		}
		return err
	}
	return conn.Close()
}

// Opens a new database connection to the given database URL and pings it to ensure it
// is reachable and ready to use with the specified connection string.
func Open(ctx context.Context, databaseURL string) (conn *sql.DB, err error) {
	var uri *dsn.DSN
	if uri, err = dsn.Parse(databaseURL); err != nil {
		return nil, err
	}

	switch uri.Provider {
	case dsn.Postgres:
		if conn, err = sql.Open("pgx", databaseURL); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported database provider: %s", uri.Provider)
	}

	if err = conn.PingContext(ctx); err != nil {
		return nil, err
	}
	return conn, nil
}

// Opens a new database connection to the given database URL and pings it to ensure it
// is reachable and ready to use with the specified connection string. A specialized
// Admin connection is returned with methods to create and manage roles and databases.
func OpenAdmin(ctx context.Context, databaseURL string) (admin *Admin, err error) {
	var conn *sql.DB
	if conn, err = Open(ctx, databaseURL); err != nil {
		return nil, err
	}
	return &Admin{DB: conn}, nil
}

// Admin is a database connection that is used to create and manage roles and databases.
type Admin struct {
	*sql.DB
}

// Checks if the database exists.
func (a *Admin) DatabaseExists(ctx context.Context, name string) (exists bool, err error) {
	if err = a.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", name).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// Creates a new role and database with the given DSN such that the role is the only
// user that can access the database (read/write) and the role cannot access any other
// databases or roles. This creates an isolated database specifically for the role.
func (a *Admin) CreateDatabase(ctx context.Context, target *dsn.DSN) (err error) {
	// Validate the target DSN.
	if target.Path == "" || target.User == nil || target.User.Username == "" || target.User.Password == "" {
		return errors.ErrInvalidCreateDSN
	}

	// With the admin connection, create the database and role and perform grant/revoke operations.
	if err = a.createDatabase(ctx, target); err != nil {
		return err
	}

	// Open a new connection to the created database and perform schema grant/revoke operations.
	if err = a.modifySchema(ctx, target); err != nil {
		return err
	}

	return nil
}

const (
	createRoleSQL     = `CREATE ROLE $1 WITH LOGIN PASSWORD $2 NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS`
	createDatabaseSQL = `CREATE DATABASE $1 OWNER $2 ENCODING 'UTF8'`
	revokePublicSQL   = `REVOKE ALL ON DATABASE $1 FROM PUBLIC`
	grantConnectSQL   = `GRANT CONNECT, TEMPORARY ON DATABASE $1 to $2`
)

// With the admin connection, create the database and role and perform grant/revoke operations.
// NOTE: these operations cannot be performed inside a transaction as they cannot be rolled back.
func (a *Admin) createDatabase(ctx context.Context, target *dsn.DSN) (err error) {
	// Create the role
	if _, err = a.ExecContext(ctx, createRoleSQL, target.User.Username, target.User.Password); err != nil {
		return fmt.Errorf("failed to create role: %w", err)
	}

	// Create the database
	if _, err = a.ExecContext(ctx, createDatabaseSQL, target.Path, target.User.Username); err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}

	// Revoke public access to the database
	if _, err = a.ExecContext(ctx, revokePublicSQL, target.Path); err != nil {
		return fmt.Errorf("failed to revoke public access: %w", err)
	}

	// Grant connect and temporary access to the role
	if _, err = a.ExecContext(ctx, grantConnectSQL, target.Path, target.User.Username); err != nil {
		return fmt.Errorf("failed to grant connect and temporary access: %w", err)
	}

	return nil
}

const (
	revokePublicSchemaSQL = `REVOKE ALL ON SCHEMA public FROM PUBLIC`
	grantAllSchemaSQL     = `GRANT ALL ON SCHEMA public TO $1`
	alterSchemaSQL        = `ALTER SCHEMA public OWNER TO $1`
)

// Open a new connection to the created database and perform schema grant/revoke operations.
func (a *Admin) modifySchema(ctx context.Context, target *dsn.DSN) (err error) {
	var conn *sql.DB
	if conn, err = Open(ctx, target.String()); err != nil {
		return err
	}
	defer conn.Close()

	var tx *sql.Tx
	if tx, err = conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable}); err != nil {
		return err
	}
	defer tx.Rollback()

	// Revoke public access to the schema
	if _, err = tx.Exec(revokePublicSchemaSQL); err != nil {
		return fmt.Errorf("failed to revoke public access to the schema: %w", err)
	}

	// Grant all access to the schema to the role
	if _, err = tx.Exec(grantAllSchemaSQL, target.User.Username); err != nil {
		return fmt.Errorf("failed to grant all access to the schema: %w", err)
	}

	// Alter the schema owner to the role
	if _, err = tx.Exec(alterSchemaSQL, target.User.Username); err != nil {
		return fmt.Errorf("failed to alter the schema owner: %w", err)
	}

	return tx.Commit()
}
