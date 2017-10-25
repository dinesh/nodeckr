package gcp

import (
	"context"

	gce "google.golang.org/api/compute/v1"
	gke "google.golang.org/api/container/v1"
)

const (
	nodeDrainTimeoutKey = "spotter/drain-at"
)

// NewManager initiates GKEManager with required services
func NewManager(zone, clusterName, keyPath, kubeConfigPath string) (*GKEManager, error) {
	ctx := context.TODO()
	projectID, err := getProjectID(keyPath)
	if err != nil {
		return nil, err
	}

	hc, err := newHTTPClient(ctx, keyPath)
	if err != nil {
		return nil, err
	}

	manager := &GKEManager{
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

	return manager, nil
}
