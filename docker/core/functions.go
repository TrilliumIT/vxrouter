package core

import (
	"fmt"
	"net"
	"strings"

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
