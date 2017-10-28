package cmd

import (
	"time"

	"github.com/codegangsta/cli"
	"github.com/dinesh/nodeckr/provider/gcp"
)

var (
	doneC = make(chan struct{}, 1)
)

//Start initiates the manager and pool status of nodes to save you bucks.
func Start(c *cli.Context) error {
	clusterName := c.String("name")
	keyPath := c.String("key")
	zone := c.String("zone")
	kubeConfigPath := c.String("kubeconfig")
	interval := c.Int("interval")
	checkRequiredFlags(c, "name", "key", "zone", "kubeconfig")

	manager, err := gcp.NewManager(zone, clusterName, keyPath, kubeConfigPath)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(time.Minute * time.Duration(interval))

	for {
		select {
		case <-doneC:
			return nil
		case <-ticker.C:
			manager.Monitor()
		}
	}

}
