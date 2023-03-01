package utils

import (
	"sync"
	"sync/atomic"
)

type WaitPool struct {
	pool  sync.Pool
	cond  sync.Cond
	lock  sync.Mutex
	count uint32
	max   uint32
}

func NewWaitPool(max uint32, new func() interface{}) *WaitPool {
	p := &WaitPool{pool: sync.Pool{New: new}, max: max}
	p.cond = sync.Cond{L: &p.lock}
	return p
}

func (p *WaitPool) Get() interface{} {
	if p.max != 0 {
		p.lock.Lock()
		for atomic.LoadUint32(&p.count) >= p.max {
			p.cond.Wait()
		}
		atomic.AddUint32(&p.count, 1)
		p.lock.Unlock()
	}
	return p.pool.Get()
}

func (p *WaitPool) Put(x interface{}) {
	p.pool.Put(x)
	if p.max == 0 {
		return
	}
	atomic.AddUint32(&p.count, ^uint32(0))
	p.cond.Signal()
}
