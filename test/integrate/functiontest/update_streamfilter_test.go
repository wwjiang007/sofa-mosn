package functiontest

import (
	"testing"
	"time"

	"mosn.io/mosn/pkg/config"
	"mosn.io/mosn/pkg/mosn"
	"mosn.io/mosn/pkg/protocol"
	"mosn.io/mosn/pkg/protocol/rpc/sofarpc"
	"mosn.io/mosn/pkg/server"
	"mosn.io/mosn/test/util"
)

// Test Update stream filters by LDS can effect the connections that created before.
// we use some codes in faultinejct_test.go
// client->mosn->server
// protocol independent case
func TestUpdateStreamFilters(t *testing.T) {
	server.ResetAdapter()
	// start a server
	appAddr := "127.0.0.1:8080"
	server := util.NewRPCServer(t, appAddr, util.Bolt1)
	server.GoServe()
	defer server.Close()
	// create mosn without stream filters
	clientMeshAddr := util.CurrentMeshAddr()
	cfg := util.CreateProxyMesh(clientMeshAddr, []string{appAddr}, protocol.SofaRPC)
	mesh := mosn.NewMosn(cfg)
	go mesh.Start()
	defer mesh.Close()
	time.Sleep(5 * time.Second)
	// send a request to mosn, create connection between mosns
	rpc, ok := server.(*util.RPCServer)
	if !ok {
		t.Fatal("not a expected rpc server")
	}
	clt := rpc.Client
	if err := clt.Connect(clientMeshAddr); err != nil {
		t.Fatalf("create connection to mosn failed, %v", err)
	}
	defer clt.Close()
	clt.SendRequestWithData("testdata")
	if !util.WaitMapEmpty(&clt.Waits, 2*time.Second) {
		t.Fatal("no expected response")
	}
	// add stream filters
	if err := updateListener(cfg, MakeFaultStr(500, 0)); err != nil {
		t.Fatalf("update listener failed, error: %v", err)
	}
	// set expected status
	clt.ExpectedStatus = sofarpc.RESPONSE_STATUS_UNKNOWN
	// send request to verify the stream filters is valid
	clt.SendRequestWithData("testdata")
	if !util.WaitMapEmpty(&clt.Waits, 2*time.Second) {
		t.Fatal("no expected response")
	}
	// update stream filters
	if err := updateListener(cfg, MakeFaultStr(200, 0)); err != nil {
		t.Fatalf("update listener failed, error: %v", err)
	}
	// verify stream fllters
	clt.ExpectedStatus = sofarpc.RESPONSE_STATUS_SUCCESS
	clt.SendRequestWithData("testdata")
	if !util.WaitMapEmpty(&clt.Waits, 2*time.Second) {
		t.Fatal("no expected response")
	}

}

// call mosn LDS API
func updateListener(cfg *config.MOSNConfig, faultstr string) error {
	// reset stream filters
	cfg.Servers[0].Listeners[0].StreamFilters = cfg.Servers[0].Listeners[0].StreamFilters[:0]
	AddFaultInject(cfg, "proxyListener", faultstr)
	// get config
	lc := cfg.Servers[0].Listeners[0]
	streamFilterFactories := config.GetStreamFilters(lc.StreamFilters)
	// nil network filters, nothing changed
	return server.GetListenerAdapterInstance().AddOrUpdateListener("", &lc, nil, streamFilterFactories)
}
