package genoa

import (
	"context"
	"fmt"
	"log/slog"

	"go.rtnl.ai/genoa/config"
	"go.rtnl.ai/genoa/db"
	"go.rtnl.ai/genoa/errors"
	"go.rtnl.ai/genoa/k8s"
	"go.rtnl.ai/x/rlog"
)

const (
	qdsSecretNameKey     = "superuserName"
	qdsSecretEmailKey    = "superuserEmail"
	qdsSecretPasswordKey = "superuserPassword"
	qdsSecretForceKey    = "superuserForcePassword"
)

type QuarterdeckSuperuser struct {
	Name     string           `json:"name" yaml:"name"`         // Optional: the name of the superuser e.g. "Rotational Support"
	Email    string           `json:"email" yaml:"email"`       // Required: the email address of the superuser e.g. "support@rotational.io"
	Password string           `json:"password" yaml:"password"` // Optional: the password of the superuser e.g. "password", will be generated if omitted.
	Force    bool             `json:"force" yaml:"force"`       // Optional: if true, the superuser will be updated if it already exists.
	Secret   *SuperuserSecret `json:"secret" yaml:"secret"`     // Optional: the secret definition to use for the superuser credentials.

	secret *k8s.Secrets
}

type SuperuserSecret struct {
	Name             string `json:"name" yaml:"name"`               // The name of the secret to use for the superuser credentials.
	NameKey          string `json:"nameKey" yaml:"nameKey"`         // The key to use for the name in the secret.
	EmailKey         string `json:"emailKey" yaml:"emailKey"`       // The key to use for the email in the secret.
	PasswordKey      string `json:"passwordKey" yaml:"passwordKey"` // The key to use for the password in the secret.
	ForcePasswordKey string `json:"forceKey" yaml:"forceKey"`       // The key to use for the force in the secret.
}

func (q *QuarterdeckSuperuser) Kind() string {
	return "QuarterdeckSuperuser"
}

func (q *QuarterdeckSuperuser) Run(ctx context.Context, conf config.Config) (err error) {
	// Validate the configuration and set defaults.
	if err = q.Validate(); err != nil {
		return errors.Exit(errors.ExitConfig, err)
	}

	// Fetch any secrets if they exist, create them if they don't.
	if err = q.FetchSecret(ctx); err != nil {
		return errors.Exit(errors.ExitCommand, err)
	}

	switch op := q.Operation(ctx); op {
	case "create":
		if err = q.CreateSecret(ctx); err != nil {
			return errors.Exit(errors.ExitCommand, err)
		}
		rlog.Info("created quarterdeck superuser secret", slog.String("secret", q.secret.Name))
		return nil
	case "update":
		// NOTE: q.UpdateSecret handles all logging based on the operation result.
		if err = q.UpdateSecret(ctx); err != nil {
			return errors.Exit(errors.ExitCommand, err)
		}
		return nil
	case "match":
		rlog.Info("quarterdeck superuser secret already exists and matches configured values")
		return nil
	default:
		return fmt.Errorf("unknown operation %q", op)
	}
}

func (q *QuarterdeckSuperuser) Validate() (err error) {
	if serr := q.Secret.Validate(); err != nil {
		err = errors.Join(err, serr)
	}

	if q.Email == "" {
		err = errors.Join(err, errors.New("quarterdeck superuser email is required"))
	}

	return err
}

func (s *SuperuserSecret) Validate() error {
	if s.Name == "" {
		return errors.New("quarterdeck superuser secret name is required")
	}

	if s.NameKey == "" {
		s.NameKey = qdsSecretNameKey
	}

	if s.EmailKey == "" {
		s.EmailKey = qdsSecretEmailKey
	}

	if s.PasswordKey == "" {
		s.PasswordKey = qdsSecretPasswordKey
	}

	if s.ForcePasswordKey == "" {
		s.ForcePasswordKey = qdsSecretForceKey
	}

	return nil
}

func (q *QuarterdeckSuperuser) FetchSecret(ctx context.Context) (err error) {
	// NOTE: this must use the cache so we don't create different secret pointers for
	// the same secret, which might lead to overwriting the secret data.
	secretCacheMu.Lock()
	defer secretCacheMu.Unlock()

	var ok bool
	if q.secret, ok = secretCache[q.Secret.Name]; !ok {
		if q.secret, err = k8s.FetchSecret(ctx, q.Secret.Name); err != nil {
			if !errors.Is(err, errors.ErrNotFound) {
				return err
			}
		}
	}

	if q.secret == nil {
		q.secret = &k8s.Secrets{
			Name:        q.Secret.Name,
			Annotations: make(map[string]string),
			Data:        make(map[string]k8s.Secret),
		}
	}

	secretCache[q.Secret.Name] = q.secret
	return nil
}

// Returns "create" if the secret is empty, "match" if the secret exists and matches
// the configured values and "update" if the secret exists and does not match the
// configured values.
func (q *QuarterdeckSuperuser) Operation(ctx context.Context) string {
	// Get the secret data from the secret.
	//cSpell:ignore sname, semail, spassword
	sname := q.secret.Data[q.Secret.NameKey].String()
	semail := q.secret.Data[q.Secret.EmailKey].String()
	spassword := q.secret.Data[q.Secret.PasswordKey].String()

	// If the secret is empty, return "create".
	if sname == "" && semail == "" && spassword == "" {
		return "create"
	}

	// If any of the values do not match, return "update".
	if sname != q.Name || semail != q.Email || (q.Password != "" && spassword != q.Password) || (q.Password == "" && spassword == "") {
		return "update"
	}

	// All above values match, no operation is needed.
	return "match"
}

func (q *QuarterdeckSuperuser) CreateSecret(ctx context.Context) (err error) {
	// Only set the name if it is configured.
	if q.Name != "" {
		q.secret.Data[q.Secret.NameKey] = k8s.Secret(q.Name)
	}

	// If the password is empty, create a new one.
	if q.Password == "" {
		q.GeneratePassword(ctx)
	}

	q.secret.Data[q.Secret.EmailKey] = k8s.Secret(q.Email)
	q.secret.Data[q.Secret.PasswordKey] = k8s.Secret(q.Password)

	if q.Force {
		q.secret.Data[q.Secret.ForcePasswordKey] = k8s.Secret("true")
	}

	return q.secret.Save(ctx)
}

func (q *QuarterdeckSuperuser) UpdateSecret(ctx context.Context) (err error) {
	// Cannot update the name or email
	if q.Name != "" && q.Name != q.secret.Data[q.Secret.NameKey].String() {
		return errors.New("cannot update the name of the quarterdeck superuser secret")
	}

	if q.Email != "" && q.Email != q.secret.Data[q.Secret.EmailKey].String() {
		return errors.New("cannot update the email of the quarterdeck superuser secret")
	}

	if q.Password != "" && q.Password != q.secret.Data[q.Secret.PasswordKey].String() {
		// If there is a password mismatch, warn if not forcing.
		if !q.Force {
			rlog.Warn("password mismatch without forcing, password may not be updated")
		}

		// If the password is not empty, set it in the secret
		q.secret.Data[q.Secret.PasswordKey] = k8s.Secret(q.Password)
	} else if q.Password == "" && q.secret.Data[q.Secret.PasswordKey].String() == "" {
		// Generate a new password if the current password is empty.
		q.GeneratePassword(ctx)
		q.secret.Data[q.Secret.PasswordKey] = k8s.Secret(q.Password)
	}

	if q.Force {
		q.secret.Data[q.Secret.ForcePasswordKey] = k8s.Secret("true")
	}

	if err = q.secret.Save(ctx); err != nil {
		return err
	}

	rlog.Info("updated quarterdeck superuser secret", slog.String("secret", q.secret.Name))
	return nil
}

func (q *QuarterdeckSuperuser) GeneratePassword(ctx context.Context) {
	q.Password = db.Password()
}
