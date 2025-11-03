package types

import (
	"context"
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
	HSet(ctx context.Context, hashName string, key string, value any) error
	HGet(ctx context.Context, hashName string, key string) (any, error)
	HLength(ctx context.Context, hashName string) (int64, error)
	HDelete(ctx context.Context, hashName string, key string) error
	HExpire(ctx context.Context, hashName string, expiration time.Duration) error
}
