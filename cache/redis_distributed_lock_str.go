// 简单 string 分布式锁操作
package cache

import (
	"context"
	_ "embed"
	"github.com/Aonaufly/goLightGenericLib/manager"
	"github.com/Aonaufly/goLightGenericLib/myErrors"
	"github.com/Aonaufly/goLightGenericLib/types"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"strings"
	"time"
)

type ErrHookProgress4RedisLock func(item *RedisDistributedRenewalAndUnlockStr, err error)

var (
	//go:embed luaStr/unlock_redis.lua
	lua2UnLock string
	//go:embed luaStr/renewal_redis.lua
	lua2Renewal string
	//go:embed luaStr/lock_redis.lua
	lua2lock string
)

// 分布式锁 str 处理
type RedisDistributedLockStr struct {
	client redis.Cmdable
	item   *RedisDistributedRenewalAndUnlockStr
	//是否正在加锁过程中。。。
	isLockingStatus bool
	//唯一ID，countdown有效
	uniqueId string
}

// 重置
func (rl *RedisDistributedLockStr) Reset(client redis.Cmdable) {
	rl.client = client
}

func (rl *RedisDistributedLockStr) Clear() {
	manager.GetInstallCountDownManager().RemoveCd("___countdown_cd_redis_lock_str" + rl.uniqueId)
	rl.isLockingStatus = false
	rl.client = nil
	if rl.item != nil {
		GetInstallRedisDistributeRenewalAndUnlockStrResManager().Put(rl.item)
		rl.item = nil
	}
}

// 可以尝试多次加锁，避免一次锁定失败 API ***************************************************************************************************
func (rl *RedisDistributedLockStr) TryLock(
	ctx context.Context,
	key string,
	expiration time.Duration,
	retry types.FixIntervalRetry,
	callback ErrHookProgress4RedisLock,
) {
	if rl.isLockingStatus == false {
		rl.isLockingStatus = true
	} else {
		if callback != nil {
			callback(nil, myErrors.ErrRedisAtLockingStatus)
		}
		return
	}
	interval, ok := retry.Next()
	if !ok {
		if callback != nil {
			callback(nil, myErrors.ErrFailedToRedisPreemptLock)
		}
		types.GetInstallFixIntervalRetryManager().Put(&retry)
	} else {
		if retry.Interval <= 0 { //立刻进行重试操作
			rl.countdown4Lock(
				"___countdown_cd_redis_lock_str"+rl.uniqueId,
				0,
				1,
				true,
				[]any{
					callback,
					retry,
					uuid.New().String(),
					ctx,
					key,
					expiration,
				},
			)
		} else {
			manager.GetInstallCountDownManager().AddCd(
				"___countdown_cd_redis_lock_str"+rl.uniqueId,
				false,
				true,
				interval,
				1,
				rl.countdown4Lock, //提供钩子函数
				callback,
				retry,
				uuid.New().String(),
				ctx,
				key,
				expiration,
			)
		}
	}
}

func (rl *RedisDistributedLockStr) countdown4Lock(flag string, curCD time.Duration, repeat uint64, isComplete bool, parameterList []any) {
	if !strings.HasPrefix(flag, "___countdown_cd_redis_lock_str") {
		return
	}
	callback := parameterList[0].(ErrHookProgress4RedisLock)
	retry := parameterList[1].(types.FixIntervalRetry)
	val, _ := parameterList[2].(string)
	ctx, _ := parameterList[3].(context.Context)
	key, _ := parameterList[4].(string)
	expiration, _ := parameterList[5].(time.Duration)
	item, err := rl.stepLock(ctx, key, val, expiration)
	if err != nil {
		if err == myErrors.ErrRedisLockFailedNeedTry || err == myErrors.ErrRedisTimeout {
			interval, ok := retry.Next()
			if !ok {
				if callback != nil {
					callback(nil, myErrors.ErrFailedToRedisPreemptLock)
				}
				types.GetInstallFixIntervalRetryManager().Put(&retry)
			} else {
				if retry.Interval <= 0 { //立刻执行重试
					rl.countdown4Lock(
						"___countdown_cd_redis_lock_str"+rl.uniqueId,
						0,
						1,
						true,
						[]any{
							callback,
							retry,
							val,
							ctx,
							key,
							expiration,
						},
					)
				} else {
					manager.GetInstallCountDownManager().AddCd(
						"___countdown_cd_redis_lock_str"+rl.uniqueId,
						false,
						true,
						interval,
						1,
						rl.countdown4Lock, //提供钩子函数
						callback,
						retry,
						val,
						ctx,
						key,
						expiration,
					)
				}
			}
		} else {
			if callback != nil {
				callback(nil, myErrors.ErrFailedToRedisPreemptLock)
			}
			types.GetInstallFixIntervalRetryManager().Put(&retry)
		}
	} else {
		types.GetInstallFixIntervalRetryManager().Put(&retry)
		if callback != nil {
			callback(item, nil)
		}
	}
}

// 只加一次锁，有可能失败
func (rl *RedisDistributedLockStr) stepLock(ctx context.Context, key string, val string, expiration time.Duration) (*RedisDistributedRenewalAndUnlockStr, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	//原子操作
	ok := rl.client.Eval(ctx, lua2lock, []string{key}, []any{val, expiration}).String()
	cancel()
	select {
	case <-ctx.Done():
		return nil, myErrors.ErrRedisTimeout
	default:
		if ok == "" {
			//别人抢到了锁
			return nil, myErrors.ErrFailedToRedisPreemptLock
		} else if ok == "OK" {
			//计入对象池，用来提高性能, 获得一个续约/解锁的item
			item, e := GetInstallRedisDistributeRenewalAndUnlockStrResManager().Get(
				rl.client,
				key,
				val,
				expiration,
			)
			if e == nil {
				rl.item = item
			}
			return item, nil
		}
		return nil, myErrors.ErrRedisLockFailedNeedTry
	}
}

// 续约和释放锁处理（Redis分布式锁）----------------------------------------------------------------------------------------------
type RedisDistributedRenewalAndUnlockStr struct {
	client redis.Cmdable
	//唯一ID，countdown有效
	uniqueId   string
	key        string
	value      string
	expiration time.Duration
	//是不是自动续约中
	isStatusRenewing bool
}

func (rl *RedisDistributedRenewalAndUnlockStr) Reset(client redis.Cmdable, key string, value string, expiration time.Duration) {
	rl.client = client
	rl.key = key
	rl.value = value
	rl.expiration = expiration
	rl.isStatusRenewing = false
}

func (rl *RedisDistributedRenewalAndUnlockStr) Clear() {
	rl.isStatusRenewing = false
	manager.GetInstallCountDownManager().RemoveCd("___countdown_cd_redis_unlock_renewing_str" + rl.uniqueId)
	rl.client = nil
}

// 续约要进行原子操作： 考虑lua, 只能续约1次
func (l *RedisDistributedRenewalAndUnlockStr) Renewal(ctx context.Context) error {
	cnt, err := l.client.Eval(ctx, lua2Renewal, []string{l.key}, []any{l.value, l.expiration.Seconds()}).Int64()
	if err != nil {
		return err
	}
	if cnt != 1 {
		return myErrors.ErrRedisLockNotHold
	}
	return nil
}

// 自动续约要进行原子操作： 考虑lua,
// maxRenewingCnt 续约的此时
// everyRetry 每次续约的重试策略
// expiration 间隔多长时间开始续约
// callback 钩子函数
func (l *RedisDistributedRenewalAndUnlockStr) AutoRenewal(maxRenewingCnt uint, expiration time.Duration, everyRetry types.FixIntervalRetry, callback types.ErrHookProgress) {
	if l.isStatusRenewing == false {
		l.isStatusRenewing = true
	} else {
		//正在进行续约处理
		if callback != nil {
			callback(everyRetry.CurIndex, everyRetry.Max, myErrors.ErrRedisAtRenewingStatus)
		}
		return
	}
	var curRenewingCnt uint = 0
	l.startCD(expiration, maxRenewingCnt, &curRenewingCnt, &everyRetry, callback)
}

func (l *RedisDistributedRenewalAndUnlockStr) startCD(expiration time.Duration, maxRenewingCnt uint, curRenewingCnt *uint, everyRetry *types.FixIntervalRetry, callback types.ErrHookProgress) {
	interval, ok := everyRetry.Next()
	if !ok {
		//超过重试的次数， 不能再进行重试了
		types.GetInstallFixIntervalRetryManager().Put(everyRetry)
		if callback != nil {
			callback(int(*curRenewingCnt), int(maxRenewingCnt), context.DeadlineExceeded) //超时
		}
	} else {
		if interval <= 0 || everyRetry.CurIndex > 1 { //立即执行
			l.countdown4Renewing(
				"___countdown_cd_redis_unlock_renewing_str"+l.uniqueId,
				0,
				1,
				true,
				[]any{
					expiration,
					everyRetry,
					callback,
					maxRenewingCnt,
					curRenewingCnt},
			)
		} else {
			manager.GetInstallCountDownManager().AddCd(
				"___countdown_cd_redis_unlock_renewing_str"+l.uniqueId,
				false,
				false,
				interval,
				1,
				l.countdown4Renewing, //提供钩子函数
				expiration,
				everyRetry,
				callback,
				maxRenewingCnt,
				curRenewingCnt,
			)
		}
	}
}

func (l *RedisDistributedRenewalAndUnlockStr) countdown4Renewing(flag string, curCD time.Duration, repeat uint64, isComplete bool, parameterList []any) {
	if flag != "___countdown_cd_redis_unlock_renewing_str"+l.uniqueId {
		return
	}
	if l.isStatusRenewing == false {
		return
	}
	everyRetry, _ := parameterList[1].(*types.FixIntervalRetry)
	var cxtSeconds time.Duration = 2 + (time.Duration)(everyRetry.CurIndex) //重试的时候加时间
	ctx, cannel := context.WithTimeout(context.Background(), time.Second*cxtSeconds)
	//原子操作
	cnt, err := l.client.Eval(ctx, lua2Renewal, []string{l.key}, []any{l.value, l.expiration.Seconds()}).Int64()
	cannel()
	callback := parameterList[2].(types.ErrHookProgress)
	maxRenewingCnt := parameterList[3].(uint)
	curRenewingCnt := parameterList[4].(*uint)
	select {
	case <-ctx.Done(): //超时
		l.startCD(parameterList[0].(time.Duration), maxRenewingCnt, curRenewingCnt, everyRetry, callback)
	default:
		if everyRetry.IsOver() {
			types.GetInstallFixIntervalRetryManager().Put(everyRetry)
			if err != nil {
				if callback != nil {
					callback(int(*curRenewingCnt), int(maxRenewingCnt), err)
				}
				return
			}
			if cnt != 1 {
				if callback != nil {
					callback(int(*curRenewingCnt), int(maxRenewingCnt), myErrors.ErrRedisLockNotHold)
				}
				return
			}
		} else {
			if err != nil {
				if err == context.DeadlineExceeded {
					//重试,续约失败
					l.startCD(parameterList[0].(time.Duration), maxRenewingCnt, curRenewingCnt, everyRetry, callback)
					return
				}
			} else {
				everyRetry.CurIndex = 0 //重置try次数
				if callback != nil {
					callback(int(*curRenewingCnt), int(maxRenewingCnt), nil)
				}
				if *curRenewingCnt < maxRenewingCnt {
					//还需要继续续约
					*curRenewingCnt++
					l.startCD(parameterList[0].(time.Duration), maxRenewingCnt, curRenewingCnt, everyRetry, callback)
				} else {
					types.GetInstallFixIntervalRetryManager().Put(everyRetry)
				}
				return
			}
		}
	}
}

// 解锁要进行原子操作： 考虑lua
func (l *RedisDistributedRenewalAndUnlockStr) UnLock(ctx context.Context) error {
	if l.isStatusRenewing == true { //通知取消重试Redis分布式锁续约
		l.isStatusRenewing = false
		manager.GetInstallCountDownManager().RemoveCd("___countdown_cd_redis_unlock_renewing_str" + l.uniqueId)
	}
	cnt, err := l.client.Eval(ctx, lua2UnLock, []string{l.key}, l.value).Int64()
	if err == redis.Nil {
		//执行报错
		return myErrors.ErrRedisLockNotHold
	}
	if err != nil {
		return err
	}
	if cnt != 1 {
		//锁已经找不到了， 2种可能性： ①，过期了/ ②，其他人连redis手动删除了
		return myErrors.ErrRedisUnLockFailedToUnExistKey
	}
	return nil
}
