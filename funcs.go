package vxrouter

import (
	"os"
	"strconv"
	"time"

	log "github.com/Sirupsen/logrus"
)

func getEnvOpt(val, opt string) string { //nolint: unparam
	e := os.Getenv(val)
	if e == "" {
		e = opt
	}
	return e
}

// GetEnvIntWithDefault gets value, prioritizing first opt, if it is not empty, then the environment variable specified by val, and lastly the default.
func GetEnvIntWithDefault(val, opt string, def int) int { //nolint: unparam
	e := getEnvOpt(val, opt)
	if e == "" {
		return def
	}
	ei, err := strconv.Atoi(e)
	if err != nil {
		log.WithField("string", e).WithError(err).Warnf("failed to convert string to int, using default")
		return def
	}
	return ei
}

// GetEnvDurWithDefault gets value, prioritizing first opt, if it is not empty, then the environment variable specified by val, and lastly the default.
func GetEnvDurWithDefault(val, opt string, def time.Duration) time.Duration { //nolint: unparam
	e := getEnvOpt(val, opt)
	if e == "" {
		return def
	}
	ei, err := time.ParseDuration(e)
	if err != nil {
		log.WithField("string", e).WithError(err).Warnf("failed to convert string to duration, using default")
		return def
	}
	return ei
}
