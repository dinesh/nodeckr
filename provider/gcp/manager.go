package gcp

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"sync"
	"time"

	kube "github.com/dinesh/nodeckr/kubernetes"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	gce "google.golang.org/api/compute/v1"
	gke "google.golang.org/api/container/v1"
)

var debugMode bool

func init() {
	debugMode = os.Getenv("SPOTTER_DEBUG") == "1"
}

// GKERef contains reference to any GCP resource having project, zone
type GKERef struct {
	ProjectID    string
	Zone         string
	ResourceName string
}

// Manager discovers preemptible nodes and tries to maintain capacity constant
// when they are about to die.
type Manager struct {
	GKEService  *gke.Service
	GCEService  *gce.Service
	KubeService *kube.KubeService

	// a map of instance to nodePool
	Instances []string

	GKERef
}

func (gm *Manager) fetchPreemptibleNodes() error {
	nodePoolService := gke.NewProjectsZonesClustersNodePoolsService(gm.GKEService)
	resp, err := nodePoolService.List(gm.ProjectID, gm.Zone, gm.ResourceName).Do()
	if err != nil {
		return fmt.Errorf("fetching nodePool list: %v", err)
	}

	log.Printf("Number of nodepools: %d\n", len(resp.NodePools))
	var preemptibleNodes []string

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

			log.Printf("NodePool %s have %d preemptible nodes in %s group\n",
				pool.Name, len(instances), igmName)

			preemptibleNodes = append(preemptibleNodes, instances...)
		}
	}

	// select uniq nodes
	var existed map[string]bool
	gm.Instances = []string{}
	for _, i := range preemptibleNodes {
		if _, ok := existed[i]; ok {
			gm.Instances = append(gm.Instances, i)
		}
	}

	return nil
}

// Monitor pools the status of nodes
func (gm *Manager) Monitor() {
	gm.fetchPreemptibleNodes()

	var wg sync.WaitGroup
	wg.Add(len(gm.Instances))

	for _, instance := range gm.Instances {
		go func(instance string) {
			defer wg.Done()

			if err := gm.processNode(instance); err != nil {
				log.Warn().Err(err).Str("instaceURL", instance)
			}
		}(instance)
	}
	wg.Wait()
}

func (gm *Manager) processNode(instanceURL string) error {
	project, zone, name, err := parseGceURL(instanceURL, "instances")
	instance, err := gm.GCEService.Instances.Get(project, zone, name).Do()
	if err != nil {
		return err
	}

	log.Info().Str("host", name).Str("status", instance.Status)
	now := time.Now()

	if instance.Status == "RUNNING" {
		if drainAtStr, ok := instance.Labels[nodeDrainTimeoutKey]; ok {
			drainAt, err := parseTimestamp(drainAtStr)
			if err != nil {
				return err
			}

			if drainAt.Sub(now).Minutes() < 0 {
				gm.expireNode(name)
			} else {
				difference := drainAt.Sub(now).Hours()
				unit := "Hrs"
				if difference < 1 {
					difference = drainAt.Sub(now).Minutes()
					unit = "Min"
				}
				log.Info().Str("node", name).
					Msgf("ignoring because of healthy drain period: %.2f %s", difference, unit)
			}

		} else {
			updateDrainingInstanceLabel(name, project, zone, instance, gm.GCEService)
		}
	} else {
		log.Info().Str("node", name).Str("status", instance.Status).Msg("ignoring node because of status")
	}

	return nil
}

func (gm *Manager) expireNode(name string) error {
	log.Info().Str("node", name).Msg("expiring node")

	if gm.KubeService != nil {
		if err := gm.KubeService.SetUnschedulableState(name, true); err != nil {
			return err
		}

		if err := gm.KubeService.DrainNode(name); err != nil {
			return err
		}
	}

	log.Info().Str("name", name).Msg("deleting node")
	op, err := gm.GCEService.Instances.Delete(gm.ProjectID, gm.Zone, name).Do()
	if err != nil {
		return fmt.Errorf("deleting node: %v", err)
	}

	return waitForOperation(gm.GCEService, gm.ProjectID, gm.Zone, op)
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
		log.Debug().Str("node", host).Str("duration", fmt.Sprintf("%dM", drainDuration/time.Minute)).Msg("setting draining period")
	} else {
		preemptDeadlineAt := createdAt.Add(24 * time.Hour)
		spanHours := int(math.Floor(preemptDeadlineAt.Sub(now).Hours() / 3))

		// set the draining time between between 1/3 and 2/3 until 24 hours of creationTime
		drainDuration = time.Duration(spanHours+rand.Intn(spanHours)) * time.Hour
		log.Debug().Str("node", host).Str("duration", fmt.Sprintf("%dH", drainDuration/time.Hour)).Msg("setting draining period")
	}
	drainAt := now.Add(drainDuration)

	labels := make(map[string]string)
	labelsForLog := make(map[string]interface{})
	for k, v := range instance.Labels {
		labels[k] = v
	}

	labels[nodeDrainTimeoutKey] = fmt.Sprintf("%d", makeTimestamp(drainAt))

	for k, v := range labels {
		labelsForLog[k] = v
	}

	log.Debug().Str("node", host).
		Dict("labels", zerolog.Dict().Fields(labelsForLog)).
		Msg("setting labels")

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
		log.Fatal().Err(jerr).
			Str("operation", op.Name).
			Msg("can't unmarshal operation error")
	}

	return fmt.Errorf("%s[%s] failed: %v", op.Name, op.OperationType, string(errbody))
}
