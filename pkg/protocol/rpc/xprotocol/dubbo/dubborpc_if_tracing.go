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
	"fmt"
	"strconv"

	hessian "github.com/apache/dubbo-go-hessian2"
	"mosn.io/mosn/pkg/types"
)

// regular
const (
	RESPONSE_WITH_EXCEPTION                  int32 = 0
	RESPONSE_WITH_EXCEPTION_WITH_ATTACHMENTS int32 = 3
)

func init() {
	serviceNameFunc = dubboGetServiceName
	methodNameFunc = dubboGetMethodName
	metaFunc = dubboGetMeta
}

func getSerializeId(flag byte) int {
	return int(flag & 0x1f)
}

func getEventPing(flag byte) bool {
	return (flag & (1 << 5)) != 0
}

func isReqFrame(flag byte) bool {
	return (flag & (1 << 7)) != 0
}

type dubboAttr struct {
	serviceName  string
	methodName   string
	path         string
	version      string
	dubboVersion string
	attachments  map[string]string
}

// unserializeCtl flag for Dubbo unserialize control
type unserializeCtl uint8

const (
	_ unserializeCtl = iota
	unserializeCtlDubboVersion
	unserializeCtlPath
	unserializeCtlVersion
	unserializeCtlMethod
	unserializeCtlArgsTypes
	unserializeCtlAttachments
)

// unSerialize xprotocol dubbo_version + path + version + method + argsTypes ... + attachments
func unSerialize(serializeId int, data []byte, parseCtl unserializeCtl) *dubboAttr {

	if serializeId != 2 {
		// not hessian, do not support
		fmt.Printf("unSerialize: id=%d is not hessian\n", serializeId)
		return nil
	}
	attr := &dubboAttr{}
	decoder := hessian.NewDecoder(data[:])
	var field interface{}
	var err error
	var ok bool
	var str string
	var attachments map[string]string

	field, err = decoder.Decode()
	if err != nil {
		fmt.Printf("unSerialize: Decode dubbo_version fail, err=%v\n", err)
		return nil
	}
	str, ok = field.(string)
	if !ok {
		fmt.Printf("unSerialize: Decode dubbo_version fail, illegal type\n")
		return nil
	}
	attr.dubboVersion = str
	if parseCtl <= unserializeCtlDubboVersion {
		return attr
	}

	field, err = decoder.Decode()
	if err != nil {
		fmt.Printf("unSerialize: Decode path fail, err=%v\n", err)
		return nil
	}
	str, ok = field.(string)
	if !ok {
		fmt.Printf("unSerialize: Decode path fail, illegal type\n")
		return nil
	}
	attr.serviceName = str
	attr.path = str
	if parseCtl <= unserializeCtlPath {
		return attr
	}

	field, err = decoder.Decode()
	if err != nil {
		fmt.Printf("unSerialize: Decode version fail, err=%v\n", err)
		return nil
	}
	str, ok = field.(string)
	if !ok {
		fmt.Printf("unSerialize: Decode version fail, illegal type\n")
		return nil
	}
	attr.version = str
	if parseCtl <= unserializeCtlVersion {
		return attr
	}

	field, err = decoder.Decode()
	if err != nil {
		fmt.Printf("unSerialize: Decode method fail, err=%v\n", err)
		return nil
	}
	str, ok = field.(string)
	if !ok {
		fmt.Printf("unSerialize: Decode method fail, illegal type\n")
		return nil
	}
	attr.methodName = str
	if parseCtl <= unserializeCtlMethod {
		return attr
	}

	field, err = decoder.Decode()
	if err != nil {
		fmt.Printf("unSerialize: Decode argsTypes fail, err=%v\n", err)
		return nil
	}

	ats := hessian.DescRegex.FindAllString(field.(string), -1)
	for i := 0; i < len(ats); i++ {
		_, err = decoder.Decode()
		if err != nil {
			fmt.Printf("unSerialize: Decode argsTypes item fail, err=%v\n", err)
			return nil
		}
	}
	// No need here
	//if parseCtl <= unserializeCtlArgsTypes {
	//	return attr
	//}

	field, err = decoder.Decode()
	if err != nil {
		fmt.Printf("unSerialize: Decode attachments fail, err=%v\n", err)
		return nil
	}
	if v, ok := field.(map[interface{}]interface{}); ok {
		attachments = hessian.ToMapStringString(v)
		attr.attachments = attachments
	}
	// No need here
	//if parseCtl <= unserializeCtlAttachments {
	//	return attr
	//}

	return attr
}

func dubboGetServiceName(data []byte) string {
	rslt, bodyLen := isValidDubboData(data)
	if rslt == false || bodyLen <= 0 {
		return ""
	}

	flag := data[DUBBO_FLAG_IDX]
	if getEventPing(flag) {
		// heart-beat frame, there is not service-name
		return ""
	}
	if isReqFrame(flag) != true {
		// response frame, there is not service-name
		return ""
	}
	serializeId := getSerializeId(flag)
	ret := unSerialize(serializeId, data[DUBBO_HEADER_LEN:], unserializeCtlPath)
	serviceName := ""
	if ret != nil {
		serviceName = ret.serviceName
	}
	return serviceName
}

func dubboGetMethodName(data []byte) string {
	//return "dubboMethod"
	rslt, bodyLen := isValidDubboData(data)
	if rslt == false || bodyLen <= 0 {
		return ""
	}

	flag := data[DUBBO_FLAG_IDX]
	if getEventPing(flag) {
		// heart-beat frame, there is not method-name
		return ""
	}
	if isReqFrame(flag) != true {
		// response frame, there is not method-name
		return ""
	}
	serializeId := getSerializeId(flag)
	ret := unSerialize(serializeId, data[DUBBO_HEADER_LEN:], unserializeCtlMethod)
	methodName := ""
	if ret != nil {
		methodName = ret.methodName
	}
	return methodName
}

func dubboGetMeta(data []byte) map[string]string {
	//return "dubboMeta"
	retMap := make(map[string]string)
	rslt, bodyLen := isValidDubboData(data)
	if rslt == false || bodyLen <= 0 {
		return nil
	}

	flag := data[DUBBO_FLAG_IDX]
	if getEventPing(flag) {
		// heart-beat frame, there is not method-name
		retMap[types.HeaderXprotocolHeartbeat] = XPROTOCOL_PLUGIN_DUBBO
		return retMap
	}
	if isReqFrame(flag) != true {
		status := data[DUBBO_STATUS_IDX]
		retMap[types.HeaderXprotocolRespStatus] = strconv.Itoa(int(status))

		// TODO: support version under v2.7.1
		decoder := hessian.NewDecoder(data[DUBBO_HEADER_LEN:])
		field, err := decoder.Decode()
		if err != nil {
			fmt.Printf("Decode resWithException fail, err=%v\n", err)
		}
		resWithException, ok := field.(int32)
		if !ok {
			fmt.Printf("Decode resWithException fail, illegal type\n")
		} else {
			if resWithException == RESPONSE_WITH_EXCEPTION || resWithException == RESPONSE_WITH_EXCEPTION_WITH_ATTACHMENTS {
				retMap[types.HeaderXprotocolRespIsException] = "true"
			}
		}

		return retMap
	}
	serializeId := getSerializeId(flag)
	ret := unSerialize(serializeId, data[DUBBO_HEADER_LEN:], unserializeCtlAttachments)
	retMap["serviceName"] = ret.serviceName
	retMap["dubboVersion"] = ret.dubboVersion
	retMap["methodName"] = ret.methodName
	retMap["path"] = ret.path
	retMap["version"] = ret.version

	if ret.attachments != nil {
		for k, v := range ret.attachments {
			retMap[k] = v
		}
	}

	return retMap
}
