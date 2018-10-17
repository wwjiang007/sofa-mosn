package sofarpc

import (
	"github.com/alipay/sofa-mosn/pkg/buffer"
	"sync"
	"context"
)

//type SofaProtocolBufferCtx struct{}
//
//func (ctx SofaProtocolBufferCtx) Name() int {
//	return buffer.SofaProtocol
//}
//
//func (ctx SofaProtocolBufferCtx) New() interface{} {
//	buffer := new(SofaProtocolBuffers)
//	return buffer
//}
//
//func (ctx SofaProtocolBufferCtx) Reset(i interface{}) {
//	buf, _ := i.(*SofaProtocolBuffers)
//	buf.BoltReq = BoltRequest{}
//	buf.BoltRsp = BoltResponse{}
//	buf.BoltEncodeReq = BoltRequest{}
//	buf.BoltEncodeRsp = BoltResponse{}
//}
//
//type SofaProtocolBuffers struct {
//	BoltReq       BoltRequest
//	BoltRsp       BoltResponse
//	BoltEncodeReq BoltRequest
//	BoltEncodeRsp BoltResponse
//}

//func SofaProtocolBuffersByContext(ctx context.Context) *SofaProtocolBuffers {
//	poolCtx := buffer.PoolContext(ctx)
//	return poolCtx.Find(SofaProtocolBufferCtx{}, nil).(*SofaProtocolBuffers)
//}

var (
	// TODO separate map alloc
	reqPool = sync.Pool{
		New: func() interface{} {
			return &BoltRequest{
				RequestHeader: make(map[string]string, 8),
			}
		},
	}
	respPool = sync.Pool{
		New: func() interface{} {
			return &BoltResponse{
				ResponseHeader: make(map[string]string, 8),
			}
		},
	}
)

// ~ Reusable
func (b *BoltRequest) Free() {
	// reset fields
	for k := range b.RequestHeader {
		delete(b.RequestHeader, k)
	}

	*b = BoltRequest{
		RequestHeader: b.RequestHeader,
	}
	// return to pool
	reqPool.Put(b)
}

// ~ Reusable
func (b *BoltResponse) Free() {
	// reset fields
	for k := range b.ResponseHeader {
		delete(b.ResponseHeader, k)
	}

	*b = BoltResponse{
		ResponseHeader: b.ResponseHeader,
	}
	// return to pool
	respPool.Put(b)
}

func AllocReq(ctx context.Context) *BoltRequest {
	// alloc from pool
	req := reqPool.Get().(*BoltRequest)

	// append to buffer context's free list
	buffer.AppendReusable(ctx, req)

	return req
}

func AllocResp(ctx context.Context) *BoltResponse {
	// alloc from pool
	resp := respPool.Get().(*BoltResponse)

	// append to buffer context's free list
	buffer.AppendReusable(ctx, resp)

	return resp
}
