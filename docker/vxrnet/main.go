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
	gphipam "github.com/docker/go-plugins-helpers/ipam"
	gphnet "github.com/docker/go-plugins-helpers/network"
	"github.com/urfave/cli"

	"github.com/TrilliumIT/vxrouter/docker/ipam"
	"github.com/TrilliumIT/vxrouter/docker/network"
)

const (
	version   = "0.1"
	envPrefix = "VXR_"
)

func main() {
	app := cli.NewApp()
	app.Name = "docker-" + network.DriverName
	app.Usage = "Docker vxLan Networking"
	app.Version = version

	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:   "debug, d",
			Usage:  "Enable debugging.",
			EnvVar: envPrefix + "DEBUG_LOGGING",
		},
		cli.StringFlag{
			Name:   "scope, s",
			Value:  "local",
			Usage:  "Scope of the network. local or global.",
			EnvVar: envPrefix + "NETWORK-SCOPE",
		},
		cli.DurationFlag{
			Name:   "prop-timeout, pt",
			Value:  100 * time.Millisecond,
			Usage:  "How long to wait for external route propagation",
			EnvVar: envPrefix + "IPAM-PROP-TIMEOUT",
		},
		cli.DurationFlag{
			Name:   "resp-timeout, rt",
			Value:  10 * time.Second,
			Usage:  "Maximum allowed response milliseconds, to prevent hanging docker daemon",
			EnvVar: envPrefix + "IPAM-RESP-TIMEOUT",
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

	ns := ctx.String("scope")
	pt := ctx.Duration("prop-timeout")
	rt := ctx.Duration("resp-timeout")

	dc, err := client.NewEnvClient()
	if err != nil {
		log.WithError(err).Fatal("failed to create docker client")
	}

	nd, err := network.NewDriver(ns, pt, rt, dc)
	if err != nil {
		log.WithField("driver", network.DriverName).WithError(err).Fatal("failed to create driver")
	}
	ncerr := make(chan error)

	id, err := ipam.NewDriver()
	if err != nil {
		log.WithField("driver", ipam.DriverName).WithError(err).Fatal("failed to create driver")
	}
	icerr := make(chan error)

	nh := gphnet.NewHandler(nd)
	go func() { ncerr <- nh.ServeUnix(network.DriverName, 0) }()

	ih := gphipam.NewHandler(id)
	go func() { icerr <- ih.ServeUnix(ipam.DriverName, 0) }()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	select {
	case err = <-ncerr:
		log.WithField("driver", network.DriverName).WithError(err).Error()
		close(ncerr)
	case err = <-icerr:
		log.WithField("driver", network.DriverName).WithError(err).Error()
		close(icerr)
	case <-c:
	}

	err = nh.Shutdown(context.Background())
	if err != nil {
		log.WithField("driver", network.DriverName).WithError(err).Error("error shutting down driver")
	}

	err = ih.Shutdown(context.Background())
	if err != nil {
		log.WithField("driver", ipam.DriverName).WithError(err).Error("error shutting down driver")
	}

	err = <-ncerr
	if err != nil && err != http.ErrServerClosed {
		log.WithField("driver", network.DriverName).WithError(err).Error()
	}

	err = <-icerr
	if err != nil && err != http.ErrServerClosed {
		log.WithField("driver", ipam.DriverName).WithError(err).Error()
	}

	fmt.Println()
	fmt.Println("tetelestai")
}