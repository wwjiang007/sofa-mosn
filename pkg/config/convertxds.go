package config

import (
	"net"
	"gitlab.alipay-inc.com/afe/mosn/pkg/api/v2"
	xdsapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	xdslistener"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	xdscore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	xdscluster "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	xdsroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	xdsutil "github.com/envoyproxy/go-control-plane/pkg/util"
	xdshttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	xdstcp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
	xdsaccesslog "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	xdsendpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	xdsauth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"fmt"
	"gitlab.alipay-inc.com/afe/mosn/pkg/log"
	"gitlab.alipay-inc.com/afe/mosn/pkg/protocol"
	"github.com/gogo/protobuf/types"
	"github.com/fatih/structs"
	"time"
)

var supportFilter map[string]bool = map[string]bool{
	xdsutil.HTTPConnectionManager: true,
	v2.SOFARPC_INBOUND_FILTER:     true,
	v2.SOFARPC_OUTBOUND_FILTER:    true,
	v2.X_PROTOCOL_FILTER:          true,
}

var httpBaseConfig map[string]bool = map[string]bool{
	xdsutil.HTTPConnectionManager: true,
	v2.SOFARPC_INBOUND_FILTER:     true,
	v2.SOFARPC_OUTBOUND_FILTER:    true,
}

func convertListenerConfig(xdsListener *xdsapi.Listener) *v2.ListenerConfig {
	if !isSupport(xdsListener) {
		return nil
	}
	listenerConfig :=  &v2.ListenerConfig{
		Name:                                    xdsListener.GetName(),
		Addr:                                    convertAddress(&xdsListener.Address),
		BindToPort:                              convertBindToPort(xdsListener.GetDeprecatedV1()),
		PerConnBufferLimitBytes:                 xdsListener.GetPerConnectionBufferLimitBytes().GetValue(),
		HandOffRestoredDestinationConnections:   xdsListener.GetUseOriginalDst().GetValue(),
		AccessLogs:                              convertAccessLogs(xdsListener),
	}
	if listenerConfig.HandOffRestoredDestinationConnections {
		return listenerConfig
	}
	listenerConfig.FilterChains = convertFilterChains(xdsListener.GetFilterChains())
	return listenerConfig
}

func convertClustersConfig(xdsClusters []*xdsapi.Cluster) []*v2.Cluster {
	if xdsClusters == nil {
		return nil
	}
	clusters := make([]*v2.Cluster, 0, len(xdsClusters))
	for _, xdsCluster := range xdsClusters {
		cluster := &v2.Cluster{
			Name:                 xdsCluster.GetName(),
			ClusterType:          convertClusterType(xdsCluster.GetType()),
			LbType:               convertLbPolicy(xdsCluster.GetLbPolicy()),
			LBSubSetConfig:       convertLbSubSetConfig(xdsCluster.GetLbSubsetConfig()),
			MaxRequestPerConn:    xdsCluster.GetMaxRequestsPerConnection().GetValue(),
			ConnBufferLimitBytes: xdsCluster.GetPerConnectionBufferLimitBytes().GetValue(),
			HealthCheck:          convertHealthChecks(xdsCluster.GetHealthChecks()),
			CirBreThresholds:     convertCircuitBreakers(xdsCluster.GetCircuitBreakers()),
			OutlierDetection:     convertOutlierDetection(xdsCluster.GetOutlierDetection()),
			//Hosts:                convertClusterHosts(xdsCluster.GetHosts()),
			Spec:                 convertSpec(xdsCluster),
		}

		clusters = append(clusters, cluster)
	}

	return clusters
}

func convertEndpointsConfig(xdsEndpoint *xdsendpoint.LocalityLbEndpoints) []*v2.Host {
	if xdsEndpoint == nil {
		return nil
	}
	hosts := make([]*v2.Host, 0, len(xdsEndpoint.GetLbEndpoints()))
	for _, xdsHost := range xdsEndpoint.GetLbEndpoints() {
		var address string
		if xdsAddress, ok := xdsHost.GetEndpoint().GetAddress().GetAddress().(*xdscore.Address_SocketAddress); ok {
			if xdsPort, ok := xdsAddress.SocketAddress.GetPortSpecifier().(*xdscore.SocketAddress_PortValue); ok {
				address = fmt.Sprintf("%s:%d", xdsAddress.SocketAddress.GetAddress(), xdsPort.PortValue)
			}else if xdsPort, ok := xdsAddress.SocketAddress.GetPortSpecifier().(*xdscore.SocketAddress_NamedPort); ok {
				address = fmt.Sprintf("%s:%s", xdsAddress.SocketAddress.GetAddress(), xdsPort.NamedPort)
			}else{
				log.DefaultLogger.Warnf("unsupported port type")
				continue
			}

		}else if xdsAddress, ok := xdsHost.GetEndpoint().GetAddress().GetAddress().(*xdscore.Address_Pipe); ok {
			address = xdsAddress.Pipe.GetPath()
		}else{
			log.DefaultLogger.Warnf("unsupported address type")
			continue
		}
		host := v2.Host{
			Address:    address,
			Weight:     xdsHost.GetLoadBalancingWeight().GetValue(),
		}
		hosts = append(hosts, &host)
	}
	return hosts
}

// todo: more filter type support
func isSupport(xdsListener *xdsapi.Listener) bool {
	if xdsListener == nil {
		return false
	}
	if xdsListener.UseOriginalDst.GetValue() {
		return true
	}
	for _, filterChain := range xdsListener.GetFilterChains() {
		for _, filter := range filterChain.GetFilters() {
			if value, ok := supportFilter[filter.GetName()]; !ok || !value {
				return false
			}
		}
	}
	return true
}

func convertBindToPort(xdsDeprecatedV1 *xdsapi.Listener_DeprecatedV1) bool {
	if xdsDeprecatedV1 == nil || xdsDeprecatedV1.GetBindToPort() == nil{
		return true
	}
	return xdsDeprecatedV1.GetBindToPort().GetValue()
}

// todo: more filter config support
func convertAccessLogs(xdsListener *xdsapi.Listener) []v2.AccessLog {
	if xdsListener == nil {
		return nil
	}

	accessLogs := make([]v2.AccessLog, 0)
	for _, xdsFilterChain := range xdsListener.GetFilterChains() {
		for _, xdsFilter := range xdsFilterChain.GetFilters() {
			if value, ok := httpBaseConfig[xdsFilter.GetName()]; ok && value {
				filterConfig := &xdshttp.HttpConnectionManager{}
				xdsutil.StructToMessage(xdsFilter.GetConfig(), filterConfig)
				for _, accConfig := range filterConfig.GetAccessLog() {
					if accConfig.Name == xdsutil.FileAccessLog {
						als := &xdsaccesslog.FileAccessLog{}
						xdsutil.StructToMessage(accConfig.GetConfig(), als)
						accessLog := v2.AccessLog{
							Path:   als.GetPath(),
							Format: als.GetFormat(),
						}
						accessLogs = append(accessLogs, accessLog)
					}
				}
			} else if xdsFilter.GetName() == xdsutil.TCPProxy {
				filterConfig := &xdstcp.TcpProxy{}
				xdsutil.StructToMessage(xdsFilter.GetConfig(), filterConfig)
				for _, accConfig := range filterConfig.GetAccessLog() {
					if accConfig.Name == xdsutil.FileAccessLog {
						als := &xdsaccesslog.FileAccessLog{}
						xdsutil.StructToMessage(accConfig.GetConfig(), als)
						accessLog := v2.AccessLog{
							Path:   als.GetPath(),
							Format: als.GetFormat(),
						}
						accessLogs = append(accessLogs, accessLog)
					}
				}
			}else if xdsFilter.GetName() == v2.X_PROTOCOL_FILTER {
				//TODO
		    }else{
				log.DefaultLogger.Fatalf("unsupport filter config type, filter name: %s", xdsFilter.GetName())
			}
		}
	}
	return accessLogs
}

func convertFilterChains(xdsFilterChains []xdslistener.FilterChain) []v2.FilterChain {
	if xdsFilterChains == nil {
		return nil
	}
	filterChains := make([]v2.FilterChain, 0, len(xdsFilterChains))
	for _, xdsFilterChain := range xdsFilterChains {
		filterChain := v2.FilterChain{
			FilterChainMatch:  xdsFilterChain.GetFilterChainMatch().String(),
			TLS:        convertTLS(xdsFilterChain.GetTlsContext()),
			Filters:           convertFilters(xdsFilterChain.GetFilters()),
		}
		filterChains = append(filterChains, filterChain)
	}
	return filterChains
}

func convertFilters(xdsFilters []xdslistener.Filter) []v2.Filter {
	if xdsFilters == nil {
		return nil
	}
	filters := make([]v2.Filter, 0, len(xdsFilters))
	for _, xdsFilter := range xdsFilters {
		filter := v2.Filter{
			Name:    v2.DEFAULT_NETWORK_FILTER,
			Config:  convertFilterConfig(xdsFilter.GetName(), xdsFilter.GetConfig()),
		}
		filters = append(filters, filter)
	}
	return filters
}

// TODO: more filter config support
func convertFilterConfig(name string, s *types.Struct) map[string]interface{} {
	if s == nil {
		return nil
	}
	if name == xdsutil.HTTPConnectionManager {
		filterConfig := &xdshttp.HttpConnectionManager{}
		xdsutil.StructToMessage(s, filterConfig)
		proxyConfig := v2.Proxy{
			DownstreamProtocol: string(protocol.Http2),
			UpstreamProtocol:   string(protocol.Http2),
			VirtualHosts:       convertVirtualHosts(filterConfig),
		}
		return structs.Map(proxyConfig)
	}else if name == v2.SOFARPC_OUTBOUND_FILTER || name == v2.SOFARPC_INBOUND_FILTER {
		filterConfig := &xdshttp.HttpConnectionManager{}
		xdsutil.StructToMessage(s, filterConfig)
		proxyConfig := v2.Proxy{
			DownstreamProtocol: string(protocol.SofaRpc),
			UpstreamProtocol:   string(protocol.SofaRpc),
			VirtualHosts:       convertVirtualHosts(filterConfig),
		}
		return structs.Map(proxyConfig)
	}else if name == v2.X_PROTOCOL_FILTER {
		//TODO
	}else{
		log.DefaultLogger.Fatalf("unsupport filter config, filter name: %s", name)
	}
	return nil
}

func convertVirtualHosts(xdsFilterConfig *xdshttp.HttpConnectionManager) []*v2.VirtualHost {
	if xdsFilterConfig == nil {
		return nil
	}
	virtualHosts := make([]*v2.VirtualHost, 0)
	if xdsRouteConfig := xdsFilterConfig.GetRouteConfig(); xdsRouteConfig != nil {
		for _, xdsVirtualHost := range xdsRouteConfig.GetVirtualHosts() {
			virtualHost := &v2.VirtualHost{
				Name:               xdsVirtualHost.GetName(),
				Domains:            xdsVirtualHost.GetDomains(),
				Routers:            convertRoutes(xdsVirtualHost.GetRoutes()),
				RequireTls:         xdsVirtualHost.GetRequireTls().String(),
				VirtualClusters:    convertVirtualClusters(xdsVirtualHost.GetVirtualClusters()),
			}
			virtualHosts = append(virtualHosts, virtualHost)
		}
	}
	return virtualHosts
}

func convertRoutes(xdsRoutes []xdsroute.Route) []v2.Router {
	if xdsRoutes == nil {
		return nil
	}
	routes := make([]v2.Router, 0, len(xdsRoutes))
	for _, xdsRoute := range xdsRoutes {
		if xdsRouteAction := xdsRoute.GetRoute(); xdsRouteAction != nil {
			route := v2.Router{
				Match:         convertRouteMatch(xdsRoute.GetMatch()),
				Route:         convertRouteAction(xdsRouteAction),
				Metadata:      convertMeta(xdsRoute.GetMetadata()),
				Decorator:     v2.Decorator(xdsRoute.GetDecorator().String()),
			}
			routes = append(routes, route)
		}else if xdsRouteAction := xdsRoute.GetRedirect(); xdsRouteAction != nil {
			route := v2.Router{
				Match:         convertRouteMatch(xdsRoute.GetMatch()),
				Redirect:      convertRedirectAction(xdsRouteAction),
				Metadata:      convertMeta(xdsRoute.GetMetadata()),
				Decorator:     v2.Decorator(xdsRoute.GetDecorator().String()),
			}
			routes = append(routes, route)
		}else{
			log.DefaultLogger.Fatalf("unsupport route actin, just Route and Redirect support yet, ignore this route")
			continue
		}
	}
	return routes
}

func convertRouteMatch(xdsRouteMatch xdsroute.RouteMatch) v2.RouterMatch {
	return v2.RouterMatch{
		Prefix:           xdsRouteMatch.GetPrefix(),
		Path:             xdsRouteMatch.GetPath(),
		Regex:            xdsRouteMatch.GetRegex(),
		CaseSensitive:    xdsRouteMatch.GetCaseSensitive().GetValue(),
		Runtime:          convertRuntime(xdsRouteMatch.GetRuntime()),
		Headers:          convertHeaders(xdsRouteMatch.GetHeaders()),
	}
}

func convertRuntime(xdsRuntime *xdscore.RuntimeUInt32) v2.RuntimeUInt32 {
	if xdsRuntime == nil {
		return v2.RuntimeUInt32{}
	}
	return v2.RuntimeUInt32{
		DefaultValue:    xdsRuntime.GetDefaultValue(),
		RuntimeKey:      xdsRuntime.GetRuntimeKey(),
	}
}

func convertHeaders(xdsHeaders []*xdsroute.HeaderMatcher) []v2.HeaderMatcher {
	if xdsHeaders == nil {
		return nil
	}
	headerMatchers := make([]v2.HeaderMatcher, 0, len(xdsHeaders))
	for _, xdsHeader := range xdsHeaders {
		headerMatcher := v2.HeaderMatcher{
			Name:    xdsHeader.GetName(),
			Value:   xdsHeader.GetValue(),
			Regex:   xdsHeader.GetRegex().GetValue(),
		}
		headerMatchers = append(headerMatchers, headerMatcher)
	}
	return headerMatchers
}

func convertMeta(xdsMeta *xdscore.Metadata) v2.Metadata {
	if xdsMeta == nil {
		return nil
	}
	meta := make(map[string]string, len(xdsMeta.GetFilterMetadata()))
	for key, value := range xdsMeta.GetFilterMetadata() {
		meta[key] = value.String()
	}
	return meta
}

func convertRouteAction(xdsRouteAction *xdsroute.RouteAction) v2.RouteAction {
	if xdsRouteAction == nil {
		return v2.RouteAction{}
	}
	return v2.RouteAction{
		ClusterName:         xdsRouteAction.GetCluster(),
		ClusterHeader:       xdsRouteAction.GetClusterHeader(),
		WeightedClusters:    convertWeightedClusters(xdsRouteAction.GetWeightedClusters()),
		MetadataMatch:       convertMeta(xdsRouteAction.GetMetadataMatch()),
		Timeout:             convertTimeDurPoint2TimeDur(xdsRouteAction.GetTimeout()),
		RetryPolicy:         convertRetryPolicy(xdsRouteAction.GetRetryPolicy()),
	}
}

func convertTimeDurPoint2TimeDur(duration *time.Duration) time.Duration {
	if duration == nil {
		return time.Duration(0)
	}
	return *duration
}

func convertWeightedClusters(xdsWeightedClusters *xdsroute.WeightedCluster)  []v2.WeightedCluster{
	if xdsWeightedClusters == nil {
		return nil
	}
	weightedClusters := make([]v2.WeightedCluster, 0, len(xdsWeightedClusters.GetClusters()))
	for _, cluster := range xdsWeightedClusters.GetClusters(){
		weightedCluster := v2.WeightedCluster{
			Clusters:           convertWeightedCluster(cluster),
			RuntimeKeyPrefix:   xdsWeightedClusters.GetRuntimeKeyPrefix(),
		}
		weightedClusters = append(weightedClusters, weightedCluster)
	}
	return weightedClusters
}

func convertWeightedCluster(xdsWeightedCluster *xdsroute.WeightedCluster_ClusterWeight)  v2.ClusterWeight{
	if xdsWeightedCluster == nil {
		return v2.ClusterWeight{}
	}
	return v2.ClusterWeight{
		Name:              xdsWeightedCluster.GetName(),
		Weight:            xdsWeightedCluster.GetWeight().GetValue(),
		MetadataMatch:     convertMeta(xdsWeightedCluster.GetMetadataMatch()),
	}
}

func convertRetryPolicy(xdsRetryPolicy *xdsroute.RouteAction_RetryPolicy) *v2.RetryPolicy {
	if xdsRetryPolicy == nil {
		return &v2.RetryPolicy{}
	}
	return &v2.RetryPolicy{
		RetryOn:         len(xdsRetryPolicy.GetRetryOn()) > 0,
		RetryTimeout:    convertTimeDurPoint2TimeDur(xdsRetryPolicy.GetPerTryTimeout()),
		NumRetries:      xdsRetryPolicy.GetNumRetries().GetValue(),
	}
}

func convertRedirectAction(xdsRedirectAction *xdsroute.RedirectAction) v2.RedirectAction {
	if xdsRedirectAction == nil {
		return v2.RedirectAction{}
	}
	return v2.RedirectAction{
		HostRedirect:    xdsRedirectAction.GetHostRedirect(),
		PathRedirect:    xdsRedirectAction.GetPathRedirect(),
		ResponseCode:    uint32(xdsRedirectAction.GetResponseCode()),
	}
}

func convertVirtualClusters(xdsVirtualClusters []*xdsroute.VirtualCluster) []v2.VirtualCluster {
	if xdsVirtualClusters == nil {
		return nil
	}
	virtualClusters := make([]v2.VirtualCluster, 0, len(xdsVirtualClusters))
	for _, xdsVirtualCluster := range xdsVirtualClusters {
		virtualCluster := v2.VirtualCluster{
			Pattern:      xdsVirtualCluster.GetPattern(),
			Name:         xdsVirtualCluster.GetName(),
			Method:       xdsVirtualCluster.GetMethod().String(),
		}
		virtualClusters = append(virtualClusters, virtualCluster)
	}
	return virtualClusters
}

func convertAddress(xdsAddress *xdscore.Address) net.Addr {
	if xdsAddress == nil {
		return nil
	}
	var address string
	if addr, ok := xdsAddress.GetAddress().(*xdscore.Address_SocketAddress); ok {
		if xdsPort, ok := addr.SocketAddress.GetPortSpecifier().(*xdscore.SocketAddress_PortValue); ok {
			address = fmt.Sprintf("%s:%d", addr.SocketAddress.GetAddress(), xdsPort.PortValue)
		}else{
			log.DefaultLogger.Warnf("only port value supported")
			return nil
		}
	}else{
		log.DefaultLogger.Fatalf("only SocketAddress supported")
		return nil
	}

	tcpAddr, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		log.DefaultLogger.Fatalf("Invalid address: %v", err)
		return nil
	}
	return tcpAddr
}

func convertClusterType(xdsClusterType xdsapi.Cluster_DiscoveryType) v2.ClusterType {
	switch xdsClusterType {
	case xdsapi.Cluster_STATIC:
		return v2.STATIC_CLUSTER
	case xdsapi.Cluster_STRICT_DNS:
	case xdsapi.Cluster_LOGICAL_DNS:
	case xdsapi.Cluster_EDS:
		return v2.DYNAMIC_CLUSTER
	case xdsapi.Cluster_ORIGINAL_DST:
	}
	//log.DefaultLogger.Fatalf("unsupported cluster type: %s, exchage to SIMPLE_CLUSTER", xdsClusterType.String())
	return v2.SIMPLE_CLUSTER
}

func convertLbPolicy(xdsLbPolicy xdsapi.Cluster_LbPolicy) v2.LbType {
	switch xdsLbPolicy {
	case xdsapi.Cluster_ROUND_ROBIN:
		return v2.LB_ROUNDROBIN
	case xdsapi.Cluster_LEAST_REQUEST:
	case xdsapi.Cluster_RING_HASH:
	case xdsapi.Cluster_RANDOM:
		return v2.LB_RANDOM
	case xdsapi.Cluster_ORIGINAL_DST_LB:
	case xdsapi.Cluster_MAGLEV:
	}
	//log.DefaultLogger.Fatalf("unsupported lb policy: %s, exchange to LB_RANDOM", xdsLbPolicy.String())
	return v2.LB_RANDOM
}

func convertLbSubSetConfig(xdsLbSubsetConfig *xdsapi.Cluster_LbSubsetConfig) v2.LBSubsetConfig {
	if xdsLbSubsetConfig == nil {
		return v2.LBSubsetConfig{}
	}
	return v2.LBSubsetConfig{
		FallBackPolicy:  uint8(xdsLbSubsetConfig.GetFallbackPolicy()),
		DefaultSubset:   convertTypesStruct(xdsLbSubsetConfig.GetDefaultSubset()),
		SubsetSelectors: convertSubsetSelectors(xdsLbSubsetConfig.GetSubsetSelectors()),
	}
}

func convertTypesStruct(s *types.Struct) map[string]string {
	if s == nil {
		return nil
	}
	meta := make(map[string]string, len(s.GetFields()))
	for key, value := range s.GetFields() {
		meta[key] = value.String()
	}
	return meta
}

func convertSubsetSelectors(xdsSubsetSelectors []*xdsapi.Cluster_LbSubsetConfig_LbSubsetSelector) [][]string {
	if xdsSubsetSelectors == nil {
		return nil
	}
	subsetSelectors := make([][]string, 0, len(xdsSubsetSelectors))
	for _, xdsSubsetSelector := range xdsSubsetSelectors {
		subsetSelectors = append(subsetSelectors, xdsSubsetSelector.GetKeys())
	}
	return subsetSelectors
}

func convertHealthChecks(xdsHealthChecks []*xdscore.HealthCheck) v2.HealthCheck {
	if xdsHealthChecks == nil || len(xdsHealthChecks) == 0 || xdsHealthChecks[0] == nil {
		return v2.HealthCheck{}
	}

	return v2.HealthCheck{
		Timeout:            convertDuration(xdsHealthChecks[0].GetTimeout()),
		HealthyThreshold:   xdsHealthChecks[0].GetHealthyThreshold().GetValue(),
		UnhealthyThreshold: xdsHealthChecks[0].GetUnhealthyThreshold().GetValue(),
		Interval:           convertDuration(xdsHealthChecks[0].GetInterval()),
		IntervalJitter:     convertDuration(xdsHealthChecks[0].GetIntervalJitter()),
	}
}

func convertCircuitBreakers(xdsCircuitBreaker *xdscluster.CircuitBreakers) v2.CircuitBreakers{
	if xdsCircuitBreaker == nil {
		return v2.CircuitBreakers{}
	}
	thresholds := make([]v2.Thresholds, 0, len(xdsCircuitBreaker.GetThresholds()))
	for _, xdsThreshold := range xdsCircuitBreaker.GetThresholds() {
		threshold := v2.Thresholds{
			Priority:             v2.RoutingPriority(xdsThreshold.GetPriority().String()),
			MaxConnections:       xdsThreshold.GetMaxConnections().GetValue(),
			MaxPendingRequests:   xdsThreshold.GetMaxPendingRequests().GetValue(),
			MaxRequests:          xdsThreshold.GetMaxRequests().GetValue(),
			MaxRetries:           xdsThreshold.GetMaxRetries().GetValue(),
		}
		thresholds = append(thresholds, threshold)
	}
	return v2.CircuitBreakers{
		Thresholds: thresholds,
	}
}

func convertOutlierDetection(xdsOutlierDetection *xdscluster.OutlierDetection) v2.OutlierDetection {
	if xdsOutlierDetection == nil {
		return v2.OutlierDetection{}
	}
	return v2.OutlierDetection{
		Consecutive_5Xx:                    xdsOutlierDetection.GetConsecutive_5Xx().GetValue(),
		Interval:                           convertDuration(xdsOutlierDetection.GetInterval()),
		BaseEjectionTime:                   convertDuration(xdsOutlierDetection.GetBaseEjectionTime()),
		MaxEjectionPercent:                 xdsOutlierDetection.GetMaxEjectionPercent().GetValue(),
		ConsecutiveGatewayFailure:          xdsOutlierDetection.GetEnforcingConsecutive_5Xx().GetValue(),
		EnforcingConsecutive5xx:            xdsOutlierDetection.GetConsecutive_5Xx().GetValue(),
		EnforcingConsecutiveGatewayFailure: xdsOutlierDetection.GetEnforcingConsecutiveGatewayFailure().GetValue(),
		EnforcingSuccessRate:               xdsOutlierDetection.GetEnforcingSuccessRate().GetValue(),
		SuccessRateMinimumHosts:            xdsOutlierDetection.GetSuccessRateMinimumHosts().GetValue(),
		SuccessRateRequestVolume:           xdsOutlierDetection.GetSuccessRateRequestVolume().GetValue(),
		SuccessRateStdevFactor:             xdsOutlierDetection.GetSuccessRateStdevFactor().GetValue(),
	}
}

func convertSpec(xdsCluster *xdsapi.Cluster) v2.ClusterSpecInfo {
	if xdsCluster == nil || xdsCluster.GetEdsClusterConfig() == nil {
		return v2.ClusterSpecInfo{}
	}
	specs := make([]v2.SubscribeSpec, 0, 1)
	spec := v2.SubscribeSpec{
		ServiceName: xdsCluster.GetEdsClusterConfig().GetServiceName(),
	}
	specs = append(specs, spec)
	return v2.ClusterSpecInfo{
		Subscribes: specs,
	}
}

/*
func convertClusterHosts(xdsHosts []*xdscore.Address) []v2.HostWithMetadata {
	if xdsHosts == nil {
		return nil
	}
	hostsWithMetaData := make([]v2.HostWithMetadata, 0, len(xdsHosts))
	for _, xdsHost := range xdsHosts {
		addr := convertAddress(xdsHost)
		host := v2.Host{
			Address:    addr.String(),
		}
		hostWithMetaData := v2.HostWithMetadata{
			Host:   host,
		}
		hostsWithMetaData = append(hostsWithMetaData, hostWithMetaData)
	}
	return hostsWithMetaData
}*/

func convertDuration(p *types.Duration) time.Duration {
	if p == nil {
		return time.Duration(0)
	}
	d := time.Duration(p.Seconds) * time.Second
	if p.Nanos != 0 {
		if dur := d + time.Duration(p.Nanos); (dur < 0) != (p.Nanos < 0) {
			log.DefaultLogger.Fatalf("duration: %#v is out of range for time.Duration, ignore nanos", p)
		}
	}
	return d
}

// TODO
func convertTLS(xdsTlsContext *xdsauth.DownstreamTlsContext) v2.TLSConfig {
	return v2.TLSConfig{}
}