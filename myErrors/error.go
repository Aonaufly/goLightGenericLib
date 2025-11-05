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
	//Redis分布式锁正在进行续约 。。。。。。。。。。。
	ErrRedisAtRenewingStatus = errors.New("redis cache: at renewing status")
	//正在尝试加锁。。。。。。
	ErrRedisAtLockingStatus = errors.New("redis cache: at locking status")
	//Redis分布式锁key没有解锁的时候
	ErrRedisUnLockFailedToUnExistKey = errors.New("redis cache: key not exist")
	//加锁失败需要重试
	ErrRedisLockFailedNeedTry = errors.New("redis cache: lock need try")
	//操作超时
	ErrRedisTimeout = errors.New("redis cache: timeout")
	//超出重试次数限制
	ErrRetryLimitExceeded = errors.New("retry limit exceeded")
)
