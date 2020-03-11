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

package dubbo

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"

	"mosn.io/mosn/pkg/protocol/rpc/xprotocol"
	"mosn.io/mosn/pkg/types"
)

const (
	XPROTOCOL_PLUGIN_DUBBO = "dubbo"
)

func init() {
	xprotocol.Register(XPROTOCOL_PLUGIN_DUBBO, &pluginDubboFactory{})
}

type pluginDubboFactory struct{}

func (ref *pluginDubboFactory) CreateSubProtocolCodec(context context.Context) xprotocol.Multiplexing {
	return NewRPCDubbo()
}

type rpcDubbo struct{}

func NewRPCDubbo() xprotocol.Tracing {
	return &rpcDubbo{}
}

/**
 * Dubbo protocol
 * Request & Response: (byte)
 * 0           1           2           3           4           5           6           7           8
 * +-----------+-----------+-----------+-----------+-----------+-----------+-----------+-----------+
 * |magic high | magic low |  flag     | status    |               id                              |
 * +-----------+-----------+-----------+-----------+-----------+-----------+-----------+-----------+
 * |      id                                       |               data length                     |
 * +-----------+-----------+-----------+-----------+-----------+-----------+-----------+-----------+
 * |                               payload                                                         |
 * +-----------------------------------------------------------------------------------------------+
 * magic: 0xdabb
 *
 * flag: (bit offset)
 * 0           1           2           3           4           5           6           7           8
 * +-----------+-----------+-----------+-----------+-----------+-----------+-----------+-----------+
 * |              serialization id                             |  event    | two way   |   req/rsp |
 * +-----------+-----------+-----------+-----------+-----------+-----------+-----------+-----------+
 * event: 1 mean ping
 * two way: 1 mean req & rsp pair
 * req/rsp: 1 mean req
 */

const (
	DUBBO_HEADER_LEN = 16
	DUBBO_ID_LEN     = 8

	DUBBO_MAGIC_IDX    = 0
	DUBBO_FLAG_IDX     = 2
	DUBBO_STATUS_IDX   = 3
	DUBBO_ID_IDX       = 4
	DUBBO_DATA_LEN_IDX = 12
)

var DUBBO_MAGIC_TAG []byte = []byte{0xda, 0xbb}

func getDubboLen(data []byte) int {
	rslt, bodyLen := isValidDubboData(data)
	if rslt == false {
		return -1
	}
	return DUBBO_HEADER_LEN + bodyLen
}

func (d *rpcDubbo) SplitFrame(data []byte) [][]byte {
	var frames [][]byte
	start := 0
	dataLen := len(data)
	for true {
		frameLen := getDubboLen(data[start:])
		if frameLen > 0 && dataLen >= frameLen {
			// there is one valid xprotocol request
			frames = append(frames, data[start:(start+frameLen)])
			start += frameLen
			dataLen -= frameLen
			if dataLen == 0 {
				// finish
				//fmt.Printf("[SplitFrame] finish\n")
				break
			}
		} else {
			// invalid data
			fmt.Printf("[SplitFrame] over! frameLen=%d, dataLen=%d. frame_cnt=%d\n", frameLen, dataLen, len(frames))
			break
		}
	}
	return frames
}

func isValidDubboData(data []byte) (bool, int) {
	//return true
	dataLen := len(data)
	if dataLen < DUBBO_HEADER_LEN {
		return false, -1
	}
	if bytes.Compare(data[DUBBO_MAGIC_IDX:DUBBO_FLAG_IDX], DUBBO_MAGIC_TAG) != 0 {
		// illegal
		fmt.Printf("[isValidDubboData] illegal(len=%d): %v\n", dataLen, data)
		return false, -1
	}
	bodyLen := binary.BigEndian.Uint32(data[12:(12 + 4)])
	frameLen := uint32(DUBBO_HEADER_LEN) + bodyLen
	if uint32(dataLen) < frameLen {
		return false, -1
	}
	return true, int(bodyLen)
}

func (d *rpcDubbo) GetStreamID(data []byte) string {
	rslt, _ := isValidDubboData(data)
	if rslt == false {
		return ""
	}
	reqIDRaw := data[DUBBO_ID_IDX:(DUBBO_ID_IDX + DUBBO_ID_LEN)]
	reqID := binary.BigEndian.Uint64(reqIDRaw)
	reqIDStr := fmt.Sprintf("%d", reqID)
	return reqIDStr
}

func (d *rpcDubbo) SetStreamID(data []byte, streamID string) []byte {
	rslt, _ := isValidDubboData(data)
	if rslt == false {
		return data
	}

	reqID, err := strconv.ParseInt(streamID, 10, 64)
	if err != nil {
		return data
	}
	buf := bytes.Buffer{}
	err = binary.Write(&buf, binary.BigEndian, reqID)
	if err != nil {
		return data
	}
	reqIDStr := buf.Bytes()
	reqIDStrLen := len(reqIDStr)
	fmt.Printf("src=%s, len=%d, reqid:%v\n", streamID, reqIDStrLen, reqIDStr)

	start := DUBBO_ID_IDX

	for i := 0; i < DUBBO_ID_LEN && i <= reqIDStrLen; i++ {
		data[start+i] = reqIDStr[i]
	}
	return data
}

func (d *rpcDubbo) BuildHeartbeatResp(headers types.HeaderMap) []byte {
	requestId, ok := headers.Get(types.HeaderXprotocolStreamId)
	if !ok {
		return nil
	}

	strEchoBytes, _ := hex.DecodeString("dabb22140000000000000001000000024e4e")
	// replace dubboId
	return d.SetStreamID(strEchoBytes, requestId)
}

type serviceNameFuncModel func(data []byte) string
type methodNameFuncModel func(data []byte) string
type metaFuncModel func(data []byte) map[string]string

var serviceNameFunc serviceNameFuncModel
var methodNameFunc methodNameFuncModel
var metaFunc metaFuncModel

func (d *rpcDubbo) GetServiceName(data []byte) string {
	if serviceNameFunc != nil {
		return serviceNameFunc(data)
	}
	return ""
}

func (d *rpcDubbo) GetMethodName(data []byte) string {
	if methodNameFunc != nil {
		return methodNameFunc(data)
	}
	return ""
}

func (d *rpcDubbo) GetMetas(data []byte) map[string]string {
	if metaFunc != nil {
		return metaFunc(data)
	}
	return nil
}
