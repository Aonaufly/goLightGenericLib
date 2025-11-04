// 重试策略
package types

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

type IRetryStrategy interface {
	//①，重试的间隔； ②，要不要重试
	Next() (time.Duration, bool)
}

type FixIntervalRetry struct {
	Interval time.Duration
	Max      int
	CurIndex int
}

func (f *FixIntervalRetry) Reset(Interval time.Duration, Max int) {
	f.Interval = Interval
	f.Max = Max
}

func (f *FixIntervalRetry) Clear() {

}

func (f *FixIntervalRetry) Next() (time.Duration, bool) {
	f.CurIndex++
	return f.Interval, f.CurIndex <= f.Max
}

func (f *FixIntervalRetry) IsOver() bool {
	return f.CurIndex >= f.Max
}

var (
	instPtr atomic.Pointer[FixIntervalRetryManager]
)

type FixIntervalRetryManager struct {
	pool *sync.Pool
	cap  int32
	size int32
}

func GetInstallFixIntervalRetryManager() *FixIntervalRetryManager {
	if p := instPtr.Load(); p != nil {
		return p
	}
	newInst := &FixIntervalRetryManager{
		pool: &sync.Pool{
			New: func() interface{} {
				return &FixIntervalRetry{
					CurIndex: 0,
				}
			},
		},
		cap: 25,
	}
	if instPtr.CompareAndSwap(nil, newInst) {
		return newInst
	}
	return instPtr.Load()
}

func (f *FixIntervalRetryManager) Get(Interval time.Duration, Max int) (*FixIntervalRetry, error) {
	if atomic.LoadInt32(&f.size) > 0 {
		atomic.AddInt32(&f.size, -1)
	}
	item, ok := f.pool.Get().(*FixIntervalRetry)
	if !ok {
		return nil, errors.New("FixIntervalRetryManager Get fail")
	}
	item.Reset(Interval, Max)
	return item, nil
}

func (f *FixIntervalRetryManager) Put(item *FixIntervalRetry) {
	if item == nil {
		return
	}
	item.Clear()
	if atomic.LoadInt32(&f.size) >= f.cap {
		return
	}
	atomic.AddInt32(&f.size, 1)
	f.pool.Put(item)
}
