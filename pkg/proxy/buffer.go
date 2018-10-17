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
	"sync"
	"github.com/alipay/sofa-mosn/pkg/buffer"
	"github.com/alipay/sofa-mosn/pkg/network"
	"context"
)

//type proxyBufferCtx struct{}
//
//func (ctx proxyBufferCtx) Name() int {
//	return buffer.Proxy
//}
//
//func (ctx proxyBufferCtx) New() interface{} {
//	return new(proxyBuffers)
//}
//
//func (ctx proxyBufferCtx) Reset(i interface{}) {
//	buf, _ := i.(*proxyBuffers)
//	*buf = proxyBuffers{}
//}
//
//type proxyBuffers struct {
//	stream  downStream
//	request upstreamRequest
//	info    network.RequestInfo
//}
//func proxyBuffersByContext(ctx context.Context) *proxyBuffers {
//	poolCtx := buffer.PoolContext(ctx)
//	return poolCtx.Find(proxyBufferCtx{}, nil).(*proxyBuffers)
//}

var (
	// TODO separate request info alloc
	dsPool = sync.Pool{
		New: func() interface{} {
			return &downStream{
				requestInfo: network.NewRequestInfo(),
			}
		},
	}
	usPool = sync.Pool{
		New: func() interface{} {
			return &upstreamRequest{}
		},
	}
)

// ~ Reusable
func (ds *downStream) Free() {
	// reset fields
	if info, ok := ds.requestInfo.(*network.RequestInfo); ok {
		*info = network.RequestInfo{}
	} else {
		ds.requestInfo = network.NewRequestInfo()
	}

	*ds = downStream{
		requestInfo: ds.requestInfo,
	}
	// return to pool
	dsPool.Put(ds)
}

// ~ Reusable
func (us *upstreamRequest) Free() {
	// reset fields
	*us = upstreamRequest{}
	// return to pool
	usPool.Put(us)
}

func allocDownstream(ctx context.Context) *downStream {
	// alloc from pool
	req := dsPool.Get().(*downStream)

	// append to buffer context's free list
	buffer.AppendReusable(ctx, req)

	return req
}

func allocUpstream(ctx context.Context) *upstreamRequest {
	// alloc from pool
	resp := usPool.Get().(*upstreamRequest)

	// append to buffer context's free list
	buffer.AppendReusable(ctx, resp)

	return resp
}

