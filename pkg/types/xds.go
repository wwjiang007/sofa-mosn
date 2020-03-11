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

package types

import (
	"strings"

	"github.com/gogo/protobuf/types"
)

const serviceMetaSeparator = ":"

// XdsInfo The xds start parameters
type XdsInfo struct {
	ServiceCluster string
	ServiceNode    string
	Metadata       *types.Struct
}

var globalXdsInfo = &XdsInfo{}

// GetGlobalXdsInfo returns pointer of globalXdsInfo
func GetGlobalXdsInfo() *XdsInfo {
	return globalXdsInfo
}

// InitXdsFlags init globalXdsInfo
func InitXdsFlags(serviceCluster, serviceNode string, serviceMeta []string) {
	globalXdsInfo.ServiceCluster = serviceCluster
	globalXdsInfo.ServiceNode = serviceNode
	globalXdsInfo.Metadata = &types.Struct{
		Fields: map[string]*types.Value{},
	}

	for _, keyValue := range serviceMeta {
		keyValueSep := strings.SplitN(keyValue, serviceMetaSeparator, 2)
		if len(keyValueSep) != 2 {
			continue
		}
		key := keyValueSep[0]
		value := keyValueSep[1]

		globalXdsInfo.Metadata.Fields[key] = &types.Value{
			Kind: &types.Value_StringValue{
				StringValue: value,
			},
		}
	}
}
