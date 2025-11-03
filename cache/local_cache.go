package cache

import (
	"context"
	"github.com/Aonaufly/goLightGenericLib/common"
	"github.com/Aonaufly/goLightGenericLib/manager"
	"github.com/Aonaufly/goLightGenericLib/myErrors"
	"github.com/google/uuid"
	"strings"
	"sync"
	"time"
)

// 本地缓存结构体
type LocalCache struct {
	dataMap  map[string]*localCacheItem
	uniqueId string
	pool     *common.LocalPool[*localCacheItem]
	mutex    sync.RWMutex
}

func NewInstance(cap uint) *LocalCache {
	newCache := &LocalCache{
		dataMap:  make(map[string]*localCacheItem),
		uniqueId: uuid.New().String(),
	}
	pool := common.NewLocalPool[*localCacheItem](cap, 0, newCache.newItemFunc, newCache.clearItemFunc, newCache.destroyItemFunc)
	newCache.pool = pool
	return newCache
}

// 获得倒计时Flag
func (b *LocalCache) getCdFlag(key string) string {
	return "___CD_Local_Cache_" + b.uniqueId + key
}

func (b *LocalCache) newItemFunc() *localCacheItem {
	return &localCacheItem{}
}

func (b *LocalCache) clearItemFunc(item *localCacheItem) {
	for k, v := range b.dataMap {
		if v == item {
			manager.GetInstallCountDownManager().RemoveCd(b.getCdFlag(k))
			break
		}
	}
	item.value = nil
}

func (b *LocalCache) destroyItemFunc(item *localCacheItem) {
	for k, v := range b.dataMap {
		if v == item {
			manager.GetInstallCountDownManager().RemoveCd(b.getCdFlag(k))
			break
		}
	}
	item.value = nil
}

func (b *LocalCache) Get(ctx context.Context, key string) (any, error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	v, ok := b.dataMap[key]
	if !ok {
		return nil, myErrors.ErrLocalKeyNotExist
	}
	return v.value, nil
}

func (b *LocalCache) cdHandler(flag string, curCD time.Duration, repeat uint64, isComplete bool, parameterList []any) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if isComplete && strings.HasPrefix(flag, "___CD_Local_Cache_"+b.uniqueId) {
		key, _ := parameterList[0].(string)
		val, ok := b.dataMap[key]
		if ok {
			b.pool.Put(val)
			delete(b.dataMap, key)
		}
	}
}

func (b *LocalCache) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	b.mutex.TryLock()
	defer b.mutex.Unlock()
	oldVal, oldOk := b.dataMap[key]
	var item *localCacheItem
	if oldOk {
		item = oldVal
	} else {
		item = b.pool.Get()
		item.value = value
		b.dataMap[key] = item
	}
	if expiration > 0 {
		manager.GetInstallCountDownManager().AddCd(
			b.getCdFlag(key),
			true, //最后一次调用钩子函数
			false,
			1,
			uint64(expiration.Seconds()),
			b.cdHandler,
			key,
		)
	}
	return nil
}

func (b *LocalCache) Clear(ctx context.Context) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	for k, v := range b.dataMap {
		if v != nil {
			b.pool.Put(v)
			delete(b.dataMap, k)
		}
	}
	return nil
}

func (b *LocalCache) Destroy(ctx context.Context) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	manager.GetInstallCountDownManager().RemoveCdByPrefix("___CD_Local_Cache_" + b.uniqueId)
	for k, v := range b.dataMap {
		if v != nil {
			delete(b.dataMap, k)
		}
	}
	b.pool.Clear()
	b.dataMap = nil
	b.pool = nil
}

func (b *LocalCache) Delete(ctx context.Context, key string) (any, error) {
	b.mutex.TryRLock()
	b.mutex.Unlock()
	defer b.mutex.Unlock()
	v, ok := b.dataMap[key]
	if !ok {
		return nil, nil
	}
	b.pool.Put(v)
	delete(b.dataMap, key)
	return v, nil
}

type localCacheItem struct {
	value any
}
