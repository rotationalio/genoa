package db

import (
	"context"
	"database/sql"
	"fmt"

	"go.rtnl.ai/genoa/errors"
	"go.rtnl.ai/x/dsn"

	"github.com/jackc/pgx/v5"
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

	admin = &Admin{DB: conn}
	admin.DSN, _ = dsn.Parse(databaseURL)
	return admin, nil
}

// Admin is a database connection that is used to create and manage roles and databases.
type Admin struct {
	*sql.DB
	DSN *dsn.DSN
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

// NOTE: database identifiers such as roles, schemas, tables names, etc. cannot be
// parameterized with placeholders, only values can be. Therefore we use pgx.Identifier
// and its sanitize method to escape the identifiers and string formatting to create
// the SQL statements. This still ensures that the identifiers are properly escaped to
// prevent SQL injection attacks.
const (
	createRoleSQL     = `CREATE ROLE %s WITH LOGIN PASSWORD %s NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS`
	grantSetRoleSQL   = `GRANT %s TO %s WITH SET TRUE`
	createDatabaseSQL = `CREATE DATABASE %s OWNER %s ENCODING 'UTF8'`
	revokePublicSQL   = `REVOKE ALL ON DATABASE %s FROM PUBLIC`
	grantConnectSQL   = `GRANT CONNECT, TEMPORARY ON DATABASE %s TO %s`
)

// With the admin connection, create the database and role and perform grant/revoke operations.
// NOTE: these operations cannot be performed inside a transaction as they cannot be rolled back.
func (a *Admin) createDatabase(ctx context.Context, target *dsn.DSN) (err error) {
	// Create the role
	rolename := pgx.Identifier{target.User.Username}.Sanitize()
	password := quoteEscape(pgx.Identifier{target.User.Password}.Sanitize())
	if _, err = a.ExecContext(ctx, fmt.Sprintf(createRoleSQL, rolename, password)); err != nil {
		return fmt.Errorf("failed to create role %s: %w", rolename, err)
	}

	// Grant the admin user the SET ROLE privilege
	adminUser := pgx.Identifier{a.DSN.User.Username}.Sanitize()
	if _, err = a.ExecContext(ctx, fmt.Sprintf(grantSetRoleSQL, rolename, adminUser)); err != nil {
		return fmt.Errorf("failed to grant set role privilege on %s to %s: %w", rolename, adminUser, err)
	}

	// Create the database
	database := pgx.Identifier{target.Path}.Sanitize()
	if _, err = a.ExecContext(ctx, fmt.Sprintf(createDatabaseSQL, database, rolename)); err != nil {
		return fmt.Errorf("failed to create database %s with owner %s: %w", database, rolename, err)
	}

	// Revoke public access to the database
	if _, err = a.ExecContext(ctx, fmt.Sprintf(revokePublicSQL, database)); err != nil {
		return fmt.Errorf("failed to revoke public access to database %s: %w", database, err)
	}

	// Grant connect and temporary access to the role
	if _, err = a.ExecContext(ctx, fmt.Sprintf(grantConnectSQL, database, rolename)); err != nil {
		return fmt.Errorf("failed to grant connect on %s to %s: %w", database, rolename, err)
	}

	return nil
}

const (
	revokePublicSchemaSQL = `REVOKE ALL ON SCHEMA public FROM PUBLIC`
	grantAllSchemaSQL     = `GRANT ALL ON SCHEMA public TO %s`
	alterSchemaSQL        = `ALTER SCHEMA public OWNER TO %s`
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
	rolename := pgx.Identifier{target.User.Username}.Sanitize()
	if _, err = tx.Exec(fmt.Sprintf(grantAllSchemaSQL, rolename)); err != nil {
		return fmt.Errorf("failed to grant all access to the schema: %w", err)
	}

	// Alter the schema owner to the role
	if _, err = tx.Exec(fmt.Sprintf(alterSchemaSQL, rolename)); err != nil {
		return fmt.Errorf("failed to alter the schema owner: %w", err)
	}

	return tx.Commit()
}

const (
	squote = '\''
)

// quoteEscape replaces the first and last double quotes with single quotes and escapes
// any single quotes within the string. This method is necessary because the
// pgx.Identifier.Sanitize method returns a postgresql identifier rather than a string
// value which can't be used for passwords (and neither can parameterized placeholders).
//
// I really hate that we have to do this -- but as stack overflow says, the reason
// drivers don't provide escape methods is because they don't want to make it seem like
// a good idea to use them.
func quoteEscape(s string) string {
	out := make([]rune, 0, len(s)+2)
	for i, chr := range s {
		switch i {
		case 0:
			if chr == '"' {
				// Replace the first double quote with a single quote
				out = append(out, squote)
			} else if chr == squote {
				// Prepend a single quote and escape the single quote
				out = append(out, squote, squote, chr)
			} else {
				// Prepend a single quote and add the character
				out = append(out, squote, chr)
			}
		case len(s) - 1:
			if chr == '"' {
				// Replace the last double quote with a single quote
				out = append(out, squote)
			} else if chr == squote {
				// Escape the single quote and append a closing single quote
				out = append(out, squote, chr, squote)
			} else {
				// Append the character and a closing single quote
				out = append(out, chr, squote)
			}
		default:
			if chr == squote {
				// Escape the single quote
				out = append(out, squote, chr)
			} else {
				// Add the character without modification
				out = append(out, chr)
			}
		}
	}
	return string(out)
}
