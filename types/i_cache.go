package types

import (
	"context"
	"github.com/redis/go-redis/v9"
	"time"
)

// 本地缓存接口
type ICache interface {
	Get(ctx context.Context, key string) (any, error)
	Set(ctx context.Context, key string, value any, expiration time.Duration) error
	Delete(ctx context.Context, key string) error
	//删除所有的缓存
	Clear(ctx context.Context, errHonk ErrHookProgress)
	//销毁
	Destroy(ctx context.Context)
}

// Redis缓存接口
type IRedisCache interface {
	ICache
	GetRedis() (*redis.Cmdable, uint8)
	//#region hash
	HSet(ctx context.Context, hashName string, key string, value any) error
	HGet(ctx context.Context, hashName string, key string) (any, error)
	HLength(ctx context.Context, hashName string) (int64, error)
	HDelete(ctx context.Context, hashName string, key string) error
	HExpire(ctx context.Context, hashName string, expiration time.Duration) error
	HKeys(ctx context.Context, hashName string) ([]string, error)
	HGetAll(ctx context.Context, hashName string) (map[string]string, error)
	//#endregion

	//#region list
	ListPush(ctx context.Context, listName string, value ...any) error
	ListPop(ctx context.Context, listName string) (string, error)
	ListUnshift(ctx context.Context, listName string, value ...any) error
	ListShift(ctx context.Context, listName string) (string, error)
	LLength(ctx context.Context, listName string) (int64, error)
	//#endregion
}
