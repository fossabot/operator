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
package controller

import (
	cs "kubedb.dev/apimachinery/client/clientset/versioned"
	amc "kubedb.dev/apimachinery/pkg/controller"
	"kubedb.dev/apimachinery/pkg/controller/dormantdatabase"
	"kubedb.dev/apimachinery/pkg/controller/restoresession"
	snapc "kubedb.dev/apimachinery/pkg/controller/snapshot"
	"kubedb.dev/apimachinery/pkg/eventer"
	esc "kubedb.dev/elasticsearch/pkg/controller"
	edc "kubedb.dev/etcd/pkg/controller"
	mcc "kubedb.dev/memcached/pkg/controller"
	mgc "kubedb.dev/mongodb/pkg/controller"
	myc "kubedb.dev/mysql/pkg/controller"
	pxc "kubedb.dev/percona-xtradb/pkg/controller"
	pgb "kubedb.dev/pgbouncer/pkg/controller"
	pgc "kubedb.dev/postgres/pkg/controller"
	prc "kubedb.dev/proxysql/pkg/controller"
	rdc "kubedb.dev/redis/pkg/controller"

	pcm "github.com/coreos/prometheus-operator/pkg/client/versioned/typed/monitoring/v1"
	crd_cs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	reg_util "kmodules.xyz/client-go/admissionregistration/v1beta1"
	"kmodules.xyz/client-go/discovery"
	appcat_cs "kmodules.xyz/custom-resources/client/clientset/versioned"
	scs "stash.appscode.dev/stash/client/clientset/versioned"
)

const (
	mutatingWebhookConfig   = "mutators.kubedb.com"
	validatingWebhookConfig = "validators.kubedb.com"
)

type OperatorConfig struct {
	amc.Config

	ClientConfig     *rest.Config
	KubeClient       kubernetes.Interface
	APIExtKubeClient crd_cs.ApiextensionsV1beta1Interface
	DBClient         cs.Interface
	StashClient      scs.Interface
	DynamicClient    dynamic.Interface
	AppCatalogClient appcat_cs.Interface
	PromClient       pcm.MonitoringV1Interface
	CronController   snapc.CronControllerInterface
}

func NewOperatorConfig(clientConfig *rest.Config) *OperatorConfig {
	return &OperatorConfig{
		ClientConfig: clientConfig,
	}
}

func (c *OperatorConfig) New() (*Controller, error) {
	if err := discovery.IsDefaultSupportedVersion(c.KubeClient); err != nil {
		return nil, err
	}

	recorder := eventer.NewEventRecorder(c.KubeClient, "KubeDB operator")

	// define all the controllers
	ctrl := New(
		c.ClientConfig,
		c.KubeClient,
		c.APIExtKubeClient,
		c.DBClient,
		c.StashClient,
		c.DynamicClient,
		c.AppCatalogClient,
		c.PromClient,
		c.CronController,
		c.Config,
	)

	ctrl.DrmnInformer = dormantdatabase.NewController(ctrl.Controller, nil, ctrl.Config, nil, recorder).InitInformer()
	ctrl.SnapInformer, ctrl.JobInformer = snapc.NewController(ctrl.Controller, nil, ctrl.Config, nil, recorder).InitInformer()
	ctrl.RSInformer = restoresession.NewController(ctrl.Controller, nil, ctrl.Config, nil, recorder).InitInformer()

	ctrl.edCtrl = edc.New(c.ClientConfig, c.KubeClient, c.APIExtKubeClient, c.DBClient, c.DynamicClient, c.AppCatalogClient, c.PromClient, c.CronController, ctrl.Config, recorder)
	ctrl.esCtrl = esc.New(c.ClientConfig, c.KubeClient, c.APIExtKubeClient, c.DBClient, c.StashClient, c.DynamicClient, c.AppCatalogClient, c.PromClient, c.CronController, ctrl.Config, recorder)
	ctrl.mcCtrl = mcc.New(c.ClientConfig, c.KubeClient, c.APIExtKubeClient, c.DBClient, c.AppCatalogClient, c.PromClient, ctrl.Config, recorder)
	ctrl.mgCtrl = mgc.New(c.ClientConfig, c.KubeClient, c.APIExtKubeClient, c.DBClient, c.StashClient, c.DynamicClient, c.AppCatalogClient, c.PromClient, c.CronController, ctrl.Config, recorder)
	ctrl.myCtrl = myc.New(c.ClientConfig, c.KubeClient, c.APIExtKubeClient, c.DBClient, c.StashClient, c.DynamicClient, c.AppCatalogClient, c.PromClient, c.CronController, ctrl.Config, recorder)
	ctrl.pgbCtrl = pgb.New(c.ClientConfig, c.KubeClient, c.APIExtKubeClient, c.DBClient, c.DynamicClient, c.AppCatalogClient, c.PromClient, ctrl.Config, recorder)
	ctrl.pgCtrl = pgc.New(c.ClientConfig, c.KubeClient, c.APIExtKubeClient, c.DBClient, c.StashClient, c.DynamicClient, c.AppCatalogClient, c.PromClient, c.CronController, ctrl.Config, recorder)
	ctrl.prCtrl = prc.New(c.ClientConfig, c.KubeClient, c.APIExtKubeClient, c.DBClient, c.DynamicClient, c.PromClient, ctrl.Config, recorder)
	ctrl.pxCtrl = pxc.New(c.ClientConfig, c.KubeClient, c.APIExtKubeClient, c.DBClient, c.StashClient, c.DynamicClient, c.AppCatalogClient, c.PromClient, c.CronController, ctrl.Config, recorder)

	ctrl.rdCtrl = rdc.New(c.ClientConfig, c.KubeClient, c.APIExtKubeClient, c.DBClient, c.DynamicClient, c.AppCatalogClient, c.PromClient, ctrl.Config, recorder)

	if err := ctrl.Init(); err != nil {
		return nil, err
	}

	return ctrl, nil
}

// InitInformer initializes MongoDB, DormantDB amd Snapshot watcher
func (c *Controller) Init() error {
	if err := c.EnsureCustomResourceDefinitions(); err != nil {
		return err
	}
	if c.EnableMutatingWebhook {
		if err := reg_util.UpdateMutatingWebhookCABundle(c.ClientConfig, mutatingWebhookConfig); err != nil {
			return err
		}
	}
	if c.EnableValidatingWebhook {
		if err := reg_util.UpdateValidatingWebhookCABundle(c.ClientConfig, validatingWebhookConfig); err != nil {
			return err
		}
	}

	if err := c.edCtrl.Init(); err != nil {
		return err
	}

	if err := c.esCtrl.Init(); err != nil {
		return err
	}

	if err := c.mcCtrl.Init(); err != nil {
		return err
	}

	if err := c.mgCtrl.Init(); err != nil {
		return err
	}

	if err := c.myCtrl.Init(); err != nil {
		return err
	}

	if err := c.pgbCtrl.Init(); err != nil {
		return err
	}

	if err := c.pgCtrl.Init(); err != nil {
		return err
	}

	if err := c.prCtrl.Init(); err != nil {
		return err
	}

	if err := c.pxCtrl.Init(); err != nil {
		return err
	}

	if err := c.rdCtrl.Init(); err != nil {
		return err
	}
	return nil
}
