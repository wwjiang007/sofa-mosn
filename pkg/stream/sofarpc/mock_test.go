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

package sofarpc

import (
	"context"
	"net"
	"time"

	"mosn.io/mosn/pkg/buffer"
	"mosn.io/mosn/pkg/network"
	"mosn.io/mosn/pkg/protocol/rpc/sofarpc"
	"mosn.io/mosn/pkg/types"
)

// a mock server for handle heart beat request
type mockServer struct {
	ln    net.Listener
	stop  chan struct{}
	codec types.ProtocolEngine
	delay time.Duration
}

func newMockServer(delay time.Duration) (*mockServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	return &mockServer{
		ln:    ln,
		stop:  make(chan struct{}),
		codec: sofarpc.Engine(),
		delay: delay,
	}, nil
}

func (s *mockServer) AddrString() string {
	return s.ln.Addr().String()
}

func (s *mockServer) Close() error {
	close(s.stop)
	return s.ln.Close()
}

func (s *mockServer) GoServe() {
	go func() {
		for {
			conn, err := s.ln.Accept()
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Temporary() {
					continue
				}
				return
			}
			go s.HandleConn(conn)
		}
	}()
}

func (s *mockServer) HandleConn(conn net.Conn) {
	iobuf := buffer.NewIoBuffer(10240)
	for {
		select {
		case <-s.stop:
			conn.Close()
			return
		default:
			now := time.Now()
			conn.SetReadDeadline(now.Add(30 * time.Second))
			buf := make([]byte, 10240)
			n, err := conn.Read(buf)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Temporary() {
					continue
				}
				return
			}
			if n > 0 {
				iobuf.Write(buf[:n])
				for iobuf.Len() > 1 {
					resp := s.Reply(iobuf)
					if resp != nil {
						conn.Write(resp)
					}
				}
			}
		}
	}
}

func (s *mockServer) Reply(iobuf types.IoBuffer) []byte {
	if s.delay != 0 {
		time.Sleep(s.delay)
	}
	cmd, _ := s.codec.Decode(context.Background(), iobuf)
	if cmd == nil {
		return nil
	}
	rpccmd := cmd.(sofarpc.SofaRpcCmd)
	if rpccmd.CommandCode() == sofarpc.HEARTBEAT {
		ack := sofarpc.NewHeartbeatAck(rpccmd.ProtocolCode())
		ack.SetRequestID(rpccmd.RequestID())
		resp, err := s.codec.Encode(context.Background(), ack)
		if err != nil {
			return nil
		}
		return resp.Bytes()
	}
	return nil
}

type mockClusterInfo struct {
	name  string
	limit uint32
	types.ClusterInfo
}

func (ci *mockClusterInfo) Name() string {
	return ci.name
}

func (ci *mockClusterInfo) ConnBufferLimitBytes() uint32 {
	return ci.limit
}

func (ci *mockClusterInfo) SourceAddress() net.Addr {
	return nil
}

func (ci *mockClusterInfo) ConnectTimeout() time.Duration {
	return network.DefaultConnectTimeout
}
