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
	//Redis的库号
	dbIndex uint8
}

// 构造函数
func NewRedisCache(client redis.Cmdable, dbIndex uint8) *RedisCache {

	return &RedisCache{client, dbIndex}
}

// 获得Redis客户端句柄
func (r *RedisCache) GetRedis() (*redis.Cmdable, uint8) {
	return &r.client, r.dbIndex
}
func (r *RedisCache) ListPush(ctx context.Context, listName string, value ...any) error {
	err := r.client.RPush(ctx, listName, value...).Err()
	if err != nil {
		return err
	}
	return nil
}

func (r *RedisCache) ListPop(ctx context.Context, listName string) (string, error) {
	val, err := r.client.RPop(ctx, listName).Result()
	if err != nil {
		return "", err
	}
	return val, nil
}

func (r *RedisCache) ListUnshift(ctx context.Context, listName string, value ...any) error {
	err := r.client.LPush(ctx, listName, value...).Err()
	if err != nil {
		return err
	}
	return nil
}

func (r *RedisCache) ListShift(ctx context.Context, listName string) (string, error) {
	val, err := r.client.LPop(ctx, listName).Result()
	if err != nil {
		return "", err
	}
	return val, nil
}

func (r *RedisCache) LLength(ctx context.Context, listName string) (int64, error) {
	val, err := r.client.LLen(ctx, listName).Result()
	if err != nil {
		return 0, err
	}
	return val, nil
}

func (r *RedisCache) HSet(ctx context.Context, hashName string, key string, value any) error {
	err := r.client.HSet(ctx, hashName, key, value).Err()
	if err != nil {
		return err
	}
	return nil
}

func (r *RedisCache) HGet(ctx context.Context, hashName string, key string) (any, error) {
	val, err := r.client.HGet(ctx, hashName, key).Result()
	if err != nil {
		return nil, err
	}
	return val, nil
}

func (r *RedisCache) HLength(ctx context.Context, hashName string) (int64, error) {
	val, err := r.client.HLen(ctx, hashName).Result()
	if err != nil {
		return 0, err
	}
	return val, nil
}

func (r *RedisCache) HDelete(ctx context.Context, hashName string, key string) error {
	err := r.client.HDel(ctx, hashName, key).Err()
	if err != nil {
		return err
	}
	return nil
}

func (r *RedisCache) HExpire(ctx context.Context, hashName string, expiration time.Duration) error {
	err := r.client.HExpire(ctx, hashName, expiration).Err()
	if err != nil {
		return err
	}
	return nil
}

func (r *RedisCache) HKeys(ctx context.Context, hashName string) ([]string, error) {
	val, err := r.client.HKeys(ctx, hashName).Result()
	if err != nil {
		return nil, err
	}
	return val, nil
}

func (r *RedisCache) HGetAll(ctx context.Context, hashName string) (map[string]string, error) {
	val, err := r.client.HGetAll(ctx, hashName).Result()
	if err != nil {
		return nil, err
	}
	return val, err
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

func (r *RedisCache) Delete(ctx context.Context, key string) error {
	_, err := r.client.Del(ctx, key).Result()
	return err
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
	cli, ok := r.client.(*redis.Client)
	if !ok {
		return
	}
	cli.Close()
}
