// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package integration

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/talos-systems/talos/pkg/machinery/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	capiv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	bootstrapv1alpha3 "github.com/talos-systems/cluster-api-bootstrap-provider-talos/api/v1alpha3"
	// +kubebuilder:scaffold:imports
)

var skipCleanup bool

func init() {
	const env = "INTEGRATION_SKIP_CLEANUP"
	def, _ := strconv.ParseBool(os.Getenv(env))
	flag.BoolVar(&skipCleanup, "skip-cleanup", def, fmt.Sprintf("Cleanup after tests [%s]", env))
}

// sleepCtx blocks until ctx is canceled or timeout passed.
func sleepCtx(ctx context.Context, timeout time.Duration) {
	sCtx, sCancel := context.WithTimeout(ctx, timeout)
	defer sCancel()
	<-sCtx.Done()
}

// generateName generates a unique name.
func generateName(t *testing.T, kind string) string {
	// use milliseconds since UTC midnight: unique enough, short enough, ordered
	now := time.Now().UTC()
	clock := time.Duration(now.Hour())*time.Hour +
		time.Duration(now.Minute())*time.Minute +
		time.Duration(now.Second())*time.Second +
		time.Duration(now.Nanosecond())
	n := clock / time.Microsecond

	return fmt.Sprintf("%s-%s-%d", strings.ReplaceAll(strings.ToLower(t.Name()), "/", "-"), kind, n)
}

// createCluster creates a Cluster with "ready" infrastructure.
func createCluster(ctx context.Context, t *testing.T, c client.Client, namespaceName string) *capiv1.Cluster {
	t.Helper()

	clusterName := generateName(t, "cluster")
	cluster := &capiv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespaceName,
			Name:      clusterName,
		},
		Spec: capiv1.ClusterSpec{
			ClusterNetwork: &capiv1.ClusterNetwork{},
		},
	}

	require.NoError(t, c.Create(ctx, cluster), "can't create a cluster")

	cluster.Status.InfrastructureReady = true
	require.NoError(t, c.Status().Update(ctx, cluster))

	return cluster
}

// createMachine creates a Machine owned by the Cluster.
func createMachine(ctx context.Context, t *testing.T, c client.Client, cluster *capiv1.Cluster) *capiv1.Machine {
	t.Helper()

	machineName := generateName(t, "machine")
	dataSecretName := "my-test-secret"
	machine := &capiv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      machineName,
		},
		Spec: capiv1.MachineSpec{
			ClusterName: cluster.Name,
			Bootstrap: capiv1.Bootstrap{
				DataSecretName: &dataSecretName, // TODO
			},
		},
	}

	require.NoError(t, controllerutil.SetOwnerReference(cluster, machine, scheme.Scheme))

	require.NoError(t, c.Create(ctx, machine))

	return machine
}

// createTalosConfig creates a TalosConfig owned by the Machine.
func createTalosConfig(ctx context.Context, t *testing.T, c client.Client, machine *capiv1.Machine) *bootstrapv1alpha3.TalosConfig {
	t.Helper()

	talosConfigName := generateName(t, "talosconfig")
	talosConfig := &bootstrapv1alpha3.TalosConfig{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: machine.Namespace,
			Name:      talosConfigName,
		},
		Spec: bootstrapv1alpha3.TalosConfigSpec{
			GenerateType: "init",
		},
	}

	require.NoError(t, controllerutil.SetOwnerReference(machine, talosConfig, scheme.Scheme))

	require.NoError(t, c.Create(ctx, talosConfig))

	// TODO that should not be needed
	if !skipCleanup {
		t.Cleanup(func() {
			t.Logf("Deleting TalosConfig %q ...", talosConfigName)
			assert.NoError(t, c.Delete(context.Background(), talosConfig)) // not ctx because it can be already canceled
		})
	}

	return talosConfig
}

type runtimeMode struct {
	requiresInstall bool
}

func (m runtimeMode) String() string {
	return fmt.Sprintf("runtimeMode(%v)", m.requiresInstall)
}

func (m runtimeMode) RequiresInstall() bool {
	return m.requiresInstall
}

// check interface
var _ config.RuntimeMode = runtimeMode{}
