package integrate

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"mosn.io/mosn/pkg/mosn"
	"mosn.io/mosn/pkg/protocol"
	"mosn.io/mosn/pkg/protocol/rpc/sofarpc"
	"mosn.io/mosn/pkg/protocol/rpc/sofarpc/codec"
	"mosn.io/mosn/pkg/types"
	"mosn.io/mosn/test/util"
)

type RetryCase struct {
	*TestCase
	GoodServer util.UpstreamServer
	BadServer  util.UpstreamServer
	BadIsClose bool
}

func NewRetryCase(t *testing.T, serverProto, meshProto types.Protocol, isClose bool) *RetryCase {
	app1 := "127.0.0.1:8080"
	app2 := "127.0.0.1:8081"
	var good, bad util.UpstreamServer
	switch serverProto {
	case protocol.HTTP1:
		good = util.NewHTTPServer(t, &PathHTTPHandler{})
		bad = util.NewHTTPServer(t, &BadHTTPHandler{})
	case protocol.HTTP2:
		good = util.NewUpstreamHTTP2(t, app1, &PathHTTPHandler{})
		bad = util.NewUpstreamHTTP2(t, app2, &BadHTTPHandler{})
	case protocol.SofaRPC:
		good = util.NewRPCServer(t, app1, util.Bolt1)
		bad = util.RPCServer{
			Client:         util.NewRPCClient(t, "rpcClient", util.Bolt1),
			Name:           app2,
			UpstreamServer: util.NewUpstreamServer(t, app2, ServeBadBoltV1),
		}
	}
	tc := NewTestCase(t, serverProto, meshProto, util.NewRPCServer(t, "", util.Bolt1)) // Empty RPC server for get rpc client
	return &RetryCase{
		TestCase:   tc,
		GoodServer: good,
		BadServer:  bad,
		BadIsClose: isClose,
	}
}
func (c *RetryCase) StartProxy() {
	c.GoodServer.GoServe()
	c.BadServer.GoServe()
	app1 := c.GoodServer.Addr()
	app2 := c.BadServer.Addr()
	if c.BadIsClose {
		c.BadServer.Close()
	}
	clientMeshAddr := util.CurrentMeshAddr()
	c.ClientMeshAddr = clientMeshAddr
	cfg := util.CreateProxyMesh(clientMeshAddr, []string{app1, app2}, c.AppProtocol)
	mesh := mosn.NewMosn(cfg)
	mesh.Start()
	c.DeferFinishCase(func() {
		c.GoodServer.Close()
		if !c.BadIsClose {
			c.BadServer.Close()
		}
		mesh.Close()
	})
	time.Sleep(1 * time.Second) //wait server and mesh start

}

func (c *RetryCase) Start(tls bool) {
	c.GoodServer.GoServe()
	c.BadServer.GoServe()
	app1 := c.GoodServer.Addr()
	app2 := c.BadServer.Addr()
	if c.BadIsClose {
		c.BadServer.Close()
	}
	clientMeshAddr := util.CurrentMeshAddr()
	c.ClientMeshAddr = clientMeshAddr
	serverMeshAddr := util.CurrentMeshAddr()
	cfg := util.CreateMeshToMeshConfig(clientMeshAddr, serverMeshAddr, c.AppProtocol, c.MeshProtocol, []string{app1, app2}, tls)
	mesh := mosn.NewMosn(cfg)
	mesh.Start()
	c.DeferFinishCase(func() {
		c.GoodServer.Close()
		if !c.BadIsClose {
			c.BadServer.Close()
		}
		mesh.Close()
	})
	time.Sleep(1 * time.Second) //wait server and mesh start
}

type PathHTTPHandler struct{}

func (h *PathHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	if strings.Trim(r.URL.Path, "/") != HTTPTestPath {
		w.WriteHeader(http.StatusInternalServerError)
	}
	fmt.Fprintf(w, "\nRequestId:%s\n", r.Header.Get("Requestid"))
}

// BadServer Handler
type BadHTTPHandler struct{}

func (h *BadHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, "\nRequestId:%s\n", r.Header.Get("Requestid"))
}

func ServeBadBoltV1(t *testing.T, conn net.Conn) {
	response := func(iobuf types.IoBuffer) ([]byte, bool) {
		cmd, _ := codec.BoltCodec.Decode(nil, iobuf)
		if cmd == nil {
			return nil, false
		}
		if req, ok := cmd.(*sofarpc.BoltRequest); ok {
			resp := util.BuildBoltV1Response(req)
			resp.ResponseStatus = sofarpc.RESPONSE_STATUS_SERVER_EXCEPTION
			iobufresp, err := codec.BoltCodec.Encode(nil, resp)
			if err != nil {
				t.Errorf("Build response error: %v\n", err)
				return nil, true
			}
			return iobufresp.Bytes(), true
		}
		return nil, true
	}
	util.ServeSofaRPC(t, conn, response)
}

func TestRetry(t *testing.T) {
	util.StartRetry = true
	defer func() {
		util.StartRetry = false
	}()
	testCases := []*RetryCase{
		// A server reponse not success
		NewRetryCase(t, protocol.HTTP1, protocol.HTTP1, false),
		NewRetryCase(t, protocol.HTTP1, protocol.HTTP2, false),
		NewRetryCase(t, protocol.HTTP2, protocol.HTTP1, false),
		NewRetryCase(t, protocol.HTTP2, protocol.HTTP2, false),
		NewRetryCase(t, protocol.SofaRPC, protocol.HTTP1, false),
		NewRetryCase(t, protocol.SofaRPC, protocol.HTTP2, false),
		NewRetryCase(t, protocol.SofaRPC, protocol.SofaRPC, false),
		// A server is shutdown
		NewRetryCase(t, protocol.HTTP1, protocol.HTTP1, true),
		NewRetryCase(t, protocol.HTTP1, protocol.HTTP2, true),
		NewRetryCase(t, protocol.HTTP2, protocol.HTTP2, true),
		// HTTP2 and SofaRPC will create connection to upstream before send request to upstream
		// If upstream is closed, it will failed directly, and we cannot do a retry before we send a request to upstream
		/*
			NewRetryCase(t, protocol.HTTP2, protocol.HTTP1, true),
			NewRetryCase(t, protocol.HTTP2, protocol.HTTP2, true),
			NewRetryCase(t, protocol.SofaRPC, protocol.HTTP1, true),
			NewRetryCase(t, protocol.SofaRPC, protocol.HTTP2, true),
			NewRetryCase(t, protocol.SofaRPC, protocol.SofaRPC, true),
		*/
	}
	for i, tc := range testCases {
		t.Logf("start case #%d\n", i)
		tc.Start(false)
		// at least run twice
		go tc.RunCase(2, 0)
		select {
		case err := <-tc.C:
			if err != nil {
				t.Errorf("[ERROR MESSAGE] #%d %v to mesh %v test failed, error: %v\n", i, tc.AppProtocol, tc.MeshProtocol, err)
			}
		case <-time.After(15 * time.Second):
			t.Errorf("[ERROR MESSAGE] #%d %v to mesh %v hang\n", i, tc.AppProtocol, tc.MeshProtocol)
		}
		tc.FinishCase()
	}
}

func TestRetryProxy(t *testing.T) {
	util.StartRetry = true
	defer func() {
		util.StartRetry = false
	}()
	testCases := []*RetryCase{
		NewRetryCase(t, protocol.HTTP1, protocol.HTTP1, false),
		NewRetryCase(t, protocol.HTTP2, protocol.HTTP2, false),
		NewRetryCase(t, protocol.SofaRPC, protocol.SofaRPC, false),
		//NewRetryCase(t, protocol.HTTP1, protocol.HTTP1, true),
		//NewRetryCase(t, protocol.HTTP2, protocol.HTTP2, true),
		//NewRetryCase(t, protocol.SofaRPC, protocol.SofaRPC, true),
	}
	for i, tc := range testCases {
		t.Logf("start case #%d\n", i)
		tc.StartProxy()
		go tc.RunCase(10, 0)
		select {
		case err := <-tc.C:
			if err != nil {
				t.Errorf("[ERROR MESSAGE] #%d %v to mesh %v test failed, error: %v\n", i, tc.AppProtocol, tc.MeshProtocol, err)
			}
		case <-time.After(30 * time.Second):
			t.Errorf("[ERROR MESSAGE] #%d %v to mesh %v hang\n", i, tc.AppProtocol, tc.MeshProtocol)
		}
		tc.FinishCase()

	}
}
