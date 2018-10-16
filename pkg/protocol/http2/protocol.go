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

package http2

import (
	"context"
	"github.com/alipay/sofa-mosn/pkg/types"
)

const http2frameHeaderLen = 9
const clientPreface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

type protocols struct {
	preface     bool
	parseHeader bool
	header [http2frameHeaderLen]byte
}

func NewProtocols() types.Protocols {
	return &protocols{
		preface:     false,
	}
}

func (p *protocols) EncodeHeaders(context context.Context, headers types.HeaderMap) (types.IoBuffer, error) {
	return nil, nil
}

func (p *protocols) EncodeData(context context.Context, data types.IoBuffer) types.IoBuffer {
	return data
}

func (p *protocols) EncodeTrailers(context context.Context, trailers types.HeaderMap) types.IoBuffer {
	return nil
}

func (p *protocols) Decode(context context.Context, data types.IoBuffer, filter types.DecodeFilter) {
	buf := data.Bytes()
	size := data.Len()
	off := 0
	if !p.preface {
		if size >= len(clientPreface) {
			filter.OnDecodeData("", data, false)
			off += len(clientPreface)
			p.preface = true
			p.parseHeader = true
		} else {
			return
		}
	}

	for {
		if p.parseHeader {
			if off + http2frameHeaderLen > size {
				break
			}
			filter.OnDecodeData("", data, false)
			p.parseHeader = false
			copyHeader(&p.header, buf[off:])
			off += http2frameHeaderLen
		} else {
			b := p.header
			length := (uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2]))
			if off + int(length) > size {
				break
			}
			filter.OnDecodeData("", data, false)
			p.parseHeader = true
			off += int(length)
		}
	}
}

func copyHeader(header *[http2frameHeaderLen]byte, b []byte) {
	for i := 0; i < http2frameHeaderLen; i++ {
           header[i] = b[i]
	}
}
