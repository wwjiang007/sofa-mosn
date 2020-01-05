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

package proxy

import (
	"context"
	"testing"
	"time"

	"mosn.io/mosn/pkg/api/v2"
	"mosn.io/mosn/pkg/buffer"
	"mosn.io/mosn/pkg/network"
	"mosn.io/mosn/pkg/protocol"
	"mosn.io/mosn/pkg/trace"
	"mosn.io/mosn/pkg/types"

	mosnctx "mosn.io/mosn/pkg/context"
)

func TestDownstream_FinishTracing_NotEnable(t *testing.T) {
	ds := downStream{context: context.Background()}
	ds.finishTracing()
	span := trace.SpanFromContext(context.Background())
	if span != nil {
		t.Error("Span is not nil")
	}
}

func TestDownstream_FinishTracing_Enable(t *testing.T) {
	trace.Enable()
	ds := downStream{context: context.Background()}
	ds.finishTracing()
	span := trace.SpanFromContext(context.Background())
	if span != nil {
		t.Error("Span is not nil")
	}
}

func TestDownstream_FinishTracing_Enable_SpanIsNotNil(t *testing.T) {
	trace.Enable()
	err := trace.Init("SOFATracer", nil)
	if err != nil {
		t.Error("init tracing driver failed: ", err)
	}

	span := trace.Tracer(mockProtocol).Start(context.Background(), nil, time.Now())
	ctx := mosnctx.WithValue(context.Background(), types.ContextKeyActiveSpan, span)
	requestInfo := &network.RequestInfo{}
	ds := downStream{context: ctx, requestInfo: requestInfo}
	ds.finishTracing()

	span = trace.SpanFromContext(ctx)
	if span == nil {
		t.Error("Span is nil")
	}
	mockSpan := span.(*mockSpan)
	if !mockSpan.finished {
		t.Error("Span is not finish")
	}
}

func TestDirectResponse(t *testing.T) {
	testCases := []struct {
		client *mockResponseSender
		route  *mockRoute
		check  func(t *testing.T, sender *mockResponseSender)
	}{
		// without body
		{
			client: &mockResponseSender{},
			route: &mockRoute{
				direct: &mockDirectRule{
					status: 500,
				},
			},
			check: func(t *testing.T, client *mockResponseSender) {
				if client.headers == nil {
					t.Fatal("want to receive a header response")
				}
				if code, ok := client.headers.Get(types.HeaderStatus); !ok || code != "500" {
					t.Error("response status code not expected")
				}
			},
		},
		// with body
		{
			client: &mockResponseSender{},
			route: &mockRoute{
				direct: &mockDirectRule{
					status: 400,
					body:   "mock 400 response",
				},
			},
			check: func(t *testing.T, client *mockResponseSender) {
				if client.headers == nil {
					t.Fatal("want to receive a header response")
				}
				if code, ok := client.headers.Get(types.HeaderStatus); !ok || code != "400" {
					t.Error("response status code not expected")
				}
				if client.data == nil {
					t.Fatal("want to receive a body response")
				}
				if client.data.String() != "mock 400 response" {
					t.Error("response  data not expected")
				}
			},
		},
	}
	for _, tc := range testCases {
		s := &downStream{
			proxy: &proxy{
				config: &v2.Proxy{},
				routersWrapper: &mockRouterWrapper{
					routers: &mockRouters{
						route: tc.route,
					},
				},
				clusterManager: &mockClusterManager{},
				readCallbacks:  &mockReadFilterCallbacks{},
				stats:          globalStats,
				listenerStats:  newListenerStats("test"),
			},
			responseSender: tc.client,
			requestInfo:    &network.RequestInfo{},
		}
		// event call Receive Headers
		// trigger direct response
		s.OnReceive(context.Background(), protocol.CommonHeader{}, buffer.NewIoBuffer(1), nil)
		// check
		time.Sleep(100 * time.Millisecond)
		tc.check(t, tc.client)
	}
}

func TestOnewayHijack(t *testing.T) {
	initGlobalStats()
	proxy := &proxy{
		config:         &v2.Proxy{},
		routersWrapper: nil,
		clusterManager: &mockClusterManager{},
		readCallbacks:  &mockReadFilterCallbacks{},
		stats:          globalStats,
		listenerStats:  newListenerStats("test"),
	}
	s := newActiveStream(context.Background(), proxy, nil, nil)

	// not routes, sendHijack
	s.OnReceive(context.Background(), protocol.CommonHeader{}, buffer.NewIoBuffer(1), nil)
	// check
	time.Sleep(100 * time.Millisecond)
	if s.downstreamCleaned != 1 {
		t.Errorf("downStream should be cleaned")
	}
}

func TestIsRequestFailed(t *testing.T) {
	testCases := []struct {
		Flags    []types.ResponseFlag
		Expected bool
	}{
		{
			Flags:    []types.ResponseFlag{types.NoHealthyUpstream},
			Expected: true,
		},
		{
			Flags:    []types.ResponseFlag{types.UpstreamRequestTimeout},
			Expected: false,
		},
		{
			Flags:    []types.ResponseFlag{types.UpstreamRemoteReset},
			Expected: false,
		},
		{
			Flags:    []types.ResponseFlag{types.NoRouteFound},
			Expected: true,
		},
		{
			Flags:    []types.ResponseFlag{types.DelayInjected},
			Expected: false,
		},
		{
			Flags:    []types.ResponseFlag{types.FaultInjected},
			Expected: true,
		},
		{
			Flags:    []types.ResponseFlag{types.RateLimited},
			Expected: true,
		},
		{
			Flags:    []types.ResponseFlag{types.DelayInjected, types.FaultInjected},
			Expected: true,
		},
		{
			Flags:    []types.ResponseFlag{types.UpstreamRequestTimeout, types.NoHealthyUpstream},
			Expected: true,
		},
		{
			Flags:    []types.ResponseFlag{types.UpstreamConnectionTermination, types.UpstreamRemoteReset},
			Expected: false,
		},
	}
	for idx, tc := range testCases {
		s := &downStream{
			requestInfo: network.NewRequestInfo(),
		}
		for _, f := range tc.Flags {
			s.requestInfo.SetResponseFlag(f)
		}
		if s.isRequestFailed() != tc.Expected {
			t.Errorf("case no.%d is not expected, flag: %v, expected: %v", idx, tc.Flags, tc.Expected)
		}
	}
}
