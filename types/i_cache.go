package types

import (
	"context"
	"time"
)

// 本地缓存接口
type ICache interface {
	Get(ctx context.Context, key string) (any, error)
	Set(ctx context.Context, key string, value any, expiration time.Duration) error
	Delete(ctx context.Context, key string) (any, error)
	//删除所有的缓存
	Clear(ctx context.Context) error
	//销毁
	Destroy(ctx context.Context)
}

// Redis缓存接口
type IRedisCache interface {
	ICache
}
