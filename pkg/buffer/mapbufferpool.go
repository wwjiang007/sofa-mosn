package buffer
//
//import "time"
//
//var (
//	defaultMapSize = 1 << 3
//	minMapShift = 3
//	maxMapShift = 7
//)
//
//var mapBufferPools [maxPoolSize]*byteBufferPool
//
//func init() {
//	initMapBufferPool()
//
//	// print stats
//	tick := time.Tick(time.Second)
//	for {
//		<-tick
//
//
//
//	}
//}
//
//// mapBufferPool is map[string]string pools
//type mapBufferPool struct {
//	pool []*bufferSlot
//}
//
//// newMapBufferPool returns mapBufferPool
//func newMapBufferPool() *mapBufferPool {
//	p := &mapBufferPool{}
//	for i := minMapShift; i <= maxMapShift; i++ {
//		slab := &bufferSlot{
//			defaultSize: 1 << (uint)(i),
//		}
//		p.pool = append(p.pool, slab)
//	}
//
//	return p
//}
//
//func (p *mapBufferPool) slot(size int) int {
//	if size > p.maxSize || size <= p.minSize {
//		return errSlot
//	}
//	slot := 0
//	shift := 0
//	if size > p.minSize {
//		size--
//		for size > 0 {
//			size = size >> 1
//			shift++
//		}
//		slot = shift - p.minShift
//	}
//
//	return slot
//}
//
//func newMap(size int) map[string]string {
//	return make(map[string]string, size)
//}
//
//// take returns *map[string]string from mapBufferPool
//func (p *mapBufferPool) take(size int) *map[string]string {
//	slot := p.slot(size)
//	if slot == errSlot {
//		m := newMap(size)
//		return &b
//	}
//	v := p.pool[slot].pool.Get()
//	if v == nil {
//		b := newBytes(p.pool[slot].defaultSize)
//		b = b[0:size]
//		return &b
//	}
//	b := v.(*[]byte)
//	*b = (*b)[0:size]
//	return b
//}
//
//// give returns *[]byte to byteBufferPool
//func (p *byteBufferPool) give(buf *[]byte) {
//	if buf == nil {
//		return
//	}
//	size := cap(*buf)
//	slot := p.slot(size)
//	if slot == errSlot {
//		return
//	}
//	if size != int(p.pool[slot].defaultSize) {
//		return
//	}
//	p.pool[slot].pool.Put(buf)
//}
//
//func initByterBufferPool() {
//	for i := 0; i < maxPoolSize; i++ {
//		byteBufferPools[i] = newByteBufferPool()
//	}
//}
//
//// getByteBufferPool returns byteBufferPool from byteBufferPools
//func getByteBufferPool() *byteBufferPool {
//	i := bufferPoolIndex()
//	return byteBufferPools[i]
//}
//
//type ByteBufferCtx struct{}
//
//type ByteBufferPoolContainer struct {
//	bytes []*[]byte
//	*byteBufferPool
//}
//
//func (ctx ByteBufferCtx) Name() int {
//	return Bytes
//}
//
//func (ctx ByteBufferCtx) New() interface{} {
//	return &ByteBufferPoolContainer{
//		byteBufferPool: getByteBufferPool(),
//	}
//}
//
//func (ctx ByteBufferCtx) Reset(i interface{}) {
//	p := i.(*ByteBufferPoolContainer)
//	for _, buf := range p.bytes {
//		p.give(buf)
//	}
//	p.bytes = p.bytes[:0]
//}
//
//// GetBytesByContext returns []byte from byteBufferPool by context
////func GetBytesByContext(context context.Context, size int) *[]byte {
////	p := PoolContext(context).Find(ByteBufferCtx{}, nil).(*ByteBufferPoolContainer)
////	buf := p.take(size)
////	p.bytes = append(p.bytes, buf)
////	return buf
////}
//
//// GetBytes returns *[]byte from byteBufferPool
//func GetMap(size int) *[]byte {
//	p := getByteBufferPool()
//	return p.take(size)
//}
//
//// PutBytes Put *[]byte to byteBufferPool
//func PutMap(buf *[]byte) {
//	p := getByteBufferPool()
//	p.give(buf)
//}
