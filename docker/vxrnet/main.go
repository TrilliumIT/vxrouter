package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coreos/go-systemd/activation"
	gphipam "github.com/docker/go-plugins-helpers/ipam"
	gphnet "github.com/docker/go-plugins-helpers/network"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	"github.com/TrilliumIT/vxrouter"
	"github.com/TrilliumIT/vxrouter/docker/core"
	"github.com/TrilliumIT/vxrouter/docker/ipam"
	"github.com/TrilliumIT/vxrouter/docker/network"
)

const (
	version         = vxrouter.Version
	envPrefix       = vxrouter.EnvPrefix
	shutdownTimeout = 10 * time.Second
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
			EnvVar: envPrefix + "NETWORK_SCOPE",
		},
		cli.DurationFlag{
			Name:   "prop-timeout, pt",
			Value:  100 * time.Millisecond,
			Usage:  "How long to wait for external route propagation",
			EnvVar: envPrefix + "PROP_TIMEOUT",
		},
		cli.DurationFlag{
			Name:   "resp-timeout, rt",
			Value:  10 * time.Second,
			Usage:  "Maximum allowed response milliseconds, to prevent hanging docker daemon",
			EnvVar: envPrefix + "RESP_TIMEOUT",
		},
		cli.DurationFlag{
			Name:   "reconcile-interval, ri",
			Value:  30 * time.Second,
			Usage:  "Interval for running periodic reconcile of routes and containers. 0 to disable",
			EnvVar: envPrefix + "RECONCILE_INTERVAL",
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

	core, err := core.New(pt, rt)
	if err != nil {
		log.WithError(err).Fatal("failed to create docker core")
	}

	go func(ri time.Duration) {
		core.Reconcile()
		if ri <= 0 {
			return
		}
		t := time.NewTicker(ri)
		for {
			<-t.C
			core.Reconcile()
		}
	}(ctx.Duration("reconcile-interval"))

	nd, err := network.NewDriver(ns, core)
	if err != nil {
		log.WithField("driver", network.DriverName).WithError(err).Fatal("failed to create driver")
	}
	ncerr := make(chan error)

	id, err := ipam.NewDriver(core)
	if err != nil {
		log.WithField("driver", ipam.DriverName).WithError(err).Fatal("failed to create driver")
	}
	icerr := make(chan error)

	nh := gphnet.NewHandler(nd)

	ih := gphipam.NewHandler(id)

	listeners, _ := activation.Listeners() // wtf coreos, this funciton never returns errors
	if len(listeners) == 0 {
		log.Debug("launching network handler with default listener")
		go func() { ncerr <- nh.ServeUnix(network.DriverName, 0) }()
		log.Debug("launching ipam handler with default listener")
		go func() { icerr <- ih.ServeUnix(ipam.DriverName, 0) }()
	} else if len(listeners) == 2 {
		nl := listeners[0]
		log.WithField("listener", nl.Addr().String()).Debug("launching network handler")
		go func() { ncerr <- nh.Serve(nl) }()
		il := listeners[1]
		log.WithField("listener", il.Addr().String()).Debug("launching ipam handler")
		go func() { icerr <- ih.Serve(il) }()
	} else {
		log.Fatal("exactly two sockets are required for socket activation")
	}

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

	nhCtx, nhCtxCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer nhCtxCancel()
	err = nh.Shutdown(nhCtx)
	if err != nil {
		log.WithField("driver", network.DriverName).WithError(err).Error("error shutting down driver")
	}

	ihCtx, ihCtxCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer ihCtxCancel()
	err = ih.Shutdown(ihCtx)
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
