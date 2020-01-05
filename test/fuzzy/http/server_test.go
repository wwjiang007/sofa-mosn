package http

import (
	"testing"
	"time"

	"mosn.io/mosn/pkg/log"
	"mosn.io/mosn/pkg/protocol"
	"mosn.io/mosn/pkg/types"
	"mosn.io/mosn/test/fuzzy"
)

func runClient(t *testing.T, meshAddr string, stop chan struct{}) {
	client := NewHTTPClient(t, meshAddr)
	fuzzy.FuzzyClient(stop, client)
	<-time.After(caseDuration)
	close(stop)
	time.Sleep(5 * time.Second)
	if client.unexpectedCount != 0 {
		t.Errorf("case%d client have unexpected request: %d\n", caseIndex, client.failureCount)
	}
	if client.successCount == 0 || client.failureCount == 0 {
		t.Errorf("case%d client suucess count: %d, failure count: %d\n", caseIndex, client.successCount, client.failureCount)
	}
	log.StartLogger.Infof("[FUZZY TEST] client suucess count: %d, failure count: %d\n", client.successCount, client.failureCount)
}

func TestServerCloseProxy(t *testing.T) {
	caseIndex++
	log.StartLogger.Infof("[FUZZY TEST] HTTP Server Close In ProxyMode  %d", caseIndex)
	serverList := []string{
		"127.0.0.1:8080",
		"127.0.0.1:8081",
		"127.0.0.1:8082",
	}
	stopClient := make(chan struct{})
	stopServer := make(chan struct{})
	meshAddr := fuzzy.CreateMeshProxy(t, stopServer, serverList, protocol.HTTP1)
	servers := CreateServers(t, serverList, stopServer, true)
	fuzzy.FuzzyServer(stopServer, servers, caseDuration/5)
	runClient(t, meshAddr, stopClient)
	close(stopServer)
	// wait server close
	time.Sleep(time.Second)
}

func runServerCloseMeshToMesh(t *testing.T, proto types.Protocol, keepalive bool) {
	serverList := []string{
		"127.0.0.1:8080",
		"127.0.0.1:8081",
		"127.0.0.1:8082",
	}
	stopClient := make(chan struct{})
	stopServer := make(chan struct{})
	meshAddr := fuzzy.CreateMeshCluster(t, stopServer, serverList, protocol.HTTP1, proto)
	servers := CreateServers(t, serverList, stopServer, keepalive)
	fuzzy.FuzzyServer(stopServer, servers, caseDuration/5)
	runClient(t, meshAddr, stopClient)
	close(stopServer)
	// wait server close
	time.Sleep(time.Second)

}

func TestServerCloseToHTTP1(t *testing.T) {
	caseIndex++
	log.StartLogger.Infof("[FUZZY TEST] HTTP Server Close HTTP1 %d", caseIndex)
	runServerCloseMeshToMesh(t, protocol.HTTP1, true)
}

func TestShortConnServerToHTTP1(t *testing.T) {
	caseIndex++
	log.StartLogger.Infof("[FUZZY TEST] HTTP ShortConn Server Close HTTP1 %d", caseIndex)
	runServerCloseMeshToMesh(t, protocol.HTTP1, false)
}

func TestServerCloseToHTTP2(t *testing.T) {
	caseIndex++
	log.StartLogger.Infof("[FUZZY TEST] HTTP Server Close HTTP2 %d", caseIndex)
	runServerCloseMeshToMesh(t, protocol.HTTP2, true)
}
