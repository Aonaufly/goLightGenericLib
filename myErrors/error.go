package myErrors

import "errors"

var (
	//本地缓存Key不存在(本地缓存错误提示)
	ErrLocalKeyNotExist = errors.New("local cache key not exist")
	//Set Redis失败
	ErrToWriteRedisFailed = errors.New("redis cache: write redis error")
	//尝试处理失败
	ErrToTriedFailed = errors.New("tried redis")
)
