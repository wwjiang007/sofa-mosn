package websocket

import (
	"context"

	"time"

	"github.com/alipay/sofa-mosn/pkg/api/v2"
	"github.com/alipay/sofa-mosn/pkg/filter/network/tcpproxy"
	"github.com/alipay/sofa-mosn/pkg/log"
	"github.com/alipay/sofa-mosn/pkg/protocol"
	str "github.com/alipay/sofa-mosn/pkg/stream"
	"github.com/alipay/sofa-mosn/pkg/types"
)

// Config
type Config struct {
	StatPrefix         string
	IdleTimeout        *time.Duration
	MaxConnectAttempts uint32
}

func init() {
	str.Register(protocol.WebSocket, &streamConnFactory{})
}

type streamConnFactory struct{}

// CreateClientStream upstream create
func (f *streamConnFactory) CreateClientStream(context context.Context, connection types.ClientConnection,
	clientCallbacks types.StreamConnectionEventListener, connCallbacks types.ConnectionEventListener) types.ClientStreamConnection {
	return newStreamConnection(context, connection, clientCallbacks, nil)
}

// CreateServerStream downstream create
func (f *streamConnFactory) CreateServerStream(context context.Context, connection types.Connection,
	serverCallbacks types.ServerStreamConnectionEventListener) types.ServerStreamConnection {
	return newStreamConnection(context, connection, nil, serverCallbacks)
}

// CreateBiDirectStream no used
func (f *streamConnFactory) CreateBiDirectStream(context context.Context, connection types.ClientConnection,
	clientCallbacks types.StreamConnectionEventListener,
	serverCallbacks types.ServerStreamConnectionEventListener) types.ClientStreamConnection {
	return nil
}

func newStreamConnection(context context.Context, connection types.Connection, clientCallbacks types.StreamConnectionEventListener,
	serverCallbacks types.ServerStreamConnectionEventListener) types.ClientStreamConnection {
	subProtocolName := types.SubProtocol(context.Value(types.ContextSubProtocol).(string))

	var clusterManager types.ClusterManager = nil
	log.DefaultLogger.Tracef("xprotocol subprotocol config name = %v", subProtocolName)
	log.DefaultLogger.Tracef("xprotocol new stream connection, codec type = %v", subProtocolName)
	sc := &streamConnection{
		context:         context,
		protocol:        protocol.WebSocket,
		connection:      connection,
		clientCallbacks: clientCallbacks,
		serverCallbacks: serverCallbacks,
		logger:          log.ByContext(context),
		clusterManager:  clusterManager,
		proxy:           nil,
		queueData:       nil,
		state:           PreConnect,
	}
	return sc
}

func newTcpProxyConfig(wsConfig Config, cluster string) *v2.TCPProxy {
	tcpProxy := &v2.TCPProxy{
		StatPrefix:         wsConfig.StatPrefix,
		Cluster:            cluster,
		IdleTimeout:        wsConfig.IdleTimeout,
		MaxConnectAttempts: wsConfig.MaxConnectAttempts,
		Routes:             nil,
	}
	return tcpProxy
}

type ConnectionState string

const (
	PreConnect ConnectionState = "PreConnect"
	Connected  ConnectionState = "Connected"
	Failed     ConnectionState = "Failed"
)

type streamConnection struct {
	context         context.Context
	protocol        types.Protocol
	connection      types.Connection
	clientCallbacks types.StreamConnectionEventListener
	serverCallbacks types.ServerStreamConnectionEventListener
	logger          log.Logger
	proxy           tcpproxy.Proxy
	clusterManager  types.ClusterManager
	cb              types.ReadFilterCallbacks
	routeEntry      v2.RouteAction
	queueData       types.IoBuffer
	state           ConnectionState
}

func (conn *streamConnection) Dispatch(buffer types.IoBuffer) {
	// H1 Codec header and get route
	if conn.proxy != nil {
		if conn.state == PreConnect {
			// http upgrade request has at most 0 or 1 data
			conn.queueData = buffer.Clone()
			buffer.Drain(buffer.Len())
		} else {
			conn.OnData(buffer)
		}
	} else {
		// TODO http1 codec handle websocket upgrade and check websocket config
		conn.Upgrade(buffer)
		conn.proxy.InitializeReadFilterCallbacks(conn.cb)
		conn.OnNewConnection()
	}
}

func (conn *streamConnection) OnData(buffer types.IoBuffer) {
	conn.proxy.OnData(buffer)
}

func (conn *streamConnection) OnNewConnection() {
	conn.proxy.OnNewConnection()
}

func (conn *streamConnection) onConnectionSuccess() {
	// path and host rewrites

	// for auto host rewrite

	// http1 codec header encode and send
	if conn.state != PreConnect {
		log.DefaultLogger.Errorf("error connection state,stream connection =%v", conn)
		return
	}

	conn.state = Connected
	if conn.queueData != nil {
		conn.proxy.OnData(conn.queueData)
		conn.queueData = nil
	}
}

// Upgrade
// check websocket upgrade and route config
// new tcp proxy
// set routeAction
func (conn *streamConnection) Upgrade(buffer types.IoBuffer) {
	// TODO handle http1 protocol & get route action
	// TODO send header

	wsConfig := Config{
		StatPrefix:         "websocket",
		IdleTimeout:        nil,
		MaxConnectAttempts: 10,
	}
	cluster := conn.routeEntry.ClusterName
	tcpProxyConfig := newTcpProxyConfig(wsConfig, cluster)
	conn.proxy = tcpproxy.NewProxyForWebSocket(conn.context, tcpProxyConfig, conn.clusterManager, conn)

	hasData := true
	// check if req has body ,send body
	if hasData {
		conn.proxy.OnData(buffer)
	}
}

// Protocol on the connection
func (conn *streamConnection) Protocol() types.Protocol {
	return protocol.WebSocket
}

// GoAway sends go away to remote for graceful shutdown
func (conn *streamConnection) GoAway() {

}

// NewStream
func (conn *streamConnection) NewStream(ctx context.Context, streamID string, responseDecoder types.StreamReceiver) types.StreamSender {
	log.DefaultLogger.Tracef("xprotocol stream new stream,streamId =%v ", streamID)
	return nil
}

// OnEvent UpstreamEventCallbacks
func (conn *streamConnection) OnEvent(event types.ConnectionEvent) tcpproxy.UpstreamEventIterator {
	switch event {
	case types.OnConnect:
	case types.Connected:
		conn.onConnectionSuccess()
		return tcpproxy.EventStop
	}
	return tcpproxy.EventContinue
}
