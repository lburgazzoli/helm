package main

import (
	"context"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/kube"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"testing"
	"time"
)

const daprConfiguration = `
apiVersion: dapr.io/v1alpha1
kind: Configuration
metadata:
  name: dapr-config-test
spec:
  mtls:
    enabled: false
`
const daprCRD = `

---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  annotations:
    controller-gen.kubebuilder.io/version: v0.9.2
  creationTimestamp: null
  name: configurations.dapr.io
  labels:
    app.kubernetes.io/part-of: "dapr"
spec:
  group: dapr.io
  names:
    kind: Configuration
    listKind: ConfigurationList
    plural: configurations
    singular: configuration
  scope: Namespaced
  versions:
  - name: v1alpha1
    schema:
      openAPIV3Schema:
        description: Configuration describes an Dapr configuration setting.
        properties:
          apiVersion:
            description: 'APIVersion defines the versioned schema of this representatio'
            type: string
          kind:
            description: 'Kind is a string value representing the REST resource'
            type: string
          metadata:
            type: object
          spec:
            description: ConfigurationSpec is the spec for an configuration.
            properties:
              mtls:
                description: MTLSSpec defines mTLS configuration.
                properties:
                  enabled:
                    type: boolean
                required:
                - enabled
                type: object
            type: object
        type: object
    served: true
    storage: true
`

func TestDaprUpgradeConfiguration(t *testing.T) {

	ctx := context.Background()
	id := xid.New().String()
	values := map[string]interface{}{
		"testName": id,
	}

	registryClient, err := newDefaultRegistryClient(false)
	assert.NoError(t, err)

	c := chart.Chart{
		Metadata: &chart.Metadata{
			APIVersion: "v1",
			AppVersion: "1.0",
			Name:       "dapr",
			Version:    "1.11",
		},
		Files: []*chart.File{
			{
				Name: "crds/configuration.yaml",
				Data: []byte(daprCRD),
			},
		},
		Templates: []*chart.File{
			{
				Name: "templates/config",
				Data: []byte(daprConfiguration),
			},
		},
	}

	kc := kube.New(nil)
	kc.Namespace = id
	kc.Log = func(format string, v ...interface{}) {
		t.Helper()
		t.Logf(format, v...)
	}

	ks, err := kc.Factory.KubernetesClientSet()
	assert.NoError(t, err)

	cfg := action.Configuration{
		RESTClientGetter: genericclioptions.NewConfigFlags(true),
		KubeClient:       kc,
		Releases:         storage.Init(driver.NewMemory()),
		Capabilities:     chartutil.DefaultCapabilities,
		RegistryClient:   registryClient,
		Log: func(format string, v ...interface{}) {
			t.Helper()
			t.Logf(format, v...)
		},
	}

	t.Cleanup(func() {
		err := ks.CoreV1().Namespaces().Delete(
			ctx,
			id,
			metav1.DeleteOptions{},
		)

		if err != nil && !k8serrors.IsNotFound(err) {
			require.NoError(t, err)
		}
	})

	install := action.NewInstall(&cfg)
	install.Namespace = id
	install.CreateNamespace = true
	install.Wait = true
	install.Timeout = 300 * time.Second
	install.ReleaseName = c.Metadata.Name

	_, err = install.RunWithContext(ctx, &c, values)
	require.NoError(t, err)

	{
		upgrade := action.NewUpgrade(&cfg)
		upgrade.Namespace = id

		rel, err := upgrade.RunWithContext(ctx, c.Metadata.Name, &c, values)

		require.NoError(t, err)
		require.NotNil(t, rel)
	}

	{
		upgrade := action.NewUpgrade(&cfg)
		upgrade.Namespace = id

		rel, err := upgrade.RunWithContext(ctx, c.Metadata.Name, &c, values)

		require.NoError(t, err)
		require.NotNil(t, rel)
	}
}
