package gcp

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	gce "google.golang.org/api/compute/v1"
	gke "google.golang.org/api/container/v1"
)

type GKEManager struct {
	GKEService *gke.Service
	GCEService *gce.Service
	NodePools  []*NodePool
	GKERef
}

func (gm *GKEManager) FetchNodePools() error {
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
				return fmt.Errorf("fetching managed instaces: %v", err)
			}

			instances := []string{}
			for _, mi := range resp.ManagedInstances {
				instances = append(instances, mi.Instance)
			}

			gm.NodePools = append(gm.NodePools, &NodePool{
				InstanceGroupName: igmName,
				Instances:         instances,
			})
		}
	}

	return nil
}

func (gm *GKEManager) Monitor() {
	var wg sync.WaitGroup
	wg.Add(len(gm.NodePools))

	for _, pool := range gm.NodePools {
		go func() {
			defer wg.Done()
			pool.monitor()
		}()
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

	GCEService *gce.Service
}

func (np *NodePool) monitor() {
	var wg sync.WaitGroup
	wg.Add(len(np.Instances))

	for _, instance := range np.Instances {
		go func() {
			np.processNode(instance)
			wg.Done()
		}()
	}
	wg.Wait()
}

func (np *NodePool) processNode(instanceURL string) error {
	project, zone, name, err := parseGceURL(instanceURL, "instances")
	instance, err := np.GCEService.Instances.Get(project, zone, name).Do()
	if err != nil {
		return err
	}

	if instance.Status == "RUNNING" {
		if drainAtStr, ok := instance.Labels[nodeDrainTimeoutKey]; ok {
			drainAt, err := time.Parse(time.RFC3339, drainAtStr)
			if err != nil {
				return err
			}

			if drainAt.After(time.Now()) {
				// drain the node

			}
		} else {
			createdAt, err := time.Parse(time.RFC3339, instance.CreationTimestamp)
			if err != nil {
				return err
			}

			//TODO: compare drainAt with current time and always choose future date
			duration := time.Duration(rand.Intn(12)+12) * time.Hour
			drainAt := createdAt.Add(duration)
			instance.Labels[nodeDrainTimeoutKey] = drainAt.Format(time.RFC3339)

			if _, err := np.GCEService.Instances.SetLabels(project, zone, name, &gce.InstancesSetLabelsRequest{
				Labels: instance.Labels,
			}).Do(); err != nil {
				return fmt.Errorf("Setting instance labels: %v", err)
			}
		}
	}

	return nil
}
