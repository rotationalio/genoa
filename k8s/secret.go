package k8s

import (
	"context"
	"slices"

	"go.rtnl.ai/genoa/config"
	"go.rtnl.ai/genoa/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

//============================================================================
// Internal Secret Type
//============================================================================

type Secrets struct {
	Name        string
	Labels      map[string]string
	Annotations map[string]string
	Data        map[string]Secret
	created     bool // Set to true by fetch to update the secret rather than create it.
}

type Secret []byte

func (s *Secrets) Save(ctx context.Context) (err error) {
	if s.Name == "" {
		return errors.ErrMissingSecretName
	}

	if s.created {
		return UpdateSecret(ctx, s)
	} else {
		return CreateSecret(ctx, s)
	}
}

func (s *Secrets) Load(secret *corev1.Secret, keys ...string) error {
	s.Name = secret.Name

	s.Labels = make(map[string]string, len(secret.Labels))
	for key, val := range secret.Labels {
		s.Labels[key] = val
	}

	s.Annotations = make(map[string]string, len(secret.Annotations))
	for key, val := range secret.Annotations {
		s.Annotations[key] = val
	}

	if len(keys) > 0 {
		s.Data = make(map[string]Secret, len(keys))
		for key, val := range secret.Data {
			if slices.Contains(keys, key) {
				s.Data[key] = Secret(val)
			}
		}
	} else {
		s.Data = make(map[string]Secret, len(secret.Data))
		for key, val := range secret.Data {
			s.Data[key] = Secret(val)
		}
	}

	return nil
}

func (s *Secrets) Dump() (obj *corev1.Secret, err error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        s.Name,
			Labels:      s.Labels,
			Annotations: s.Annotations,
		},
		Data: make(map[string][]byte, len(s.Data)),
	}

	for key, val := range s.Data {
		secret.Data[key] = []byte(val)
	}

	if _, ok := secret.Annotations[AKManagedBy]; !ok {
		secret.Annotations[AKManagedBy] = AVManagedBy
	}

	return secret, nil
}

func (s *Secrets) Label(key, value string) {
	s.Labels[key] = value
}

func (s *Secrets) Annotation(key, value string) {
	s.Annotations[key] = value
}

func (s *Secrets) Secret(key string, value []byte) {
	s.Data[key] = Secret(value)
}

func (s Secret) String() string {
	return string(s)
}

//============================================================================
// Kubernetes Secret Interaction
//============================================================================

// Fetches the secret with the specified name and returns the values of the specified
// keys. If no keys are specified, all keys are returned.
func FetchSecret(ctx context.Context, name string, keys ...string) (secrets *Secrets, err error) {
	var client *kubernetes.Clientset
	if client, err = Connect(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, TTL)
	defer cancel()

	var obj *corev1.Secret
	if obj, err = client.CoreV1().Secrets(config.Namespace()).Get(ctx, name, metav1.GetOptions{}); err != nil {
		return nil, errors.Wrap(err)
	}

	secrets = &Secrets{}
	if err = secrets.Load(obj, keys...); err != nil {
		return nil, err
	}
	secrets.created = true
	return secrets, nil
}

// Creates a new secret with the specified name and secrets.
func CreateSecret(ctx context.Context, secrets *Secrets) (err error) {
	var obj *corev1.Secret
	if obj, err = secrets.Dump(); err != nil {
		return err
	}

	var client *kubernetes.Clientset
	if client, err = Connect(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, TTL)
	defer cancel()

	if _, err = client.CoreV1().Secrets(config.Namespace()).Create(ctx, obj, metav1.CreateOptions{}); err != nil {
		return errors.Wrap(err)
	}

	// Mark the secret as created so that it is updated rather than created on save.
	secrets.created = true
	return nil
}

// Updates the secret with the specified name and secrets.
// NOTE: this will overwrite the secret including its existing labels and annotations.
// Ensure that only secrets created by genoa are updated.
func UpdateSecret(ctx context.Context, secrets *Secrets) (err error) {
	var obj *corev1.Secret
	if obj, err = secrets.Dump(); err != nil {
		return err
	}

	var client *kubernetes.Clientset
	if client, err = Connect(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, TTL)
	defer cancel()

	if _, err = client.CoreV1().Secrets(config.Namespace()).Update(ctx, obj, metav1.UpdateOptions{}); err != nil {
		return errors.Wrap(err)
	}

	return nil
}
