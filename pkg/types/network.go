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

package types

import (
	"context"
	"errors"
	"net"
	"os"
	"time"

	"github.com/rcrowley/go-metrics"
	"mosn.io/mosn/pkg/api/v2"
)

//
//   The bunch of interfaces are structure skeleton to build a high performance, extensible network framework.
//
//   In mosn, we have 4 layers to build a mesh, net/io layer is the fundamental layer to support upper level's functionality.
//	 -----------------------
//   |        PROXY          |
//    -----------------------
//   |       STREAMING       |
//    -----------------------
//   |        PROTOCOL       |
//    -----------------------
//   |         NET/IO        |
//    -----------------------
//
//   Core model in network layer are listener and connection. Listener listens specified port, waiting for new connections.
//   Both listener and connection have a extension mechanism, implemented as listener and filter chain, which are used to fill in customized logic.
//   Event listeners are used to subscribe important event of Listener and Connection. Method in listener will be called on event occur, but not effect the control flow.
//   Filters are called on event occurs, it also returns a status to effect control flow. Currently 2 states are used: Continue to let it go, Stop to stop the control flow.
//   Filter has a callback handler to interactive with core model. For example, ReadFilterCallbacks can be used to continue filter chain in connection, on which is in a stopped state.
//
//   Listener:
//   	- Event listener
// 			- ListenerEventListener
//      - Filter
// 			- ListenerFilter
//   Connection:
//		- Event listener
// 			- ConnectionEventListener
//		- Filter
// 			- ReadFilter
//			- WriteFilter
//
//   Below is the basic relation on listener and connection:
//    --------------------------------------------------
//   |                                      			|
//   | 	  EventListener       EventListener     		|
//   |        *|                   |*          		    |
//   |         |                   |       				|
//   |        1|     1      *      |1          			|
// 	 |	    Listener --------- Connection      			|
//   |        1|      [accept]     |1          			|
//	 |         |                   |-----------         |
//   |        *|                   |*          |*       |
//   |	 ListenerFilter       ReadFilter  WriteFilter   |
//   |                                                  |
//    --------------------------------------------------
//

// Listener is a wrapper of tcp listener
type Listener interface {
	// Return config which initialize this listener
	Config() *v2.Listener

	// Set listener config
	SetConfig(config *v2.Listener)

	// Name returns the listener's name
	Name() string

	// Addr returns the listener's network address.
	Addr() net.Addr

	// Start starts listener with context
	Start(lctx context.Context, restart bool)

	// Stop stops listener
	// Accepted connections and listening sockets will not be closed
	Stop() error

	// ListenerTag returns the listener's tag, whichi the listener should use for connection handler tracking.
	ListenerTag() uint64

	// Set listener tag
	SetListenerTag(tag uint64)

	// ListenerFile returns a copy a listener file
	ListenerFile() (*os.File, error)

	// PerConnBufferLimitBytes returns the limit bytes per connection
	PerConnBufferLimitBytes() uint32

	// Set limit bytes per connection
	SetPerConnBufferLimitBytes(limitBytes uint32)

	// Set if listener should use original dst
	SetUseOriginalDst(use bool)

	// Get if listener should use original dst
	UseOriginalDst() bool

	// SetListenerCallbacks set a listener event listener
	SetListenerCallbacks(cb ListenerEventListener)

	// GetListenerCallbacks set a listener event listener
	GetListenerCallbacks() ListenerEventListener

	// Close closes listener, not closing connections
	Close(lctx context.Context) error
}

// ListenerEventListener is a Callback invoked by a listener.
type ListenerEventListener interface {
	// OnAccept is called on new connection accepted
	OnAccept(rawc net.Conn, useOriginalDst bool, oriRemoteAddr net.Addr, c chan Connection, buf []byte)

	// OnNewConnection is called on new mosn connection created
	OnNewConnection(ctx context.Context, conn Connection)

	// OnClose is called on listener close
	OnClose()
}

// FilterStatus type
type FilterStatus string

// FilterStatus types
const (
	Continue FilterStatus = "Continue"
	Stop     FilterStatus = "Stop"
)

type ListenerFilter interface {
	// OnAccept is called when a raw connection is accepted, but before a Connection is created.
	OnAccept(cb ListenerFilterCallbacks) FilterStatus
}

// ListenerFilterCallbacks is a callback handler called by listener filter to talk to listener
type ListenerFilterCallbacks interface {
	// Conn returns the Connection reference used in callback handler
	Conn() net.Conn

	ContinueFilterChain(ctx context.Context, success bool)

	// SetOriginalAddr sets the original ip and port
	SetOriginalAddr(ip string, port int)
}

// ListenerFilterManager manages the listener filter
// Note: unsupport now
type ListenerFilterManager interface {
	AddListenerFilter(lf *ListenerFilter)
}

// Connection status
type ConnState int

// Connection statuses
const (
	ConnInit ConnState = iota
	ConnActive
	ConnClosed
)

// ConnectionCloseType represent connection close type
type ConnectionCloseType string

//Connection close types
const (
	// FlushWrite means write buffer to underlying io then close connection
	FlushWrite ConnectionCloseType = "FlushWrite"
	// NoFlush means close connection without flushing buffer
	NoFlush ConnectionCloseType = "NoFlush"
)

// Connection interface
type Connection interface {
	// ID returns unique connection id
	ID() uint64

	// Start starts connection with context.
	// See context.go to get available keys in context
	Start(lctx context.Context)

	// Write writes data to the connection.
	// Called by other-side stream connection's read loop. Will loop through stream filters with the buffer if any are installed.
	Write(buf ...IoBuffer) error

	// Close closes connection with connection type and event type.
	// ConnectionCloseType - how to close to connection
	// 	- FlushWrite: connection will be closed after buffer flushed to underlying io
	//	- NoFlush: close connection asap
	// ConnectionEvent - why to close the connection
	// 	- RemoteClose
	//  - LocalClose
	// 	- OnReadErrClose
	//  - OnWriteErrClose
	//  - OnConnect
	//  - Connected:
	//	- ConnectTimeout
	//	- ConnectFailed
	Close(ccType ConnectionCloseType, eventType ConnectionEvent) error

	// LocalAddr returns the local address of the connection.
	// For client connection, this is the origin address
	// For server connection, this is the proxy's address
	// TODO: support get local address in redirected request
	// TODO: support transparent mode
	LocalAddr() net.Addr

	// RemoteAddr returns the remote address of the connection.
	RemoteAddr() net.Addr

	// SetRemoteAddr is used for originaldst we need to replace remoteAddr
	SetRemoteAddr(address net.Addr)

	// AddConnectionEventListener add a listener method will be called when connection event occur.
	AddConnectionEventListener(listener ConnectionEventListener)

	// AddBytesReadListener add a method will be called everytime bytes read
	AddBytesReadListener(listener func(bytesRead uint64))

	// AddBytesSentListener add a method will be called everytime bytes write
	AddBytesSentListener(listener func(bytesSent uint64))

	// NextProtocol returns network level negotiation, such as ALPN. Returns empty string if not supported.
	NextProtocol() string

	// SetNoDelay enable/disable tcp no delay
	SetNoDelay(enable bool)

	// SetReadDisable enable/disable read on the connection.
	// If reads are enabled after disable, connection continues to read and data will be dispatched to read filter chains.
	SetReadDisable(disable bool)

	// ReadEnabled returns whether reading is enabled on the connection.
	ReadEnabled() bool

	// TLS returns a related tls connection.
	TLS() net.Conn

	// SetBufferLimit set the buffer limit.
	SetBufferLimit(limit uint32)

	// BufferLimit returns the buffer limit.
	BufferLimit() uint32

	// SetLocalAddress sets a local address
	SetLocalAddress(localAddress net.Addr, restored bool)

	// SetCollector set read/write mertics collectors
	SetCollector(read, write metrics.Counter)
	// LocalAddressRestored returns whether local address is restored
	// TODO: unsupported now
	LocalAddressRestored() bool

	// GetWriteBuffer is used by network writer filter
	GetWriteBuffer() []IoBuffer

	// GetReadBuffer is used by network read filter
	GetReadBuffer() IoBuffer

	// FilterManager returns the FilterManager
	FilterManager() FilterManager

	// RawConn returns the original connections.
	// Caution: raw conn only used in io-loop disable mode
	// TODO: a better way to provide raw conn
	RawConn() net.Conn

	// SetTransferEventListener set a method will be called when connection transfer occur
	SetTransferEventListener(listener func() bool)

	// SetIdleTimeout sets the timeout that will set the connnection to idle. mosn close idle connection
	// if no idle timeout setted or a zero value for d means no idle connections.
	SetIdleTimeout(d time.Duration)

	// State returns the connection state
	State() ConnState
}

// ConnectionStats is a group of connection metrics
type ConnectionStats struct {
	ReadTotal     metrics.Counter
	ReadBuffered  metrics.Gauge
	WriteTotal    metrics.Counter
	WriteBuffered metrics.Gauge
}

// ClientConnection is a wrapper of Connection
type ClientConnection interface {
	Connection

	// connect to server in a async way
	Connect() error
}

// ConnectionEvent type
type ConnectionEvent string

// ConnectionEvent types
const (
	RemoteClose     ConnectionEvent = "RemoteClose"
	LocalClose      ConnectionEvent = "LocalClose"
	OnReadErrClose  ConnectionEvent = "OnReadErrClose"
	OnWriteErrClose ConnectionEvent = "OnWriteErrClose"
	OnConnect       ConnectionEvent = "OnConnect"
	Connected       ConnectionEvent = "ConnectedFlag"
	ConnectTimeout  ConnectionEvent = "ConnectTimeout"
	ConnectFailed   ConnectionEvent = "ConnectFailed"
	OnReadTimeout   ConnectionEvent = "OnReadTimeout"
	OnWriteTimeout  ConnectionEvent = "OnWriteTimeout"
)

// IsClose represents whether the event is triggered by connection close
func (ce ConnectionEvent) IsClose() bool {
	return ce == LocalClose || ce == RemoteClose ||
		ce == OnReadErrClose || ce == OnWriteErrClose || ce == OnWriteTimeout
}

// ConnectFailure represents whether the event is triggered by connection failure
func (ce ConnectionEvent) ConnectFailure() bool {
	return ce == ConnectFailed || ce == ConnectTimeout
}

// Default connection arguments
const (
	DefaultConnReadTimeout  = 15 * time.Second
	DefaultConnWriteTimeout = 15 * time.Second
)

// ConnectionEventListener is a network level callbacks that happen on a connection.
type ConnectionEventListener interface {
	// OnEvent is called on ConnectionEvent
	OnEvent(event ConnectionEvent)
}

// ConnectionHandler contains the listeners for a mosn server
type ConnectionHandler interface {
	// AddOrUpdateListener
	// adds a listener into the ConnectionHandler or
	// update a listener
	AddOrUpdateListener(lc *v2.Listener, networkFiltersFactories []NetworkFilterChainFactory,
		streamFiltersFactories []StreamFilterChainFactory) (ListenerEventListener, error)

	//StartListeners starts all listeners the ConnectionHandler has
	StartListeners(lctx context.Context)

	// FindListenerByAddress finds and returns a listener by the specified network address
	FindListenerByAddress(addr net.Addr) Listener

	// FindListenerByName finds and returns a listener by the listener name
	FindListenerByName(name string) Listener

	// RemoveListeners find and removes a listener by listener name.
	RemoveListeners(name string)

	// StopListener stops a listener  by listener name
	StopListener(lctx context.Context, name string, stop bool) error

	// StopListeners stops all listeners the ConnectionHandler has.
	// The close indicates whether the listening sockets will be closed.
	StopListeners(lctx context.Context, close bool) error

	// ListListenersFD reports all listeners' fd
	ListListenersFile(lctx context.Context) []*os.File

	// StopConnection Stop Connection
	StopConnection()
}

// ReadFilter is a connection binary read filter, registered by FilterManager.AddReadFilter
type ReadFilter interface {
	// OnData is called everytime bytes is read from the connection
	OnData(buffer IoBuffer) FilterStatus

	// OnNewConnection is called on new connection is created
	OnNewConnection() FilterStatus

	// InitializeReadFilterCallbacks initials read filter callbacks. It used by init read filter
	InitializeReadFilterCallbacks(cb ReadFilterCallbacks)
}

// WriteFilter is a connection binary write filter, only called by conn accept loop
type WriteFilter interface {
	// OnWrite is called before data write to raw connection
	OnWrite(buffer []IoBuffer) FilterStatus
}

// ReadFilterCallbacks is called by read filter to talk to connection
type ReadFilterCallbacks interface {
	// Connection returns the connection triggered the callback
	Connection() Connection

	// ContinueReading filter iteration on filter stopped, next filter will be called with current read buffer
	ContinueReading()

	// UpstreamHost returns current selected upstream host.
	UpstreamHost() HostInfo

	// SetUpstreamHost set currently selected upstream host.
	SetUpstreamHost(upstreamHost HostInfo)
}

// FilterManager is a groups of filters
type FilterManager interface {
	// AddReadFilter adds a read filter
	AddReadFilter(rf ReadFilter)

	// AddWriteFilter adds a write filter
	AddWriteFilter(wf WriteFilter)

	// ListReadFilter returns the list of read filters
	ListReadFilter() []ReadFilter

	// ListWriteFilters returns the list of write filters
	ListWriteFilters() []WriteFilter

	// InitializeReadFilters initialize read filters
	InitializeReadFilters() bool

	// OnRead is called on data read
	OnRead()

	// OnWrite is called before data write
	OnWrite(buffer []IoBuffer) FilterStatus
}

type FilterChainFactory interface {
	CreateNetworkFilterChain(conn Connection)

	CreateListenerFilterChain(listener ListenerFilterManager)
}

// NetWorkFilterChainFactoryCallbacks is a wrapper of FilterManager that called in NetworkFilterChainFactory
type NetWorkFilterChainFactoryCallbacks interface {
	AddReadFilter(rf ReadFilter)
	AddWriteFilter(wf WriteFilter)
}

// NetworkFilterChainFactory adds filter into NetWorkFilterChainFactoryCallbacks
type NetworkFilterChainFactory interface {
	CreateFilterChain(context context.Context, clusterManager ClusterManager, callbacks NetWorkFilterChainFactoryCallbacks)
}

// Addresses defines a group of network address
type Addresses []net.Addr

// Contains reports whether the specified network address is in the group.
func (as Addresses) Contains(addr net.Addr) bool {
	for _, one := range as {
		// TODO: support port wildcard
		if one.String() == addr.String() {
			return true
		}
	}

	return false
}

var (
	ErrConnectionHasClosed = errors.New("connection has closed")
)
