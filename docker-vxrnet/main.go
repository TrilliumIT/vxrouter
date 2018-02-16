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
	"github.com/docker/go-plugins-helpers/network"
	"github.com/urfave/cli"

	"github.com/TrilliumIT/vxrouter/docker-vxrnet/driver"
)

const (
	version   = "0.1"
	envPrefix = "VXR_"
)

func main() {
	app := cli.NewApp()
	app.Name = "docker-" + driver.DriverName
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

	ns := ctx.String("ns")
	pt := ctx.Duration("pt")
	rt := ctx.Duration("rt")

	dc, err := client.NewEnvClient()
	if err != nil {
		log.WithError(err).Fatal("failed to create docker client")
	}

	nd, err := driver.NewDriver(ns, pt, rt, dc)
	if err != nil {
		log.WithError(err).Fatalf("failed to create %v driver", driver.DriverName)
	}
	cerr := make(chan error)

	nh := network.NewHandler(nd)
	go func() { cerr <- nh.ServeUnix(driver.DriverName, 0) }()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	select {
	case err = <-cerr:
		log.WithError(err).Errorf("error from %v driver", driver.DriverName)
		close(cerr)
	case <-c:
	}

	err = nh.Shutdown(context.Background())
	if err != nil {
		log.WithError(err).Errorf("Error shutting down %v driver", driver.DriverName)
	}

	err = <-cerr
	if err != nil && err != http.ErrServerClosed {
		log.WithError(err).Errorf("error from %v driver", driver.DriverName)
	}

	fmt.Println()
	fmt.Println("tetelestai")
}
