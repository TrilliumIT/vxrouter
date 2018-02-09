package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/client"
	"github.com/docker/go-plugins-helpers/ipam"
	"github.com/docker/go-plugins-helpers/network"
	"github.com/urfave/cli"

	"github.com/TrilliumIT/docker-vxrouter/vxrIpam"
	"github.com/TrilliumIT/docker-vxrouter/vxrNet"
)

const (
	version = "0.1"
	//I know this isn't ideal, but it is hard coded in docker-plugins-helpers too
	//so if you change it there, you'll have to change it here too
	sockdir = "/run/docker/plugins/"
)

func main() {
	app := cli.NewApp()
	app.Name = "docker-vxlan-plugin"
	app.Usage = "Docker vxLan Networking"
	app.Version = version

	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "debug, d",
			Usage: "Enable debugging.",
		},
		cli.StringFlag{
			Name:  "network-scope, ns",
			Value: "local",
			Usage: "Scope of the network. local or global.",
		},
		cli.DurationFlag{
			Name:  "ipam-prop-timeout, pt",
			Value: 100 * time.Millisecond,
			Usage: "How long to wait for external route propagation",
		},
		cli.DurationFlag{
			Name:  "ipam-resp-timeout, rt",
			Value: 10 * time.Second,
			Usage: "Maximum allowed response milliseconds, to prevent hanging docker daemon",
		},
		cli.IntFlag{
			Name:  "ipam-exclude-first, xf",
			Value: 0,
			Usage: "Exclude the first n addresses from each pool from being provided as random addresses",
		},
		cli.IntFlag{
			Name:  "ipam-exclude-last, xl",
			Value: 0,
			Usage: "Exclude the last n addresses from each pool from being provided as random addresses",
		},
	}
	app.Action = Run
	app.Run(os.Args)
}

// Run initializes the driver
func Run(ctx *cli.Context) {
	if ctx.Bool("debug") {
		log.SetLevel(log.DebugLevel)
	}
	log.SetFormatter(&log.TextFormatter{
		ForceColors:      false,
		DisableColors:    true,
		DisableTimestamp: false,
		FullTimestamp:    true,
	})

	ns := ctx.String("ns")
	pt := ctx.Duration("pt")
	rt := ctx.Duration("rt")
	xf := ctx.Int("xf")
	xl := ctx.Int("xl")

	defaultHeaders := map[string]string{"User-Agent": "engine-api-cli-1.0"}
	dc, err := client.NewClient("unix:///var/run/docker.sock", "v1.23", nil, defaultHeaders)
	if err != nil {
		log.WithError(err).Fatal("failed to create docker client")
	}

	nd, err := vxrNet.NewDriver(ns, dc)
	if err != nil {
		log.WithError(err).Fatal("failed to create vxrNet driver")
	}
	id, err := vxrIpam.NewDriver(pt, rt, xf, xl)
	if err != nil {
		log.WithError(err).Fatal("failed to create vxrIpam driver")
	}
	ncerr := make(chan error)
	icerr := make(chan error)

	go func() { ncerr <- network.NewHandler(nd).ServeUnix("vxrNet", 0) }()
	go func() { icerr <- ipam.NewHandler(id).ServeUnix("vxrIpam", 0) }()

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-ncerr:
		log.WithError(err).Error("error from vxrNet driver")
	case err := <-icerr:
		log.WithError(err).Error("error from vxrIpam driver")
	case <-c:
		//have to manually clean up sock files because
		//go-plugins-helpers doesn't give us a shutdown
		//method, and running the handlers in a go routine
		//prevents the defers from running
		//also: see note about hardcoded path in `const` block
		err := os.Remove(sockdir + "vxrNet.sock")
		if err != nil {
			log.WithError(err).Errorf("failed to delete socket file: %v", sockdir+"vxrNet.sock")
		}
		err = os.Remove(sockdir + "vxrIpam.sock")
		if err != nil {
			log.WithError(err).Errorf("failed to delete socket file: %v", sockdir+"vxrIpam.sock")
		}
	}

	fmt.Println()
	fmt.Println("tetelestai")
}
