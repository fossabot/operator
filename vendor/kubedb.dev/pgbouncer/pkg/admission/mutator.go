/*
Copyright The KubeDB Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package admission

import (
	"sync"

	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	cs "kubedb.dev/apimachinery/client/clientset/versioned"

	"github.com/appscode/go/types"
	admission "k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	meta_util "kmodules.xyz/client-go/meta"
	mona "kmodules.xyz/monitoring-agent-api/api/v1"
	hookapi "kmodules.xyz/webhook-runtime/admission/v1beta1"
)

const (
	defaultListenPort    = int32(5432)
	DefaultListenAddress = "*"
	defaultPoolMode      = "session"
)

type PgBouncerMutator struct {
	client      kubernetes.Interface
	extClient   cs.Interface
	lock        sync.RWMutex
	initialized bool
}

var _ hookapi.AdmissionHook = &PgBouncerMutator{}

func (a *PgBouncerMutator) Resource() (plural schema.GroupVersionResource, singular string) {
	return schema.GroupVersionResource{
			Group:    "mutators.kubedb.com",
			Version:  "v1alpha1",
			Resource: "pgbouncermutators",
		},
		"pgbouncermutator"
}

func (a *PgBouncerMutator) Initialize(config *rest.Config, stopCh <-chan struct{}) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	a.initialized = true

	var err error
	if a.client, err = kubernetes.NewForConfig(config); err != nil {
		return err
	}
	if a.extClient, err = cs.NewForConfig(config); err != nil {
		return err
	}
	return err
}

func (a *PgBouncerMutator) Admit(req *admission.AdmissionRequest) *admission.AdmissionResponse {
	status := &admission.AdmissionResponse{}

	// N.B.: No Mutating for delete
	if (req.Operation != admission.Create && req.Operation != admission.Update) ||
		len(req.SubResource) != 0 ||
		req.Kind.Group != api.SchemeGroupVersion.Group ||
		req.Kind.Kind != api.ResourceKindPgBouncer {
		status.Allowed = true
		return status
	}

	a.lock.RLock()
	defer a.lock.RUnlock()
	if !a.initialized {
		return hookapi.StatusUninitialized()
	}
	obj, err := meta_util.UnmarshalFromJSON(req.Object.Raw, api.SchemeGroupVersion)
	if err != nil {
		return hookapi.StatusBadRequest(err)
	}

	pbMod := setDefaultValues(obj.(*api.PgBouncer).DeepCopy())
	if pbMod != nil {
		patch, err := meta_util.CreateJSONPatch(req.Object.Raw, pbMod)
		if err != nil {
			return hookapi.StatusInternalServerError(err)
		}
		status.Patch = patch
		patchType := admission.PatchTypeJSONPatch
		status.PatchType = &patchType
	}

	status.Allowed = true
	return status
}

// setDefaultValues provides the defaulting that is performed in mutating stage of creating/updating a PgBouncer database
func setDefaultValues(pgbouncer *api.PgBouncer) runtime.Object {
	//func setDefaultValues(client kubernetes.Interface, extClient cs.Interface, pgbouncer *api.PgBouncer) (runtime.Object, error) {
	if pgbouncer.Spec.Replicas == nil {
		pgbouncer.Spec.Replicas = types.Int32P(1)
	}

	//TODO: Make sure an image path is set

	if pgbouncer.Spec.ConnectionPool != nil {
		if pgbouncer.Spec.ConnectionPool.Port == nil {
			pgbouncer.Spec.ConnectionPool.Port = types.Int32P(defaultListenPort)
		}
		if pgbouncer.Spec.ConnectionPool.PoolMode == "" {
			pgbouncer.Spec.ConnectionPool.PoolMode = defaultPoolMode
		}
	}
	pgbouncer.SetDefaults()

	// If monitoring spec is given without port,
	// set default Listening port
	setMonitoringPort(pgbouncer)

	return pgbouncer
}

// Assign Default Monitoring Port if MonitoringSpec Exists
// and the AgentVendor is Prometheus.
func setMonitoringPort(pgbouncer *api.PgBouncer) {
	if pgbouncer.Spec.Monitor != nil &&
		pgbouncer.GetMonitoringVendor() == mona.VendorPrometheus {
		if pgbouncer.Spec.Monitor.Prometheus == nil {
			pgbouncer.Spec.Monitor.Prometheus = &mona.PrometheusSpec{}
		}
		if pgbouncer.Spec.Monitor.Prometheus.Port == 0 {
			pgbouncer.Spec.Monitor.Prometheus.Port = api.PrometheusExporterPortNumber
		}
	}
}
