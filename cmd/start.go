package cmd

import (
	"github.com/codegangsta/cli"
	"github.com/dinesh/spotter/provider/gcp"
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

	manager.Monitor()

	return nil
}
