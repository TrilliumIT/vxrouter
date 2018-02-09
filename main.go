package main

import (
	"fmt"
	"os"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/go-plugins-helpers/ipam"
	"github.com/docker/go-plugins-helpers/network"
	"github.com/urfave/cli"
)

const (
	version = "0.1"
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
		cli.StringFlag{
			Name:  "network-vtepdev, vd",
			Value: "",
			Usage: "VTEP device (VLAN tunneling end point).",
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
	vd := ctx.String("vd")
	pt := ctx.Duration("pt")
	rt := ctx.Duration("rt")
	xf := ctx.Duration("xf")
	xl := ctx.Duration("xl")

	nd, err := vxrNet.NewDriver()
	if err != nil {
		log.WithError(err).Fatal("failed to create vxrNet driver")
	}
	id, err := vxrIpam.NewDriver()
	if err != nil {
		log.WithError(err).Fatal("failed to create vxrIpam driver")
	}
	ncerr := make(chan error)
	icerr := make(chan error)

	go func() { ncerr <- network.NewHandler(nd).ServeUnix("vxrNet", 0) }()
	go func() { icerr <- ipam.NewHandler(id).ServeUnix("vxrIpam", 0) }()

	select {
	case err := <-ncerr:
		log.WithError(err).Error("error from vxrNet driver")
	case err := <-icerr:
		log.WithError(err).Error("error from vxrIpam driver")
	}

	fmt.Println("tetelestai")
}
