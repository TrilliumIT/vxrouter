package main

import (
	"context"
	"fmt"
	"net/http"
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
	version   = "0.1"
	envPrefix = "VXR_"
)

func main() {
	app := cli.NewApp()
	app.Name = "docker-vxlan-plugin"
	app.Usage = "Docker vxLan Networking"
	app.Version = version

	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:   "debug, d",
			Usage:  "Enable debugging.",
			EnvVar: envPrefix + "DEBUG_LOGGING",
		},
		cli.StringFlag{
			Name:   "network-scope, ns",
			Value:  "local",
			Usage:  "Scope of the network. local or global.",
			EnvVar: envPrefix + "NETWORK-SCOPE",
		},
		cli.DurationFlag{
			Name:   "ipam-prop-timeout, pt",
			Value:  100 * time.Millisecond,
			Usage:  "How long to wait for external route propagation",
			EnvVar: envPrefix + "IPAM-PROP-TIMEOUT",
		},
		cli.DurationFlag{
			Name:   "ipam-resp-timeout, rt",
			Value:  10 * time.Second,
			Usage:  "Maximum allowed response milliseconds, to prevent hanging docker daemon",
			EnvVar: envPrefix + "IPAM-RESP-TIMEOUT",
		},
		cli.IntFlag{
			Name:   "ipam-exclude-first, xf",
			Value:  0,
			Usage:  "Exclude the first n addresses from each pool from being provided as random addresses",
			EnvVar: envPrefix + "IPAM-EXCLUDE-FIRST",
		},
		cli.IntFlag{
			Name:   "ipam-exclude-last, xl",
			Value:  0,
			Usage:  "Exclude the last n addresses from each pool from being provided as random addresses",
			EnvVar: envPrefix + "IPAM-EXCLUDE-LAST",
		},
	}
	app.Action = Run
	err := app.Run(os.Args)
	if err != nil {
		log.WithError(err).Fatal("error running app")
	}
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

	dc, err := client.NewEnvClient()
	if err != nil {
		log.WithError(err).Fatal("failed to create docker client")
	}

	nd, err := vxrNet.NewDriver(ns, dc)
	if err != nil {
		log.WithError(err).Fatal("failed to create vxrNet driver")
	}
	id, err := vxrIpam.NewDriver(nd, pt, rt, xf, xl)
	if err != nil {
		log.WithError(err).Fatal("failed to create vxrIpam driver")
	}
	ncerr := make(chan error)
	icerr := make(chan error)

	nh := network.NewHandler(nd)
	ih := ipam.NewHandler(id)
	go func() { ncerr <- nh.ServeUnix("vxrNet", 0) }()
	go func() { icerr <- ih.ServeUnix("vxrIpam", 0) }()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	select {
	case err = <-ncerr:
		log.WithError(err).Error("error from vxrNet driver")
		close(ncerr)
	case err = <-icerr:
		log.WithError(err).Error("error from vxrIpam driver")
		close(icerr)
	case <-c:
	}

	err = ih.Shutdown(context.Background())
	if err != nil {
		log.WithError(err).Error("Error shutting down vxrIpam driver")
	}
	err = nh.Shutdown(context.Background())
	if err != nil {
		log.WithError(err).Error("Error shutting down vxrNet driver")
	}

	err = <-icerr
	if err != nil && err != http.ErrServerClosed {
		log.WithError(err).Error("error from vxrIpam driver")
	}

	err = <-ncerr
	if err != nil && err != http.ErrServerClosed {
		log.WithError(err).Error("error from vxrNet driver")
	}

	fmt.Println()
	fmt.Println("tetelestai")
}
