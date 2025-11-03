package myErrors

import "errors"

var (
	//本地缓存Key不存在(本地缓存错误提示)
	ErrLocalKeyNotExist = errors.New("local cache key not exist")
	//Set Redis失败
	ErrToWriteRedisFailed = errors.New("redis cache: write redis error")
	//尝试处理失败
	ErrToTriedFailed = errors.New("tried redis")
	//Redis 抢占锁失败
	ErrFailedToRedisPreemptLock = errors.New("redis cache: failed preempt lock")
	//Redis没有持有此锁(分布式锁)
	ErrRedisLockNotHold = errors.New("redis cache: lock not hold")
	//Redis分布式锁签约失败
	ErrRedisAtRenewingStatusFailed = errors.New("redis cache: at renewing status failed")
	//Redis分布式锁key没有解锁的时候
	ErrRedisUnLockFailedToUnExistKey = errors.New("redis cache: key not exist")
)
