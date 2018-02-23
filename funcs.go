package vxrouter

import (
	"os"
	"strconv"

	log "github.com/Sirupsen/logrus"
)

// GetEnvIntWithDefault gets value, prioritizing first opt, if it is not empty, then the environment variable specified by val, and lastly the default.
func GetEnvIntWithDefault(val, opt string, def int) int { //nolint: unparam
	e := os.Getenv(val)
	if e == "" {
		e = opt
	}
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
