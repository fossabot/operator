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
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	"kubedb.dev/apimachinery/client/clientset/versioned/typed/kubedb/v1alpha1/util"

	"github.com/appscode/go/log"
	core_util "kmodules.xyz/client-go/core/v1"
	"kmodules.xyz/client-go/tools/queue"
)

func (c *Controller) initWatcher() {
	c.mcInformer = c.KubedbInformerFactory.Kubedb().V1alpha1().Memcacheds().Informer()
	c.mcQueue = queue.New("Memcached", c.MaxNumRequeues, c.NumThreads, c.runMemcached)
	c.mcLister = c.KubedbInformerFactory.Kubedb().V1alpha1().Memcacheds().Lister()
	c.mcInformer.AddEventHandler(queue.NewReconcilableHandler(c.mcQueue.GetQueue()))
}

func (c *Controller) runMemcached(key string) error {
	log.Debugln("started processing, key:", key)
	obj, exists, err := c.mcInformer.GetIndexer().GetByKey(key)
	if err != nil {
		log.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exists {
		log.Debugf("Memcached %s does not exist anymore", key)
	} else {
		// Note that you also have to check the uid if you have a local controlled resource, which
		// is dependent on the actual instance, to detect that a Memcached was recreated with the same name
		memcached := obj.(*api.Memcached).DeepCopy()
		if memcached.DeletionTimestamp != nil {
			if core_util.HasFinalizer(memcached.ObjectMeta, api.GenericKey) {
				if err := c.terminate(memcached); err != nil {
					log.Errorln(err)
					return err
				}
				_, _, err = util.PatchMemcached(c.ExtClient.KubedbV1alpha1(), memcached, func(in *api.Memcached) *api.Memcached {
					in.ObjectMeta = core_util.RemoveFinalizer(in.ObjectMeta, api.GenericKey)
					return in
				})
				return err
			}
		} else {
			memcached, _, err = util.PatchMemcached(c.ExtClient.KubedbV1alpha1(), memcached, func(in *api.Memcached) *api.Memcached {
				in.ObjectMeta = core_util.AddFinalizer(in.ObjectMeta, api.GenericKey)
				return in
			})
			if err != nil {
				return err
			}
			if err := c.create(memcached); err != nil {
				log.Errorln(err)
				c.pushFailureEvent(memcached, err.Error())
				return err
			}
		}
	}
	return nil
}
