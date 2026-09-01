package genoa

import (
	"bytes"
	"cmp"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.rtnl.ai/genoa/config"
	"go.rtnl.ai/genoa/errors"
	"go.rtnl.ai/genoa/k8s"
	"go.rtnl.ai/ulid"
	"go.rtnl.ai/x/rlog"
)

// Default JWKS rotation period is 6 months (182 days nominal).
const (
	defaultJWKSMaxKeys        = 2
	defaultJWKSRotation       = 182 * 24 * time.Hour
	defaultJWKSSecret         = "quarterdeck-jwks"
	defaultJWKSQuarterdeckKey = "authKeys"
	defaultJWKSMountPath      = "/data/jwks"
)

var (
	JWKSKeyRegex = regexp.MustCompile(`^[0-7][0-9A-HJKMNP-TV-Z]{25}\.pem$`)
)

// JWKS creates signing keys for JWT tokens in Quarterdeck. If the secret resource
// specified by the signing key field does not exist, it will be created. The secret
// should also have an annotation with the last rotation timestamp; if the last rotation
// is older than the rotation period, the secret will be rotated to a new value.
//
// NOTE: this command is an automated key rotation command, so you cannot specify
// specific JWKS keys for the Quarterdeck instances. If you want to specify specific
// JWKS keys, you should create that secret manually rather than rotating them.
type JWKSRotation struct {
	MaxKeys     int             `json:"maxKeys" yaml:"maxKeys"`         // The maximum number of keys to keep in the secret.
	Rotation    Duration        `json:"rotation" yaml:"rotation"`       // The rotation period for the signing key.
	JWKSSecret  string          `json:"jwksSecret" yaml:"jwksSecret"`   // The secret to use that contains the signing key(s).
	Quarterdeck *SecretResource `json:"quarterdeck" yaml:"quarterdeck"` // The secret resource to use to specify the quarterdeck secret (same as signing key by default).
	MountPath   string          `json:"mountPath" yaml:"mountPath"`     // The mount path to use for the quarterdeck secret.

	keys *k8s.Secrets
}

func (j *JWKSRotation) Run(ctx context.Context, conf config.Config) (err error) {
	// Validate the configuration and set defaults.
	if err = j.Validate(); err != nil {
		return errors.Exit(errors.ExitConfig, err)
	}

	// Fetch any secrets if they exist, create them if they don't.
	if err = j.FetchSecrets(ctx); err != nil {
		return errors.Exit(errors.ExitCommand, err)
	}

	// Find any existing JWKS keys on the secret, removing expired keys.
	// NOTE: this command will only look for ULID keys and valid PEM encoded secret values.
	var validKeys, expiredKeys JWKSKeys
	if validKeys, expiredKeys, err = j.CheckSigningKeys(ctx); err != nil {
		return errors.Exit(errors.ExitCommand, err)
	}

	// Create a new key if we don't have enough keys
	var rotated bool
	if len(validKeys) == 0 {
		var key *JWKSKey
		if key, err = j.CreateSigningKey(ctx); err != nil {
			return errors.Exit(errors.ExitCommand, err)
		}

		// Add the annotation to the secret
		j.keys.Annotation("genoa.rtnl.ai/jwks-rotated-on", time.Now().Format(time.RFC3339))
		validKeys = append(validKeys, key)
		rotated = true
	}

	if len(validKeys) < j.MaxKeys {
		for _, keep := range expiredKeys {
			// Add back expired keys if we have room to gracefully rotate and deprecate
			// the old keys (e.g. so that they are still valid for a short period of time).
			if len(validKeys) < j.MaxKeys {
				validKeys = append(validKeys, keep)
			} else {
				break
			}
		}
	}

	// Add all valid keys back to the secret
	for _, key := range validKeys {
		j.keys.Data[key.Key] = key.Value
	}

	// Update the secret annotations
	j.keys.Annotations["genoa.rtnl.ai/jwks-max-keys"] = strconv.Itoa(j.MaxKeys)
	j.keys.Annotations["genoa.rtnl.ai/jwks-rotation-interval"] = j.Rotation.String()

	if j.Quarterdeck.SecretName != j.JWKSSecret {
		j.keys.Annotations["genoa.rtnl.ai/jwks-config-secret"] = j.Quarterdeck.SecretName
	}

	// Save the signing keys secret
	if err = j.keys.Save(ctx); err != nil {
		return errors.Exit(errors.ExitCommand, err)
	}

	// Update the quarterdeck secret value.
	var qds *k8s.Secrets
	if qds, err = j.Quarterdeck.Secret(ctx, false); err != nil {
		return err
	}

	qds.Data[j.Quarterdeck.SecretKey] = k8s.Secret(validKeys.String(j.MountPath))
	if j.Quarterdeck.SecretName != j.JWKSSecret {
		qds.Annotation("genoa.rtnl.ai/jwks-secret", j.JWKSSecret)
	}

	if err = qds.Save(ctx); err != nil {
		return err
	}

	rlog.Info("jwks key rotation complete",
		slog.String("jwks_secret", j.JWKSSecret),
		slog.String("quarterdeck_secret", j.Quarterdeck.SecretName),
		slog.Bool("rotated", rotated),
		slog.Int("max_keys", j.MaxKeys),
		slog.Int("valid_keys", len(validKeys)),
		slog.String("mount_path", j.MountPath),
	)
	return nil
}

func (j *JWKSRotation) Kind() string {
	return "JWKSRotation"
}

func (j *JWKSRotation) Validate() error {
	if j.MaxKeys < 1 {
		j.MaxKeys = defaultJWKSMaxKeys
	}

	if j.Rotation.Duration < 1 {
		j.Rotation.Duration = defaultJWKSRotation
	}

	if j.JWKSSecret == "" {
		j.JWKSSecret = defaultJWKSSecret
	}

	if j.Quarterdeck == nil {
		j.Quarterdeck = &SecretResource{}
	}

	if j.Quarterdeck.SecretName == "" {
		j.Quarterdeck.SecretName = j.JWKSSecret
	}

	if j.Quarterdeck.SecretKey == "" {
		j.Quarterdeck.SecretKey = defaultJWKSQuarterdeckKey
	}

	if j.MountPath == "" {
		j.MountPath = defaultJWKSMountPath
	}

	if j.Quarterdeck.Value != "" {
		return errors.New("quarterdeck secret value must be empty")
	}

	return nil
}

func (j *JWKSRotation) FetchSecrets(ctx context.Context) (err error) {
	// We're just going to get the signing secret if it exists, otherwise we'll create
	// an empty secret without any keys specified.
	if err = j.fetchSigningSecret(ctx); err != nil {
		return err
	}

	// The quarterdeck secret is just a normal secret resource.
	if _, err = j.Quarterdeck.Secret(ctx, false); err != nil {
		return err
	}
	return nil
}

func (j *JWKSRotation) fetchSigningSecret(ctx context.Context) (err error) {
	// NOTE: this must use the cache so we don't create different secret pointers for
	// the same secret, which might lead to overwriting the secret data.
	secretCacheMu.Lock()
	defer secretCacheMu.Unlock()

	var ok bool
	if j.keys, ok = secretCache[j.JWKSSecret]; !ok {
		if j.keys, err = k8s.FetchSecret(ctx, j.JWKSSecret); err != nil {
			if !errors.Is(err, errors.ErrNotFound) {
				return err
			}
		}
	}

	if j.keys == nil {
		j.keys = &k8s.Secrets{
			Name:        j.JWKSSecret,
			Annotations: make(map[string]string),
			Data:        make(map[string]k8s.Secret),
		}
	}

	secretCache[j.JWKSSecret] = j.keys
	return nil
}

func (j *JWKSRotation) CheckSigningKeys(ctx context.Context) (validKeys, expired JWKSKeys, err error) {
	now := time.Now()
	for key, value := range j.keys.Data {
		if !JWKSKeyRegex.MatchString(key) {
			continue
		}

		// Remove all valid signing keys from the secret
		delete(j.keys.Data, key)

		// Parse the timestamp from the ULID
		var keyID ulid.ULID
		if keyID, err = ulid.Parse(key[:26]); err != nil {
			return nil, nil, err
		}

		jwks := &JWKSKey{
			Key:   key,
			Value: value,
		}

		// Check if the key is expired
		if keyID.Timestamp().Add(j.Rotation.Duration).Before(now) {
			expired = append(expired, jwks)
		} else {
			validKeys = append(validKeys, jwks)
		}
	}

	// Sort the keys
	validKeys.Sort()
	expired.Sort()

	return validKeys, expired, nil
}

func (j *JWKSRotation) CreateSigningKey(ctx context.Context) (key *JWKSKey, err error) {
	// Create the KeyID and the path defined by the KeyID.
	keyID := ulid.MakeSecure()
	path := keyID.String() + ".pem"

	// Create a new key and generate the public/private key pair.
	key = &JWKSKey{
		Key: path,
	}

	if key.public, key.private, err = ed25519.GenerateKey(rand.Reader); err != nil {
		return nil, err
	}

	// PEM Encode the public/private key pair into the JWKSKey Value.
	if err = key.Encode(); err != nil {
		return nil, err
	}
	return key, nil
}

//============================================================================
// JWKS Key Helper Types
//============================================================================

type JWKSKeys []*JWKSKey

func (j JWKSKeys) Sort() {
	slices.SortFunc(j, func(a, b *JWKSKey) int {
		return cmp.Compare(b.Key, a.Key)
	})
}

// Returns an envconfig map of the JWKS keys.
func (j JWKSKeys) String(mountPath string) string {
	var buf strings.Builder
	for i, key := range j {
		if i > 0 {
			buf.WriteString(",")
		}
		path := filepath.Join(mountPath, key.Key)
		keyID := ulid.MustParse(key.Key[:26]).String()
		fmt.Fprintf(&buf, "%s:%s", keyID, path)
	}
	return buf.String()
}

type JWKSKey struct {
	Key   string
	Value []byte

	private ed25519.PrivateKey
	public  ed25519.PublicKey
}

func (j *JWKSKey) Encode() (err error) {
	if len(j.private) == 0 || len(j.public) == 0 {
		return errors.New("private or public key is empty")
	}

	private := &pem.Block{Type: "PRIVATE KEY"}
	if private.Bytes, err = x509.MarshalPKCS8PrivateKey(j.private); err != nil {
		return fmt.Errorf("could not marshal private key: %w", err)
	}

	public := &pem.Block{Type: "PUBLIC KEY"}
	if public.Bytes, err = x509.MarshalPKIXPublicKey(j.public); err != nil {
		return fmt.Errorf("could not marshal public key: %w", err)
	}

	var buf bytes.Buffer
	if err = pem.Encode(&buf, private); err != nil {
		return fmt.Errorf("could not encode private key: %w", err)
	}

	if err = pem.Encode(&buf, public); err != nil {
		return fmt.Errorf("could not encode public key: %w", err)
	}

	j.Value = buf.Bytes()
	return nil
}
