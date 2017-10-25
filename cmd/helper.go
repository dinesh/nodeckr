package cmd

import (
	"fmt"

	"github.com/codegangsta/cli"
)

func checkRequiredFlags(c *cli.Context, flags ...string) {
	for _, key := range flags {
		if c.String(key) == "" {
			fmt.Printf("Missing required param: --%s\n", key)
			cli.ShowAppHelpAndExit(c, 1)
		}
	}
}
