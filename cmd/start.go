package cmd

import (
	"time"

	"github.com/codegangsta/cli"
	"github.com/dinesh/spotter/provider/gcp"
)

var (
	doneC = make(chan struct{}, 1)
)

func Start(c *cli.Context) error {
	clusterName := c.String("name")
	keyPath := c.String("key")
	zone := c.String("zone")
	kubeConfigPath := c.String("kubeconfig")
	checkRequiredFlags(c, "name", "key", "zone", "kubeconfig")

	manager, err := gcp.NewManager(zone, clusterName, keyPath, kubeConfigPath)
	if err != nil {
		return err
	}

	if err := manager.FetchNodePools(); err != nil {
		return err
	}

	// TODO: make it random; jitter
	ticker := time.NewTicker(time.Minute * 1)

	for {
		select {
		case <-doneC:
			return nil
		case <-ticker.C:
			manager.Monitor()
		}
	}

	return nil
}
