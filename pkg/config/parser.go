/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/gogo/protobuf/jsonpb"
	"mosn.io/mosn/pkg/api/v2"
	"mosn.io/mosn/pkg/filter"
	"mosn.io/mosn/pkg/log"
	"mosn.io/mosn/pkg/protocol"
	"mosn.io/mosn/pkg/types"
)

var protocolsSupported = map[string]bool{
	string(protocol.Auto):      true,
	string(protocol.SofaRPC):   true,
	string(protocol.HTTP1):     true,
	string(protocol.HTTP2):     true,
	string(protocol.Xprotocol): true,
}

const (
	MinHostWeight               = uint32(1)
	MaxHostWeight               = uint32(128)
	DefaultMaxRequestPerConn    = uint32(1024)
	DefaultConnBufferLimitBytes = uint32(16 * 1024)
)

// RegisterProtocolParser
// used to register parser
func RegisterProtocolParser(key string) bool {
	if _, ok := protocolsSupported[key]; ok {
		return false
	}
	log.StartLogger.Infof("[config] %s added to protocolsSupported", key)
	protocolsSupported[key] = true
	return true
}

// ParsedCallback is an
// alias for closure func(data interface{}, endParsing bool) error
type ParsedCallback func(data interface{}, endParsing bool) error

var configParsedCBMaps = make(map[ContentKey][]ParsedCallback)

// Group of ContentKey
// notes: configcontentkey equals to the key of config file
const (
	ParseCallbackKeyCluster        ContentKey = "clusters"
	ParseCallbackKeyServiceRgtInfo ContentKey = "service_registry"
	ParseCallbackKeyProcessor      ContentKey = "processor"
)

// RegisterConfigParsedListener
// used to register ParsedCallback
func RegisterConfigParsedListener(key ContentKey, cb ParsedCallback) {
	if cbs, ok := configParsedCBMaps[key]; ok {
		cbs = append(cbs, cb)
		// append maybe change the slice, should be assigned again
		configParsedCBMaps[key] = cbs
	} else {
		log.StartLogger.Infof("[config] %s added to configParsedCBMaps", key)
		cpc := []ParsedCallback{cb}
		configParsedCBMaps[key] = cpc
	}
}

// ParseClusterConfig parses config data to api data, verify whether the config is valid
func ParseClusterConfig(clusters []v2.Cluster) ([]v2.Cluster, map[string][]v2.Host) {
	if len(clusters) == 0 {
		log.StartLogger.Warnf("[config] [parse cluster] No Cluster provided in cluster config")
	}
	var pClusters []v2.Cluster
	clusterV2Map := make(map[string][]v2.Host)
	for _, c := range clusters {
		if c.Name == "" {
			log.StartLogger.Fatalln("[config] [parse cluster] name is required in cluster config")
		}
		if c.MaxRequestPerConn == 0 {
			c.MaxRequestPerConn = DefaultMaxRequestPerConn
			log.StartLogger.Infof("[config] [parse cluster] max_request_per_conn is not specified, use default value %d",
				DefaultMaxRequestPerConn)
		}
		if c.ConnBufferLimitBytes == 0 {
			c.ConnBufferLimitBytes = DefaultConnBufferLimitBytes
			log.StartLogger.Infof("[config] [parse cluster] conn_buffer_limit_bytes is not specified, use default value %d",
				DefaultConnBufferLimitBytes)
		}
		if c.LBSubSetConfig.FallBackPolicy > 2 {
			log.StartLogger.Fatalln("[config] [parse cluster] lb subset config 's fall back policy set error. ",
				"For 0, represent NO_FALLBACK",
				"For 1, represent ANY_ENDPOINT",
				"For 2, represent DEFAULT_SUBSET")
		}
		if _, ok := protocolsSupported[c.HealthCheck.Protocol]; !ok && c.HealthCheck.Protocol != "" {
			log.StartLogger.Fatalf("[config] [parse cluster] unsupported health check protocol: %v", c.HealthCheck.Protocol)
		}
		c.Hosts = parseHostConfig(c.Hosts)
		clusterV2Map[c.Name] = c.Hosts
		pClusters = append(pClusters, c)
	}
	// trigger all callbacks
	if cbs, ok := configParsedCBMaps[ParseCallbackKeyCluster]; ok {
		for _, cb := range cbs {
			cb(pClusters, false)
		}
	}

	return pClusters, clusterV2Map
}

func parseHostConfig(hosts []v2.Host) (hs []v2.Host) {
	for _, host := range hosts {
		host.Weight = transHostWeight(host.Weight)
		hs = append(hs, host)
	}
	return
}

func transHostWeight(weight uint32) uint32 {
	if weight > MaxHostWeight {
		return MaxHostWeight
	}
	if weight < MinHostWeight {
		return MinHostWeight
	}
	return weight
}

var logLevelMap = map[string]log.Level{
	"TRACE": log.TRACE,
	"DEBUG": log.DEBUG,
	"FATAL": log.FATAL,
	"ERROR": log.ERROR,
	"WARN":  log.WARN,
	"INFO":  log.INFO,
}

func ParseLogLevel(level string) log.Level {
	if logLevel, ok := logLevelMap[level]; ok {
		return logLevel
	}
	return log.INFO
}

// ParseListenerConfig
func ParseListenerConfig(lc *v2.Listener, inheritListeners []net.Listener) *v2.Listener {
	if lc.AddrConfig == "" {
		log.StartLogger.Fatalln("[config] [parse listener] Address is required in listener config")
	}
	addr, err := net.ResolveTCPAddr("tcp", lc.AddrConfig)
	if err != nil {
		log.StartLogger.Fatalln("[config] [parse listener] Address not valid:", lc.AddrConfig)
	}
	//try inherit legacy listener
	var old *net.TCPListener

	for i, il := range inheritListeners {
		if il == nil {
			continue
		}
		tl := il.(*net.TCPListener)
		ilAddr, err := net.ResolveTCPAddr("tcp", tl.Addr().String())
		if err != nil {
			log.StartLogger.Fatalln("[config] [parse listener] inheritListener not valid:", tl.Addr().String())
		}

		if addr.Port != ilAddr.Port {
			continue
		}

		if (addr.IP.IsUnspecified() && ilAddr.IP.IsUnspecified()) ||
			(addr.IP.IsLoopback() && ilAddr.IP.IsLoopback()) ||
			addr.IP.Equal(ilAddr.IP) {
			log.StartLogger.Infof("[config] [parse listener] inherit listener addr: %s", lc.AddrConfig)
			old = tl
			inheritListeners[i] = nil
			break
		}
	}

	lc.Addr = addr
	lc.PerConnBufferLimitBytes = 1 << 15
	lc.InheritListener = old
	return lc
}

// ParseRouterConfiguration used to get virtualhosts from filter
func ParseRouterConfiguration(c *v2.FilterChain) *v2.RouterConfiguration {
	routerConfiguration := &v2.RouterConfiguration{}
	for _, f := range c.Filters {
		if f.Type == v2.CONNECTION_MANAGER {

			if data, err := json.Marshal(f.Config); err == nil {
				if err := json.Unmarshal(data, routerConfiguration); err != nil {
					log.StartLogger.Fatal("[config] [parse router] Parsing Virtual Host Error:", err)
				}
			} else {
				log.StartLogger.Fatal("[config] [parse router] Parsing Virtual Host Error")
			}
		}
	}

	return routerConfiguration
}

// ParseProxyFilter
func ParseProxyFilter(cfg map[string]interface{}) *v2.Proxy {
	proxyConfig := &v2.Proxy{}
	if data, err := json.Marshal(cfg); err == nil {
		json.Unmarshal(data, proxyConfig)
	} else {
		log.StartLogger.Fatal("[config] [parse proxy] Parsing Proxy Network Filter Error")
	}

	if proxyConfig.DownstreamProtocol == "" || proxyConfig.UpstreamProtocol == "" {
		log.StartLogger.Fatal("[config] [parse proxy] Protocol in String Needed in Proxy Network Filter")
	} else if _, ok := protocolsSupported[proxyConfig.DownstreamProtocol]; !ok {
		log.StartLogger.Fatal("[config] [parse proxy] Invalid Downstream Protocol = ", proxyConfig.DownstreamProtocol)
	} else if _, ok := protocolsSupported[proxyConfig.UpstreamProtocol]; !ok {
		log.StartLogger.Fatal("[config] [parse proxy] Invalid Upstream Protocol = ", proxyConfig.UpstreamProtocol)
	}

	return proxyConfig
}

// ParseFaultInjectFilter
func ParseFaultInjectFilter(cfg map[string]interface{}) *v2.FaultInject {
	filterConfig := &v2.FaultInject{}
	if data, err := json.Marshal(cfg); err == nil {
		json.Unmarshal(data, filterConfig)
	} else {
		log.StartLogger.Fatal("[config] parsing fault inject filter error")
	}
	return filterConfig
}

// ParseStreamPayloadLimitFilter
func ParseStreamPayloadLimitFilter(cfg map[string]interface{}) (*v2.StreamPayloadLimit, error) {
	filterConfig := &v2.StreamPayloadLimit{}
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, filterConfig); err != nil {
		return nil, err
	}
	return filterConfig, nil
}

// ParseStreamFaultInjectFilter
func ParseStreamFaultInjectFilter(cfg map[string]interface{}) (*v2.StreamFaultInject, error) {
	filterConfig := &v2.StreamFaultInject{}
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, filterConfig); err != nil {
		return nil, err
	}
	return filterConfig, nil
}

// ParseMixerFilter
func ParseMixerFilter(cfg map[string]interface{}) *v2.Mixer {
	mixerFilter := &v2.Mixer{}

	data, err := json.Marshal(cfg)
	if err != nil {
		log.StartLogger.Errorf("[config] parsing mixer filter error, err: %v, cfg: %v", err, cfg)
		return nil
	}

	var un jsonpb.Unmarshaler
	err = un.Unmarshal(strings.NewReader(string(data)), &mixerFilter.HttpClientConfig)
	if err != nil {
		log.StartLogger.Errorf("[config] parsing mixer filter error, err: %v, cfg: %v", err, cfg)
		return nil
	}

	return mixerFilter
}

// ParseHealthCheckFilter
func ParseHealthCheckFilter(cfg map[string]interface{}) *v2.HealthCheckFilter {
	filterConfig := &v2.HealthCheckFilter{}
	if data, err := json.Marshal(cfg); err == nil {
		json.Unmarshal(data, filterConfig)
	} else {
		log.StartLogger.Fatalln("[config] parsing health check filter failed")
	}
	return filterConfig
}

// ParseTCPProxy
func ParseTCPProxy(cfg map[string]interface{}) (*v2.TCPProxy, error) {
	proxy := &v2.TCPProxy{}
	if data, err := json.Marshal(cfg); err == nil {
		json.Unmarshal(data, proxy)
	} else {
		return nil, fmt.Errorf("[config] config is not a tcp proxy config: %v", err)
	}
	return proxy, nil
}

func ParseServiceRegistry(src v2.ServiceRegistryInfo) {
	//trigger all callbacks
	if cbs, ok := configParsedCBMaps[ParseCallbackKeyServiceRgtInfo]; ok {
		for _, cb := range cbs {
			cb(src, true)
		}
	}
}

// ParseServerConfig
func ParseServerConfig(c *v2.ServerConfig) *v2.ServerConfig {
	if n, _ := strconv.Atoi(os.Getenv("GOMAXPROCS")); n > 0 && n <= runtime.NumCPU() {
		c.Processor = n
	} else if c.Processor == 0 {
		c.Processor = runtime.NumCPU()
	}

	// trigger processor callbacks
	if cbs, ok := configParsedCBMaps[ParseCallbackKeyProcessor]; ok {
		for _, cb := range cbs {
			cb(c.Processor, true)
		}
	}
	return c
}

// GetStreamFilters returns a stream filter factory by filter.Type
func GetStreamFilters(configs []v2.Filter) []types.StreamFilterChainFactory {
	var factories []types.StreamFilterChainFactory

	for _, c := range configs {
		sfcc, err := filter.CreateStreamFilterChainFactory(c.Type, c.Config)
		if err != nil {
			log.DefaultLogger.Errorf("[config] get stream filter failed, type: %s, error: %v", c.Type, err)
			continue
		}
		factories = append(factories, sfcc)
	}

	return factories
}

// GetNetworkFilters returns a network filter factory by filter.Type
func GetNetworkFilters(c *v2.FilterChain) []types.NetworkFilterChainFactory {
	var factories []types.NetworkFilterChainFactory
	for _, f := range c.Filters {
		factory, err := filter.CreateNetworkFilterChainFactory(f.Type, f.Config)
		if err != nil {
			log.StartLogger.Errorf("[config] network filter create failed, type:%s, error: %v", f.Type, err)
			continue
		}
		factories = append(factories, factory)
	}
	return factories
}
