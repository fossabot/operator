package controller

import (
	"fmt"

	tapi "github.com/k8sdb/apimachinery/api"
	"github.com/k8sdb/apimachinery/pkg/monitor"
)

func (c *Controller) newMonitorController(elastic *tapi.Elastic) (monitor.Monitor, error) {
	monitorSpec := elastic.Spec.Monitor

	if monitorSpec == nil {
		return nil, fmt.Errorf("MonitorSpec not found in %v", elastic.Spec)
	}

	if monitorSpec.Prometheus != nil {
		return monitor.NewPrometheusController(c.Client, c.promClient, c.opt.OperatorNamespace), nil
	}

	return nil, fmt.Errorf("Monitoring controller not found for %v", monitorSpec)
}

func (c *Controller) addMonitor(elastic *tapi.Elastic) error {
	ctrl, err := c.newMonitorController(elastic)
	if err != nil {
		return err
	}
	return ctrl.AddMonitor(elastic.ObjectMeta, elastic.Spec.Monitor)
}

func (c *Controller) deleteMonitor(elastic *tapi.Elastic) error {
	ctrl, err := c.newMonitorController(elastic)
	if err != nil {
		return err
	}
	return ctrl.DeleteMonitor(elastic.ObjectMeta, elastic.Spec.Monitor)
}

func (c *Controller) updateMonitor(oldElastic, updatedElastic *tapi.Elastic) error {
	var err error
	var ctrl monitor.Monitor
	if updatedElastic.Spec.Monitor == nil {
		ctrl, err = c.newMonitorController(oldElastic)
	} else {
		ctrl, err = c.newMonitorController(updatedElastic)
	}
	if err != nil {
		return err
	}
	return ctrl.UpdateMonitor(updatedElastic.ObjectMeta, oldElastic.Spec.Monitor, updatedElastic.Spec.Monitor)
}