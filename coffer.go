package genoa

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"go.rtnl.ai/genoa/config"
	"go.rtnl.ai/genoa/errors"
	"go.rtnl.ai/genoa/k8s"
	"go.rtnl.ai/x/purser"
)

// Default Coffer rotation period is 6 months (182 days nominal).
const (
	defaultCofferRotation  = 182 * 24 * time.Hour
	defaultCofferSecretKey = "cofferKeys"
)

// Coffer annotation keys
const (
	cofferRotatedOnAnnotation = "genoa.rtnl.ai/coffer-rotated-on"
	cofferEditionAnnotation   = "genoa.rtnl.ai/coffer-edition"
)

type CofferRotation struct {
	Release    string          `json:"release" yaml:"release"`
	Coffer     *SecretResource `json:"coffer" yaml:"coffer"`
	Namespaces []string        `json:"namespaces" yaml:"namespaces"`
	Rotation   Duration        `json:"rotation" yaml:"rotation"`

	conf    config.Config
	secret  *k8s.Secrets
	edition string
}

func (c *CofferRotation) Kind() string {
	return "CofferRotation"
}

func (c *CofferRotation) Run(ctx context.Context, conf config.Config) (err error) {

	// Validate the runtime configuration.
	c.conf = conf
	if err = c.Validate(); err != nil {
		return errors.Exit(errors.ExitConfig, err)
	}

	// Create the logger for the command
	logger := slog.With(
		slog.String("release", c.Release),
		slog.String("coffer_secret", c.Coffer.SecretName),
		slog.String("rotation", c.Rotation.String()),
	)

	// Fetch or create the secret for coffer.
	if c.secret, err = c.Coffer.Secret(ctx, false); err != nil {
		return errors.Exit(errors.ExitCommand, err)
	}

	// Set the edition of the coffer key from the secret
	if c.secret.Annotations != nil {
		c.edition = c.secret.Annotations[cofferEditionAnnotation]
	}

	// Check if the coffer key already exists in the ring.
	if secret, ok := c.secret.Data[c.Coffer.SecretKey]; ok {
		// The coffer key already exists, check if it needs to be rotated.
		rotatedOn := c.RotatedOn()
		if rotatedOn.IsZero() || rotatedOn.Add(c.Rotation.Duration).Before(time.Now()) {
			// The coffer key needs to be rotated.
			logger.Debug("rotating keys", slog.Time("last_rotated_on", rotatedOn))
			if err = c.Rotate(ctx); err != nil {
				return errors.Exit(errors.ExitCommand, err)
			}

			logger.Info("coffer keys have been rotated")
			return nil
		}

		// Check that all of the configured namespaces are present in the secret.
		cofferKeys := make(CofferKeys)
		if err = cofferKeys.Load(string(secret)); err != nil {
			return errors.Exit(errors.ExitCommand, fmt.Errorf("failed to parse coffer keys from secret: %w", err))
		}

		if len(cofferKeys) == 0 || !cofferKeys.HasAll(c.Namespaces) {
			// The coffer key is missing one or more namespaces, rotate the keys.
			logger.Debug("rotating keys to ensure all namespaces are present", slog.String("namespaces", strings.Join(c.Namespaces, ",")))
			if err = c.Rotate(ctx); err != nil {
				return errors.Exit(errors.ExitCommand, err)
			}

			logger.Info("coffer keys have been rotated to ensure all namespaces are present")
			return nil
		}

		// If it doesn't need to be rotated, then the secret is ready to go and we can return.
		logger.Info("coffer keys are up to date and ready", slog.Time("rotated_on", rotatedOn))
		return nil
	}

	// Generate the coffer keys.
	logger.Debug("secret does not contain coffer key, generating new keys", slog.String("coffer_secret_key", c.Coffer.SecretKey))
	if err = c.Rotate(ctx); err != nil {
		return errors.Exit(errors.ExitCommand, err)
	}

	logger.Info("new coffer keys have been generated")
	return nil
}

func (c *CofferRotation) Validate() (err error) {
	if c.Coffer == nil {
		c.Coffer = &SecretResource{}
	}

	if c.Coffer.SecretName == "" {
		c.Coffer.SecretName = c.Name("")
	}

	if c.Coffer.SecretKey == "" {
		c.Coffer.SecretKey = defaultCofferSecretKey
	}

	if len(c.Namespaces) == 0 {
		c.Namespaces = []string{""}
	}

	if c.Rotation.Duration < 1 {
		c.Rotation.Duration = defaultCofferRotation
	}

	if c.Coffer.SecretName == "" {
		err = errors.Join(err, errors.New("coffer secret name or release name is required"))
	}

	if c.Coffer.Value != "" {
		err = errors.Join(err, errors.New("coffer secret value must be empty"))
	}

	return err
}

func (c *CofferRotation) Name(suffix string) string {
	if c.Release == "" {
		if c.Coffer != nil && c.Coffer.SecretName != "" {
			c.Release = c.Coffer.SecretName
		} else if c.conf.Release != "" {
			c.Release = c.conf.Release
		}
	}

	suffix = strings.TrimSpace(suffix)
	if suffix == "" {
		return c.Release
	}
	return c.Release + "-" + suffix
}

func (c *CofferRotation) RotatedOn() time.Time {
	if c.secret != nil && c.secret.Annotations != nil {
		if ts, ok := c.secret.Annotations[cofferRotatedOnAnnotation]; ok {
			if rotatedOn, err := time.Parse(time.RFC3339, ts); err == nil {
				return rotatedOn
			}
		}
	}

	// Return a zero valued time if the annotation is not set.
	return time.Time{}
}

// TODO: is registration in the database required?
// TODO: on rotation, do we also need to execute a database rotation?
func (c *CofferRotation) Rotate(ctx context.Context) (err error) {
	// Set the default edition if not specified by the previous secret.
	if c.edition == "" {
		c.edition = purser.EditionV1
	}

	// Generate a new set of coffer keys
	keys := make(CofferKeys)
	if len(c.Namespaces) > 0 {
		for _, namespace := range c.Namespaces {
			if err = keys.Generate(namespace); err != nil {
				return errors.Exit(errors.ExitCommand, err)
			}
		}
	} else {
		if err = keys.Generate(""); err != nil {
			return errors.Exit(errors.ExitCommand, err)
		}
	}

	// Update the secret with the new set of coffer keys
	c.secret.Data[c.Coffer.SecretKey] = []byte(keys.Dump())
	c.secret.Annotation(cofferEditionAnnotation, c.edition)
	c.secret.Annotation(cofferRotatedOnAnnotation, time.Now().Format(time.RFC3339))

	// Update the secret in the Kubernetes cluster
	if err = c.secret.Save(ctx); err != nil {
		return err
	}

	// TODO: Register the new set of coffer keys in the database

	// TODO: Execute a database rotation if applicable

	return nil
}

// Maps the namespace to the coffer key.
type CofferKeys map[string]*CofferKey

type CofferKey struct {
	Data string           // base64 encoded data of the DER encoded private key.
	key  *ecdh.PrivateKey // The underlying private key
	der  []byte           // The DER encoded private key
}

func (c CofferKeys) Generate(namespace string) (err error) {
	key := &CofferKey{}
	if key.key, err = ecdh.X25519().GenerateKey(rand.Reader); err != nil {
		return fmt.Errorf("failed to generate coffer key: %w", err)
	}

	if key.der, err = x509.MarshalPKCS8PrivateKey(key.key); err != nil {
		return fmt.Errorf("failed to marshal coffer key: %w", err)
	}

	key.Data = base64.StdEncoding.EncodeToString(key.der)
	c[namespace] = key
	return nil
}

func (c CofferKeys) HasAll(namespaces []string) bool {
	for _, namespace := range namespaces {
		if _, ok := c[namespace]; !ok {
			return false
		}
	}
	return true
}

func (c CofferKeys) Load(secret string) (err error) {
	parts := strings.Split(secret, ",")
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		pair := strings.Split(part, ":")
		if len(pair) != 2 {
			return fmt.Errorf("invalid coffer key pair: at index %d", i)
		}

		c[pair[0]] = &CofferKey{
			Data: pair[1],
		}
	}
	return nil
}

func (c CofferKeys) Dump() string {
	// Ensure that the namespaces are sorted alphabetically.
	namespaces := make([]string, 0, len(c))
	for namespace := range c {
		namespaces = append(namespaces, namespace)
	}
	sort.Strings(namespaces)

	// Build the confire map string representation.
	sb := strings.Builder{}
	fe := true
	for _, namespace := range namespaces {
		if !fe {
			sb.WriteString(",")
		}
		fe = false

		sb.WriteString(namespace)
		sb.WriteString(":")
		sb.WriteString(c[namespace].Data)
	}
	return sb.String()
}
