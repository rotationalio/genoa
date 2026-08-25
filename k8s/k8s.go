package k8s

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.rtnl.ai/genoa/config"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

var (
	connectOnce sync.Once
	connectErr  error
	clientset   *kubernetes.Clientset
)

const (
	TTL = 10 * time.Second
)

const (
	// Key and value for the managed-by annotation.
	AKManagedBy = "app.kubernetes.io/managed-by"
	AVManagedBy = "genoa"
)

// Authenticate to the kubernetes cluster switching between local kubeconfig or
// a cluster service account if available. To force the use of the cluster service
// account, set the svc variable to true.
//
// This method is thread-safe and will only attempt to connect to the cluster once.
func Connect() (*kubernetes.Clientset, error) {
	connectOnce.Do(func() {
		// Locate the kubernetes configuration from the local kubeconfig or the cluster service account
		var cfg *rest.Config
		if config.LocalAccess() {
			if cfg, connectErr = localK8SConfig(); connectErr != nil {
				return
			}
		} else {
			if cfg, connectErr = rest.InClusterConfig(); connectErr != nil {
				return
			}
		}

		// Create the new clientset from the config
		clientset, connectErr = kubernetes.NewForConfig(cfg)
	})

	return clientset, connectErr
}

// Resets the clientset and connect error so that it can be re-connected to the cluster.
func Reset() {
	connectOnce = sync.Once{}
	clientset = nil
	connectErr = nil
}

//============================================================================
// Helper functions
//============================================================================

func localK8SConfig() (config *rest.Config, err error) {
	// Try to find the kubeconfig file in the default location
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		if config, connectErr = clientcmd.BuildConfigFromFlags("", kubeconfig); connectErr != nil {
			return nil, err
		}
	} else if home := homedir.HomeDir(); home != "" {
		if config, connectErr = clientcmd.BuildConfigFromFlags("", filepath.Join(home, ".kube", "config")); connectErr != nil {
			return nil, err
		}

	} else {
		if config, connectErr = rest.InClusterConfig(); connectErr != nil {
			return nil, err
		}
	}
	return config, nil
}
