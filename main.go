package main

import (
	"log"
	"os"

	"github.com/codegangsta/cli"
	"github.com/dinesh/spotter/cmd"
)

var (
	version = "undefined"
)

func main() {
	app := cli.NewApp()
	app.Usage = "Manage GKE cluster using preemptible instances"
	app.Version = version
	app.Action = cmd.Start
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:  "name",
			Usage: "Name of GKE cluster",
		},
		&cli.StringFlag{
			Name:  "key",
			Usage: "Path of service account key",
		},
		&cli.StringFlag{
			Name:  "zone",
			Usage: "GCP zone",
		},
		&cli.StringFlag{
			Name:  "kubeconfig",
			Usage: "Path of kubernetes config",
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
