package myErrors

import "errors"

var (
	//本地缓存Key不存在(本地缓存错误提示)
	ErrLocalKeyNotExist = errors.New("local cache key not exist")
)
