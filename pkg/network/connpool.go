package network

import (
	"mosn.io/mosn/pkg/log"
	"mosn.io/mosn/pkg/types"
)

func init() {
	ConnNewPoolFactories = make(map[types.Protocol]connNewPool)
}

type connNewPool func(host types.Host) types.ConnectionPool

var ConnNewPoolFactories map[types.Protocol]connNewPool

func RegisterNewPoolFactory(protocol types.Protocol, factory connNewPool) {
	//other
	log.DefaultLogger.Infof("[network] [ register pool factory] register protocol: %v factory", protocol)
	ConnNewPoolFactories[protocol] = factory
}
