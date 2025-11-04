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
	instLockPtr             atomic.Pointer[RedisDistributeLockStrResManager]
	instRenewalAndUnlockPtr atomic.Pointer[RedisDistributeRenewalAndUnlockStrResManager]
)

// 加锁资源 --------------------------------------------------------------------------------------------------------------
type RedisDistributeLockStrResManager struct {
	pool *sync.Pool
	cap  int32
	cnt  int32
}

// Redis 分布式锁资源管理
func GetInstallRedisDistributeLockStrResManager() *RedisDistributeLockStrResManager {
	if p := instLockPtr.Load(); p != nil {
		return p
	}
	newInst := &RedisDistributeLockStrResManager{
		pool: &sync.Pool{
			New: func() interface{} {
				return &RedisDistributedLockStr{
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

func (r *RedisDistributeLockStrResManager) Get(client redis.Cmdable) (*RedisDistributedLockStr, error) {
	if atomic.LoadInt32(&r.cnt) > 0 {
		atomic.AddInt32(&r.cnt, -1)
	}
	item := r.pool.Get()
	val, ok := item.(*RedisDistributedLockStr)
	if !ok {
		return nil, errors.New("没有得到*RedisDistributedLockStr资源")
	}
	val.Reset(client)
	return val, nil
}

func (r *RedisDistributeLockStrResManager) Put(item *RedisDistributedLockStr) error {
	if item == nil {
		return errors.New("*RedisDistributedLockStr == nil")
	}
	item.Clear()
	if r.cnt >= r.cap {
		return errors.New("*RedisDistributedLockStr is full")
	}
	atomic.AddInt32(&r.cnt, 1)
	r.pool.Put(item)
	return nil
}

// 续约/解锁资源 ----------------------------------------------------------------------------------------------------------
type RedisDistributeRenewalAndUnlockStrResManager struct {
	pool *sync.Pool
	cap  int32
	cnt  int32
}

func GetInstallRedisDistributeRenewalAndUnlockStrResManager() *RedisDistributeRenewalAndUnlockStrResManager {
	if p := instRenewalAndUnlockPtr.Load(); p != nil {
		return p
	}
	newInst := &RedisDistributeRenewalAndUnlockStrResManager{
		pool: &sync.Pool{
			New: func() interface{} {
				return &RedisDistributedRenewalAndUnlockStr{
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

func (r *RedisDistributeRenewalAndUnlockStrResManager) Get(client redis.Cmdable, key string, value string, expiration time.Duration) (*RedisDistributedRenewalAndUnlockStr, error) {
	if atomic.LoadInt32(&r.cnt) > 0 {
		atomic.AddInt32(&r.cnt, -1)
	}
	item := r.pool.Get()
	val, ok := item.(*RedisDistributedRenewalAndUnlockStr)
	if !ok {
		return nil, errors.New("*RedisDistributedRenewalAndUnlockStr == nil")
	}
	val.Reset(client, key, value, expiration)
	return val, nil
}
func (r *RedisDistributeRenewalAndUnlockStrResManager) Put(item *RedisDistributedRenewalAndUnlockStr) error {
	if item == nil {
		return errors.New("*RedisDistributedRenewalAndUnlockStr == nil")
	}
	item.Clear()
	if atomic.LoadInt32(&r.cnt) >= r.cap {
		return errors.New("*RedisDistributedRenewalAndUnlockStr is full")
	}
	atomic.AddInt32(&r.cnt, 1)
	r.pool.Put(item)
	return nil
}
