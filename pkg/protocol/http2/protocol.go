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
"errors"

"github.com/alipay/sofa-mosn/pkg/log"
"github.com/alipay/sofa-mosn/pkg/types"
)

type protocols struct {
}

func NewProtocols() types.Protocols {
	return &protocols{}
}

type Header struct {
	data types.IoBuffer
}

func (h *Header) Set (key, value string) {
}

func (h *Header) Get (key string) (string, bool) {
}

func (h *Header) Del (key string) {
}

func (h *Header) Range (f func(key, value string) bool) {
}

func (p *protocols) EncodeHeaders(context context.Context, headers types.HeaderMap) (types.IoBuffer, error) {
}

func (p *protocols) EncodeData(context context.Context, data types.IoBuffer) types.IoBuffer {
	return data
}

func (p *protocols) EncodeTrailers(context context.Context, trailers types.HeaderMap) types.IoBuffer {
	return nil
}

func (p *protocols) Decode(context context.Context, data types.IoBuffer, filter types.DecodeFilter) {
	for data.Len() > 1 {
		filter.OnDecodeData("", data, false)
	}
}