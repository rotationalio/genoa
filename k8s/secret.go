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

type Secrets map[string]Secret

type Secret string

// Fetches the secret with the specified name and returns the values of the specified
// keys. If no keys are specified, all keys are returned.
func FetchSecret(ctx context.Context, name string, keys ...string) (secrets Secrets, err error) {
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

	if len(keys) > 0 {
		secrets = make(Secrets, len(keys))
		for key, val := range obj.Data {
			if slices.Contains(keys, key) {
				secrets[key] = Secret(val)
			}
		}
	} else {
		secrets = make(Secrets, len(obj.Data))
		for key, val := range obj.Data {
			secrets[key] = Secret(val)
		}
	}

	return secrets, nil
}

// Creates a new secret with the specified name and secrets.
func CreateSecret(ctx context.Context, name string, secrets Secrets) (err error) {
	var client *kubernetes.Clientset
	if client, err = Connect(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, TTL)
	defer cancel()

	obj := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: config.Namespace(),
			Annotations: map[string]string{
				AKManagedBy: AVManagedBy,
			},
		},
		Data: make(map[string][]byte, len(secrets)),
	}

	for key, val := range secrets {
		obj.Data[key] = []byte(val)
	}

	if _, err = client.CoreV1().Secrets(config.Namespace()).Create(ctx, obj, metav1.CreateOptions{}); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

// Updates the secret with the specified name and secrets.
// NOTE: this will overwrite the secret including its existing labels and annotations.
// Ensure that only secrets created by genoa are updated.
func UpdateSecret(ctx context.Context, name string, secrets Secrets) (err error) {
	var client *kubernetes.Clientset
	if client, err = Connect(); err != nil {
		return err
	}

	obj := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: config.Namespace(),
			Annotations: map[string]string{
				AKManagedBy: AVManagedBy,
			},
		},
		Data: make(map[string][]byte, len(secrets)),
	}

	for key, val := range secrets {
		obj.Data[key] = []byte(val)
	}

	ctx, cancel := context.WithTimeout(ctx, TTL)
	defer cancel()

	if _, err = client.CoreV1().Secrets(config.Namespace()).Update(ctx, obj, metav1.UpdateOptions{}); err != nil {
		return errors.Wrap(err)
	}

	return nil
}
