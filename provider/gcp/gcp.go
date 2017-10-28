package gcp

import (
	"context"

	"github.com/dinesh/spotter/provider/kube"
	"github.com/rs/zerolog/log"

	gce "google.golang.org/api/compute/v1"
	gke "google.golang.org/api/container/v1"
)

const (
	nodeDrainTimeoutKey = "spotter-drain-at"
)

// NewManager initiates GKEManager with required services
func NewManager(zone, clusterName, keyPath, kubeConfigPath string) (*Manager, error) {
	ctx := context.TODO()
	projectID, err := getProjectID(keyPath)
	if err != nil {
		return nil, err
	}

	hc, err := newHTTPClient(ctx, keyPath)
	if err != nil {
		return nil, err
	}

	manager := &Manager{
		GKERef: GKERef{
			ProjectID:   projectID,
			Zone:        zone,
			ClusterName: clusterName,
		},
	}

	if err != nil {
		return nil, err
	}

	gkeService, err := gke.New(hc)
	if err != nil {
		return nil, err
	}
	manager.GKEService = gkeService

	gceService, err := gce.New(hc)
	if err != nil {
		return nil, err
	}
	manager.GCEService = gceService

	if manager.KubeService, err = kube.NewService(kubeConfigPath); err != nil {
		if !debugMode {
			return nil, err
		}
		log.Warn().Err(err).Msg("ignoring because of debugMode")
	}

	return manager, nil
}
