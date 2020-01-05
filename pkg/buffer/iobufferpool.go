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

package buffer

import (
	"errors"
	"sync"

	"mosn.io/mosn/pkg/types"
)

var ibPool IoBufferPool

// IoBufferPool is Iobuffer Pool
type IoBufferPool struct {
	pool sync.Pool
}

// take returns IoBuffer from IoBufferPool
func (p *IoBufferPool) take(size int) (buf types.IoBuffer) {
	v := p.pool.Get()
	if v == nil {
		buf = NewIoBuffer(size)
	} else {
		buf = v.(types.IoBuffer)
		buf.Alloc(size)
		buf.Count(1)
	}
	return
}

// give returns IoBuffer to IoBufferPool
func (p *IoBufferPool) give(buf types.IoBuffer) {
	buf.Free()
	p.pool.Put(buf)
}

// GetIoBuffer returns IoBuffer from pool
func GetIoBuffer(size int) types.IoBuffer {
	return ibPool.take(size)
}

// PutIoBuffer returns IoBuffer to pool
func PutIoBuffer(buf types.IoBuffer) error {
	count := buf.Count(-1)
	if count > 0 {
		return nil
	} else if count < 0 {
		return errors.New("PutIoBuffer duplicate")
	}
	ibPool.give(buf)
	return nil
}
