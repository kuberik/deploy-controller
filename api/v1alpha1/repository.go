package v1alpha1

import (
	"context"
	"fmt"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Repository specifies details of a git repository
type Repository struct {
	// URL of the git repository
	URL string `json:"url,omitempty"`

	// Authentication configuration for the git repository
	Auth *RepositoryAuth `json:"auth,omitempty"`
}

// RepositoryAuth defines authentication configuration for a git repository
type RepositoryAuth struct {
	// SecretRef is a reference to a secret containing the credentials for a git repository.
	// Secret needs to contain the field <code>token</code> containing a GitHub or GitLab token
	// which has the permissions to read the repository.
	SecretRef corev1.LocalObjectReference `json:"secretRef,omitempty"`
}

const (
	AuthSecretTokenField = "token"
)

func (r *Repository) GetAuthMethod(ctx context.Context, client client.Client, namespace string) (transport.AuthMethod, error) {
	if r.Auth == nil {
		return nil, nil
	}

	authSecret := &corev1.Secret{}
	err := client.Get(ctx, types.NamespacedName{
		Name:      r.Auth.SecretRef.Name,
		Namespace: namespace,
	}, authSecret)
	if err != nil {
		return nil, err
	}

	if token, ok := authSecret.Data[AuthSecretTokenField]; ok {
		return &http.BasicAuth{
			Username: "dummy",
			Password: string(token),
		}, nil
	}

	return nil, fmt.Errorf("no credentials found in secret %s/%s", namespace, r.Auth.SecretRef.Name)
}
