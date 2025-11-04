// 分布式锁资源管理 Redis str
package cache

import (
	"errors"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"sync"
	"sync/atomic"
	"time"
)

var (
	instLockPtr             atomic.Pointer[RedisDistributeLockResManager]
	instRenewalAndUnlockPtr atomic.Pointer[RedisDistributeRenewalAndUnlockResManager]
)

// 加锁资源 --------------------------------------------------------------------------------------------------------------
type RedisDistributeLockResManager struct {
	pool *sync.Pool
	cap  int32
	cnt  int32
}

// Redis 分布式锁资源管理
func GetInstallRedisDistributeLockResManager() *RedisDistributeLockResManager {
	if p := instLockPtr.Load(); p != nil {
		return p
	}
	newInst := &RedisDistributeLockResManager{
		pool: &sync.Pool{
			New: func() interface{} {
				return &RedisDistributedLock{
					isLockingStatus: false,
					uniqueId:        uuid.New().String(),
				}
			},
		},
		cap: 25,
	}
	if instLockPtr.CompareAndSwap(nil, newInst) {
		return newInst
	}
	return instLockPtr.Load()
}

func (r *RedisDistributeLockResManager) Get(client redis.Cmdable) (*RedisDistributedLock, error) {
	if atomic.LoadInt32(&r.cnt) > 0 {
		atomic.AddInt32(&r.cnt, -1)
	}
	item := r.pool.Get()
	val, ok := item.(*RedisDistributedLock)
	if !ok {
		return nil, errors.New("没有得到*RedisDistributedLockStr资源")
	}
	val.Reset(client)
	return val, nil
}

func (r *RedisDistributeLockResManager) Put(item *RedisDistributedLock) error {
	if item == nil {
		return errors.New("*RedisDistributedLock == nil")
	}
	item.Clear()
	if r.cnt >= r.cap {
		return errors.New("*RedisDistributedLock is full")
	}
	atomic.AddInt32(&r.cnt, 1)
	r.pool.Put(item)
	return nil
}

// 续约/解锁资源 ----------------------------------------------------------------------------------------------------------
type RedisDistributeRenewalAndUnlockResManager struct {
	pool *sync.Pool
	cap  int32
	cnt  int32
}

func GetInstallRedisDistributeRenewalAndUnlockResManager() *RedisDistributeRenewalAndUnlockResManager {
	if p := instRenewalAndUnlockPtr.Load(); p != nil {
		return p
	}
	newInst := &RedisDistributeRenewalAndUnlockResManager{
		pool: &sync.Pool{
			New: func() interface{} {
				return &RedisDistributedRenewalAndUnlock{
					uniqueId: uuid.New().String(),
				}
			},
		},
		cap: 25,
	}
	if instRenewalAndUnlockPtr.CompareAndSwap(nil, newInst) {
		return newInst
	}
	return instRenewalAndUnlockPtr.Load()
}

func (r *RedisDistributeRenewalAndUnlockResManager) Get(client redis.Cmdable, key string, value string, expiration time.Duration) (*RedisDistributedRenewalAndUnlock, error) {
	if atomic.LoadInt32(&r.cnt) > 0 {
		atomic.AddInt32(&r.cnt, -1)
	}
	item := r.pool.Get()
	val, ok := item.(*RedisDistributedRenewalAndUnlock)
	if !ok {
		return nil, errors.New("*RedisDistributedRenewalAndUnlock == nil")
	}
	val.Reset(client, key, value, expiration)
	return val, nil
}
func (r *RedisDistributeRenewalAndUnlockResManager) Put(item *RedisDistributedRenewalAndUnlock) error {
	if item == nil {
		return errors.New("*RedisDistributedRenewalAndUnlock == nil")
	}
	item.Clear()
	if atomic.LoadInt32(&r.cnt) >= r.cap {
		return errors.New("*RedisDistributedRenewalAndUnlock is full")
	}
	atomic.AddInt32(&r.cnt, 1)
	r.pool.Put(item)
	return nil
}
