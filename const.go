package vxrouter

import (
	"time"
)

// useful constants for the whole project
const (
	Version                 = "0.0.10"
	EnvPrefix               = "VXR_"
	NetworkDriver           = "vxrNet"
	IpamDriver              = "vxrIpam"
	DefaultReqAddrSleepTime = 100 * time.Millisecond
	DefaultRouteProto       = 192
)
