package cache

import (
	"context"
	"fmt"
	"github.com/Aonaufly/goLightGenericLib/myErrors"
	"github.com/Aonaufly/goLightGenericLib/types"
	"github.com/redis/go-redis/v9"
	"time"
)

// RedisCache类
type RedisCache struct {
	client redis.Cmdable
}

// 构造函数
func NewRedisCache(client redis.Cmdable) *RedisCache {

	return &RedisCache{client}
}

func (r *RedisCache) Get(ctx context.Context, key string) (any, error) {
	return r.client.Get(ctx, key).Result()
}

func (r *RedisCache) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	res, err := r.client.Set(ctx, key, value, expiration).Result()
	if err != nil {
		return err
	}
	if res != "OK" {
		return fmt.Errorf("%w, 返回信息 %s", myErrors.ErrToWriteRedisFailed, res)
	}
	return nil
}

func (r *RedisCache) Delete(ctx context.Context, key string) (any, error) {
	_, err := r.client.Del(ctx, key).Result()
	return nil, err
}

// 慎用,清除掉此库的所有缓存资源
func (r *RedisCache) Clear(ctx context.Context, errHonk types.ErrHookProgress) {
	var curTryIndex int = 0
	var totalTry int = 3
	if err := r.client.FlushDB(ctx).Err(); err != nil {
		if err == context.DeadlineExceeded { //过时
			if curTryIndex >= totalTry {
				errHonk(0, 0, err)
			} else {
				r.tryClear(&curTryIndex, totalTry, errHonk)
			}
		} else {
			if errHonk != nil {
				errHonk(0, 0, err)
			}
		}
	} else if errHonk != nil {
		errHonk(0, 0, nil)
	}
}

// 尝试清理
func (r *RedisCache) tryClear(curTryIndex *int, totalTry int, errHonk types.ErrHookProgress) {
	ctxOut, cancelOut := context.WithTimeout(context.Background(), time.Second*5)
	err := r.client.FlushDB(ctxOut).Err()
	(*curTryIndex)++
	cancelOut()
	if err != nil {
		if err != context.DeadlineExceeded {
			if *curTryIndex >= totalTry {
				if errHonk != nil {
					errHonk(*curTryIndex, totalTry, myErrors.ErrToTriedFailed)
				}
			} else {
				r.tryClear(curTryIndex, totalTry, errHonk)
			}
		} else {
			if errHonk != nil {
				errHonk(*curTryIndex, totalTry, err)
			}
		}
	}
}

// 关闭Redis连接
func (r *RedisCache) Destroy(ctx context.Context) {
	r.client.(*redis.Client).Close()
}
