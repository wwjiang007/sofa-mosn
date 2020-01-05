package directresp

import (
	"errors"
	"testing"
	"time"

	"mosn.io/mosn/pkg/protocol/rpc/sofarpc"
	"mosn.io/mosn/test/lib"
	testlib_sofarpc "mosn.io/mosn/test/lib/sofarpc"
)

func TestBoltv1DirectResponse(t *testing.T) {
	lib.Scenario(t, "Direct response in boltv1", func() {
		var mosn *lib.MosnOperator
		lib.Setup(func() error {
			mosn = lib.StartMosn(ConfigBoltv1)
			time.Sleep(time.Second)
			return nil
		})
		lib.TearDown(func() {
			mosn.Stop()
		})
		lib.Execute("get response from mosn", func() error {
			cltVerify := &testlib_sofarpc.VerifyConfig{
				ExpectedStatus: sofarpc.RESPONSE_STATUS_SUCCESS,
			}
			cfg := testlib_sofarpc.CreateSimpleConfig("127.0.0.1:2045")
			cfg.Verify = cltVerify.Verify

			clt := testlib_sofarpc.NewClient(cfg, 1)
			if !clt.SyncCall() {
				return errors.New("client receive response unexpected")
			}
			return nil
		})
	})
}

const ConfigBoltv1 = `{
        "servers":[
                {
                        "default_log_path":"stdout",
                        "default_log_level": "FATAL",
                         "listeners":[
                                {
                                        "address":"127.0.0.1:2045",
                                        "bind_port": true,
                                        "log_path": "stdout",
                                        "log_level": "FATAL",
                                        "filter_chains": [{
                                                "filters": [
                                                        {
                                                                "type": "proxy",
                                                                "config": {
                                                                        "downstream_protocol": "SofaRpc",
                                                                        "upstream_protocol": "SofaRpc",
                                                                        "router_config_name":"router_direct"
                                                                }
                                                        },
                                                        {
                                                                "type": "connection_manager",
                                                                "config": {
                                                                        "router_config_name":"router_direct",
                                                                        "virtual_hosts":[{
                                                                                "name":"mosn_hosts",
                                                                                "domains": ["*"],
                                                                                "routers": [
                                                                                        {
                                                                                                 "match":{"headers":[{"name":"service","value":".*"}]},
                                                                                                 "direct_response": {
                                                                                                         "status": 200
                                                                                                 }
                                                                                        }
                                                                                ]
                                                                        }]
                                                                }
                                                        }
                                                ]
                                        }]
                                }
                         ]
                }
        ],
        "cluster_manager":{
                "clusters":[
                        {
                                "name": "empty_cluster",
                                "type": "SIMPLE"
                        }
                ]
        }
}`
