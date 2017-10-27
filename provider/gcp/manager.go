package gcp

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/dinesh/spotter/provider/kube"
	gce "google.golang.org/api/compute/v1"
	gke "google.golang.org/api/container/v1"
)

var (
	debugMode = os.Getenv("SPOTTER_DEBUG") == "1"
)

type Manager struct {
	GKEService  *gke.Service
	GCEService  *gce.Service
	KubeService *kube.KubeService
	NodePools   []*NodePool
	GKERef
}

func (gm *Manager) FetchNodePools() error {
	nodePoolService := gke.NewProjectsZonesClustersNodePoolsService(gm.GKEService)
	resp, err := nodePoolService.List(gm.ProjectID, gm.Zone, gm.ClusterName).Do()
	if err != nil {
		return fmt.Errorf("fetching nodePool list: %v", err)
	}

	fmt.Printf("Number of nodepools: %d\n", len(resp.NodePools))

	for _, pool := range resp.NodePools {
		if pool.Config.Preemptible {
			project, zone, igmName, err := parseGceURL(pool.InstanceGroupUrls[0], "instanceGroupManagers")
			if err != nil {
				return err
			}

			resp, err := gm.GCEService.InstanceGroupManagers.ListManagedInstances(project, zone, igmName).Do()
			if err != nil {
				return fmt.Errorf("fetching managed instances: %v", err)
			}

			instances := []string{}
			for _, mi := range resp.ManagedInstances {
				instances = append(instances, mi.Instance)
			}

			fmt.Printf("NodePool %s have %d preemptible nodes in %s group\n",
				pool.Name, len(instances), igmName)

			gm.NodePools = append(gm.NodePools, &NodePool{
				InstanceGroupName: igmName,
				Instances:         instances,
				Manager:           gm,
			})
		}
	}

	return nil
}

func (gm *Manager) Monitor() {
	fmt.Println("triggerd Monitor loop .. ")

	var wg sync.WaitGroup
	wg.Add(len(gm.NodePools))

	for _, pool := range gm.NodePools {
		go func(pool *NodePool) {
			defer wg.Done()
			pool.monitor()
		}(pool)
	}

	wg.Wait()
}

type GKERef struct {
	ProjectID   string
	Zone        string
	ClusterName string
}

type NodePool struct {
	InstanceGroupName string
	Instances         []string
	*Manager
}

func (np *NodePool) monitor() {
	var wg sync.WaitGroup
	wg.Add(len(np.Instances))

	fmt.Printf("In Nodepool Monitor loop for %s\n", np.InstanceGroupName)

	for _, instance := range np.Instances {
		go func(instance string) {
			defer wg.Done()

			if err := np.processNode(instance); err != nil {
				fmt.Printf("Error while processing node %s: %v\n", instance, err)
			}
		}(instance)
	}
	wg.Wait()
}

func (np *NodePool) processNode(instanceURL string) error {
	project, zone, name, err := parseGceURL(instanceURL, "instances")
	instance, err := np.GCEService.Instances.Get(project, zone, name).Do()
	if err != nil {
		return err
	}

	fmt.Printf("host=%s status=%s\n", name, instance.Status)
	now := time.Now()

	if instance.Status == "RUNNING" {
		if drainAtStr, ok := instance.Labels[nodeDrainTimeoutKey]; ok {
			drainAt, err := parseTimestamp(drainAtStr)
			if err != nil {
				return err
			}
			// if draining timeout is in past, and we somehow missed it, we reset the drain timeout
			if drainAt.Before(now) {
				fmt.Printf("node %s didn't get drained as scheduled at %v", name, drainAt)
				updateDrainingInstanceLabel(name, project, zone, instance, np.GCEService)
			} else if drainAt.Sub(time.Now()) <= time.Minute {
				// np.KubeService.DrainNode(name)
				np.deleteNode(name)
			} else {
				fmt.Printf("Skipping node %s because of healthy draining timeout %v\n", name, drainAt)
			}
		} else {
			updateDrainingInstanceLabel(name, project, zone, instance, np.GCEService)
		}
	} else {
		fmt.Printf("Skipping node %s because of status=%s\n", name, instance.Status)
	}

	return nil
}

func (np *NodePool) deleteNode(name string) error {
	fmt.Printf("deleting node: %s\n", name)
	_, err := np.GCEService.Instances.Delete(np.ProjectID, np.Zone, name).Do()

	return err
}

func updateDrainingInstanceLabel(host, projectID, zone string, instance *gce.Instance, gceService *gce.Service) error {
	now := time.Now()
	createdAt, err := time.Parse(time.RFC3339, instance.CreationTimestamp)
	if err != nil {
		return err
	}

	var drainDuration time.Duration

	if debugMode {
		drainDuration = time.Duration(rand.Intn(5)+2) * time.Minute
		fmt.Printf("setting drainAt for %s after %d minutes\n", host, drainDuration/time.Minute)
	} else {
		preemptDeadlineAt := createdAt.Add(24 * time.Hour)
		spanHours := int(math.Floor(preemptDeadlineAt.Sub(now).Hours() / 3))

		// set the draining time between between 1/3 and 2/3 until 24 hours of creationTime
		drainDuration = time.Duration(spanHours+rand.Intn(spanHours)) * time.Hour
		fmt.Printf("setting drainAt for %s after %d Hours\n", host, drainDuration/time.Hour)
	}
	drainAt := now.Add(drainDuration)

	labels := make(map[string]string)
	for k, v := range instance.Labels {
		labels[k] = v
	}

	labels[nodeDrainTimeoutKey] = fmt.Sprintf("%d", makeTimestamp(drainAt))

	fmt.Printf("node: %s setting labels: %+v\n", host, labels)
	op, err := gceService.Instances.SetLabels(projectID, zone, host, &gce.InstancesSetLabelsRequest{
		Labels:           labels,
		LabelFingerprint: instance.LabelFingerprint,
	}).Do()
	if err != nil {
		return fmt.Errorf("Setting instance labels: %v", err)
	}

	return waitForOperation(gceService, projectID, zone, op)
}

func waitForOperation(service *gce.Service, project, zone string, op *gce.Operation) error {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	if op.Error != nil {
		return composeOperationError(op)
	}

	for _ = range ticker.C {
		op, err := service.ZoneOperations.Get(project, zone, op.Name).Do()
		if err != nil {
			return fmt.Errorf("fetching operation: %v", err)
		}

		if op.Error != nil {
			return composeOperationError(op)
		}

		if op.Status == "DONE" {
			return nil
		}
	}

	return nil
}

func composeOperationError(op *gce.Operation) (result error) {
	errbody, jerr := op.Error.MarshalJSON()
	if jerr != nil {
		log.Fatal(jerr)
	}

	return fmt.Errorf("%s[%s] failed: %v", op.Name, op.OperationType, string(errbody))
}
