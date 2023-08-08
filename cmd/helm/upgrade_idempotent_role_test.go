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
	"testing"
	"time"
)

const daprEmptyClusterRoles = `
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: {{ .Values.testName }}
rules: null
`

func TestDaprUpgradeRoleWithNoRules(t *testing.T) {

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
		Templates: []*chart.File{
			{
				Name: "templates/placement",
				Data: []byte(daprEmptyClusterRoles),
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
		KubeClient:     kc,
		Releases:       storage.Init(driver.NewMemory()),
		Capabilities:   chartutil.DefaultCapabilities,
		RegistryClient: registryClient,
		Log: func(format string, v ...interface{}) {
			t.Helper()
			t.Logf(format, v...)
		},
	}

	t.Cleanup(func() {
		err := ks.RbacV1().Roles(id).Delete(
			ctx,
			"dapr-placement",
			metav1.DeleteOptions{},
		)

		if err != nil && !k8serrors.IsNotFound(err) {
			require.NoError(t, err)
		}
	})
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

	upgrade := action.NewUpgrade(&cfg)
	upgrade.Namespace = id
	_, err = upgrade.RunWithContext(ctx, c.Metadata.Name, &c, values)

	require.NoError(t, err)
}