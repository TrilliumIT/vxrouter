package core

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
)

func poolFromNR(nr *types.NetworkResource) (string, error) {
	for _, c := range nr.IPAM.Config {
		if c.Subnet != "" {
			return c.Subnet, nil
		}
	}
	return "", fmt.Errorf("pool not found")
}

func getEnvIntWithDefault(val, opt string, def int) int { //nolint: unparam
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

func poolFromID(poolid string) string {
	return strings.TrimPrefix(poolid, ipamDriverName+"/")
}

// IPNetFromReqInfo returns an an IPNet from an ipam request
func IPNetFromReqInfo(poolid, reqAddr string) (*net.IPNet, error) {
	_, n, err := net.ParseCIDR(poolFromID(poolid))
	if err != nil {
		return nil, err
	}
	n.IP = net.ParseIP(reqAddr)
	if n.IP == nil {
		return nil, fmt.Errorf("invalid requested address")
	}
	return n, nil
}
