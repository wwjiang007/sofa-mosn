package integrate

import (
	"context"
	"testing"
	"time"

	"mosn.io/mosn/pkg/module/http2"
	"mosn.io/mosn/pkg/mosn"
	"mosn.io/mosn/pkg/protocol"
	_ "mosn.io/mosn/pkg/protocol/rpc/sofarpc/codec"
	_ "mosn.io/mosn/pkg/protocol/rpc/sofarpc/conv"
	"mosn.io/mosn/pkg/stream"
	_ "mosn.io/mosn/pkg/stream/http"
	_ "mosn.io/mosn/pkg/stream/http2"
	_ "mosn.io/mosn/pkg/stream/sofarpc"
	_ "mosn.io/mosn/pkg/stream/xprotocol"
	"mosn.io/mosn/pkg/types"
	"mosn.io/mosn/test/util"
)

func (c *TestCase) StartAuto(tls bool) {
	c.AppServer.GoServe()
	appAddr := c.AppServer.Addr()
	clientMeshAddr := util.CurrentMeshAddr()
	c.ClientMeshAddr = clientMeshAddr
	serverMeshAddr := util.CurrentMeshAddr()
	cfg := util.CreateMeshToMeshConfig(clientMeshAddr, serverMeshAddr, protocol.Auto, protocol.Auto, []string{appAddr}, tls)
	mesh := mosn.NewMosn(cfg)
	mesh.Start()
	c.DeferFinishCase(func() {
		c.AppServer.Close()
		mesh.Close()
	})
	time.Sleep(1 * time.Second) //wait server and mesh start
}

func TestAuto(t *testing.T) {
	testCases := []*TestCase{
		NewTestCase(t, protocol.HTTP2, protocol.HTTP2, util.NewUpstreamHTTP2WithAnyPort(t, nil)),
		NewTestCase(t, protocol.HTTP1, protocol.HTTP1, util.NewHTTPServer(t, nil)),
	}
	for i, tc := range testCases {
		t.Logf("start case #%d\n", i)
		tc.StartAuto(false)
		go tc.RunCase(5, 0)
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

func TestAutoTLS(t *testing.T) {
	testCases := []*TestCase{
		NewTestCase(t, protocol.HTTP2, protocol.HTTP2, util.NewUpstreamHTTP2WithAnyPort(t, nil)),
		NewTestCase(t, protocol.HTTP1, protocol.HTTP1, util.NewHTTPServer(t, nil)),
	}
	for i, tc := range testCases {
		t.Logf("start case #%d\n", i)
		tc.StartAuto(true)
		go tc.RunCase(5, 0)
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

func TestProtocolHttp2(t *testing.T) {
	var prot types.Protocol
	var magic string
	var err error

	magic = http2.ClientPreface
	prot, err = stream.SelectStreamFactoryProtocol(context.Background(), "", []byte(magic))
	if prot != protocol.HTTP2 {
		t.Errorf("[ERROR MESSAGE] type error magic : %v\n", magic)
	}

	len := len(http2.ClientPreface)
	prot, err = stream.SelectStreamFactoryProtocol(context.Background(), "", []byte(magic)[0:len-1])
	if err != stream.EAGAIN {
		t.Errorf("[ERROR MESSAGE] type error protocol :%v", err)
	}

	prot, err = stream.SelectStreamFactoryProtocol(context.Background(), "", []byte("helloworld"))
	if err != stream.FAILED {
		t.Errorf("[ERROR MESSAGE] type error protocol :%v", err)
	}
}

func TestProtocolHttp1(t *testing.T) {
	var prot types.Protocol
	var magic string
	var err error

	magic = "GET"
	prot, err = stream.SelectStreamFactoryProtocol(context.Background(), "", []byte(magic))
	if prot != protocol.HTTP1 {
		t.Errorf("[ERROR MESSAGE] type error magic : %v\n", magic)
	}

	magic = "POST"
	prot, err = stream.SelectStreamFactoryProtocol(context.Background(), "", []byte(magic))
	if prot != protocol.HTTP1 {
		t.Errorf("[ERROR MESSAGE] type error magic : %v\n", magic)
	}

	magic = "POS"
	prot, err = stream.SelectStreamFactoryProtocol(context.Background(), "", []byte(magic))
	if err != stream.EAGAIN {
		t.Errorf("[ERROR MESSAGE] type error protocol :%v", err)
	}

	magic = "PPPPPPP"
	prot, err = stream.SelectStreamFactoryProtocol(context.Background(), "", []byte(magic))
	if err != stream.FAILED {
		t.Errorf("[ERROR MESSAGE] type error protocol :%v", err)
	}
}
