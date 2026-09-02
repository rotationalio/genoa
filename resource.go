package genoa

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"go.rtnl.ai/genoa/errors"
	"go.rtnl.ai/genoa/k8s"
)

// Resource is a generic object that can be deserialized from the args into an object
// registered to the Kind specified (case insensitive).
type Resource struct {
	Kind string          `json:"kind" yaml:"kind"`
	Args json.RawMessage `json:"args" yaml:"args"`
}

// A resource used to represent a database connection including its secret values.
// Note resource resolution is dependend on the Genoa command being run.
type DatabaseResource struct {
	Name     string          `json:"name" yaml:"name"`
	URL      *SecretResource `json:"url" yaml:"url"`
	Username *SecretResource `json:"username" yaml:"username"`
	Password *SecretResource `json:"password" yaml:"password"`
}

// A resource used to represent a single secret value
type SecretResource struct {
	SecretName string `json:"secretName" yaml:"secretName"`
	SecretKey  string `json:"secretkey" yaml:"secretKey"`
	Value      string `json:"value" yaml:"value"`

	secret *k8s.Secrets
}

func (s *SecretResource) IsZero() bool {
	return s == nil || (s.SecretName == "" && s.SecretKey == "" && s.Value == "")
}

func (s *SecretResource) Complete() bool {
	return s != nil && s.SecretName != "" && s.SecretKey != "" && s.Value != ""
}

func (s *SecretResource) Resolve(defaults *SecretResource) {
	if defaults == nil {
		return
	}

	if s.SecretName == "" {
		s.SecretName = defaults.SecretName
	}

	if s.SecretKey == "" {
		s.SecretKey = defaults.SecretKey
	}

	if s.Value == "" {
		s.Value = defaults.Value
	}
}

// Secret returns the secret from the Kubernetes API. If the secret is not cached, it will be fetched.
func (s *SecretResource) Secret(ctx context.Context, refresh bool) (_ *k8s.Secrets, err error) {
	if s.secret == nil || refresh {
		if s.secret, err = GetOrCreateSecret(ctx, s); err != nil {
			return nil, err
		}
	}
	return s.secret, nil
}

// Secret value returns the value from the underlying secret, not the value specified in the resource.
func (s *SecretResource) SecretValue(ctx context.Context, refresh bool) (_ string, err error) {
	var secret *k8s.Secrets
	if secret, err = s.Secret(ctx, refresh); err != nil {
		return "", err
	}
	return string(secret.Data[s.SecretKey]), nil
}

// Save the secret value to the Kubernetes API.
func (s *SecretResource) Save(ctx context.Context) (err error) {
	var secret *k8s.Secrets
	if secret, err = s.Secret(ctx, false); err != nil {
		return err
	}

	secret.Data[s.SecretKey] = k8s.Secret(s.Value)
	return secret.Save(ctx)
}

//============================================================================
// Get or Create Secret
//============================================================================

var (
	secretCacheMu sync.Mutex
	secretCache   = map[string]*k8s.Secrets{}
)

// GetOrCreateSecret gets or creates a secret from the Kubernetes API. It also uses
// a simple cache to avoid unnecessary API calls for the same secret name.
func GetOrCreateSecret(ctx context.Context, resource *SecretResource) (secret *k8s.Secrets, err error) {
	secretCacheMu.Lock()
	defer secretCacheMu.Unlock()

	switch {
	case resource.SecretName == "":
		return nil, errors.ErrMissingSecretName
	case resource.SecretKey == "":
		return nil, errors.ErrMissingSecretKey
	}

	var ok bool
	if secret, ok = secretCache[resource.SecretName]; !ok {
		// Secret does not exist in the cache, so we need to fetch it.
		if secret, err = k8s.FetchSecret(ctx, resource.SecretName); err != nil {
			if !errors.Is(err, errors.ErrNotFound) {
				return nil, err
			}
		}
	}

	// If the secret is nil, we need to create it.
	if secret == nil {
		secret = &k8s.Secrets{
			Name:        resource.SecretName,
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
			Data:        make(map[string]k8s.Secret),
		}
	}

	// If the value is specified set it on the secret.
	if resource.Value != "" {
		secret.Data[resource.SecretKey] = k8s.Secret(resource.Value)
	}

	// Ensure the secret is cached
	secretCache[secret.Name] = secret
	return secret, nil
}

//============================================================================
// Duration Serialization as a String
//============================================================================

type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *Duration) UnmarshalJSON(data []byte) (err error) {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	if d.Duration, err = time.ParseDuration(s); err != nil {
		return err
	}
	return nil
}
