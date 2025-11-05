// 简单 string 分布式锁操作
package cache

import (
	"context"
	_ "embed"
	"errors"
	"github.com/Aonaufly/goLightGenericLib/manager"
	"github.com/Aonaufly/goLightGenericLib/myErrors"
	"github.com/Aonaufly/goLightGenericLib/types"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
	"strings"
	"time"
)

type ErrHookProgress4RedisLock func(item *RedisDistributedRenewalAndUnlock, err error)

var (
	//go:embed luaStr/unlock_redis.lua
	lua2UnLock string
	//go:embed luaStr/renewal_redis.lua
	lua2Renewal string
	//go:embed luaStr/lock_redis.lua
	lua2lock string
)

// 分布式锁 str 处理
type RedisDistributedLock struct {
	client redis.Cmdable
	item   *RedisDistributedRenewalAndUnlock
	//是否正在加锁过程中。。。
	isLockingStatus bool
	//唯一ID，countdown有效
	uniqueId string
	//实现singleflight抢锁机制
	g singleflight.Group
}

// 重置
func (rl *RedisDistributedLock) Reset(client redis.Cmdable) {
	rl.client = client
}

func (rl *RedisDistributedLock) Clear() {
	manager.GetInstallCountDownManager().RemoveCd("___countdown_cd_redis_lock_str" + rl.uniqueId)
	rl.isLockingStatus = false
	rl.client = nil
	if rl.item != nil {
		GetInstallRedisDistributeRenewalAndUnlockResManager().Put(rl.item)
		rl.item = nil
	}
}

//#region singleflight抢锁

// 自动singleflight加锁（非常高并发并且热点集中的极端情况下使用，慎用）
func (rl *RedisDistributedLock) AutoSingleFightLock(
	ctx context.Context,
	key string,
	expiration time.Duration,
	timeout time.Duration,
	retry *types.FixIntervalRetry,
) (*RedisDistributedRenewalAndUnlock, error) {
	for {
		var flag bool = false
		resCh := rl.g.DoChan(key, func() (interface{}, error) {
			flag = true
			return rl.AutoLock(ctx, key, expiration, timeout, retry)
		})
		select {
		case res := <-resCh:
			if flag {
				rl.g.Forget(key)
				if res.Err != nil {
					return nil, res.Err
				}
				return res.Val.(*RedisDistributedRenewalAndUnlock), nil
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// 阻塞式加锁重试方法
func (rl *RedisDistributedLock) AutoLock(
	ctx context.Context,
	key string,
	expiration time.Duration,
	timeout time.Duration,
	retry *types.FixIntervalRetry,
) (*RedisDistributedRenewalAndUnlock, error) {
	var timer *time.Timer
	value := uuid.New().String() //锁的值
	for {
		lctx, cancel := context.WithTimeout(ctx, timeout)
		res, err := rl.client.Eval(lctx, lua2lock, []string{key}, []any{value, expiration.Seconds()}).Result()
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			types.GetInstallFixIntervalRetryManager().Put(retry)
			return nil, err
		}
		if res == "OK" { //加锁成功了
			item, e := GetInstallRedisDistributeRenewalAndUnlockResManager().Get(
				rl.client,
				key,
				value,
				expiration,
			)
			if e == nil {
				rl.item = item
			}
			types.GetInstallFixIntervalRetryManager().Put(retry)
			return item, nil
		}
		cancel()
		interval, ok := retry.Next()
		if !ok { //超过了重试次数
			return nil, myErrors.ErrRetryLimitExceeded
		}
		if timer == nil {
			timer = time.NewTimer(interval)
		} else {
			timer.Reset(interval)
		}
		select {
		case <-timer.C:
		case <-ctx.Done():
			types.GetInstallFixIntervalRetryManager().Put(retry)
			return nil, ctx.Err()
		}
	}
}

//#endregion

// 可以尝试多次加锁，避免一次锁定失败 API ***************************************************************************************************
func (rl *RedisDistributedLock) AutoLockWithHook(
	ctx context.Context,
	key string,
	expiration time.Duration,
	timeout time.Duration,
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
					timeout,
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
				timeout,
			)
		}
	}
}

func (rl *RedisDistributedLock) countdown4Lock(flag string, curCD time.Duration, repeat uint64, isComplete bool, parameterList []any) {
	if !strings.HasPrefix(flag, "___countdown_cd_redis_lock_str") {
		return
	}
	callback := parameterList[0].(ErrHookProgress4RedisLock)
	retry := parameterList[1].(types.FixIntervalRetry)
	val, _ := parameterList[2].(string)
	ctx, _ := parameterList[3].(context.Context)
	key, _ := parameterList[4].(string)
	expiration, _ := parameterList[5].(time.Duration)
	timeout, _ := parameterList[6].(time.Duration)
	item, err := rl.stepLock(ctx, key, val, expiration, timeout)
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
							timeout,
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
						timeout,
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
func (rl *RedisDistributedLock) stepLock(ctx context.Context, key string, val string, expiration time.Duration, timeout time.Duration) (*RedisDistributedRenewalAndUnlock, error) {
	tctx, cancel := context.WithTimeout(ctx, timeout)
	//原子性操作
	res, err := rl.client.Eval(tctx, lua2lock, []string{key}, []any{val, expiration}).Result()
	cancel()
	if err != nil && errors.Is(err, context.DeadlineExceeded) {
		return nil, myErrors.ErrRedisTimeout
	}
	if res == "OK" {
		item, e := GetInstallRedisDistributeRenewalAndUnlockResManager().Get(
			rl.client,
			key,
			val,
			expiration,
		)
		if e == nil {
			rl.item = item
		}
		return item, nil
	} else {
		return nil, myErrors.ErrRedisLockFailedNeedTry
	}
}

// 续约和释放锁处理（Redis分布式锁）----------------------------------------------------------------------------------------------
type RedisDistributedRenewalAndUnlock struct {
	client redis.Cmdable
	//唯一ID，countdown有效
	uniqueId   string
	key        string
	value      string
	expiration time.Duration
	//是不是自动续约中
	isStatusRenewing bool
	unlockChan       chan struct{}
}

func (rl *RedisDistributedRenewalAndUnlock) Reset(client redis.Cmdable, key string, value string, expiration time.Duration) {
	rl.client = client
	rl.key = key
	rl.value = value
	rl.expiration = expiration
	rl.isStatusRenewing = false
}

func (rl *RedisDistributedRenewalAndUnlock) Clear() {
	rl.isStatusRenewing = false
	manager.GetInstallCountDownManager().RemoveCd("___countdown_cd_redis_unlock_renewing_str" + rl.uniqueId)
	rl.client = nil
	for len(rl.unlockChan) > 0 {
		<-rl.unlockChan
	}
}

// 续约要进行原子操作： 考虑lua, 只能续约1次
func (l *RedisDistributedRenewalAndUnlock) Renewal(ctx context.Context) error {
	cnt, err := l.client.Eval(ctx, lua2Renewal, []string{l.key}, []any{l.value, l.expiration.Seconds()}).Int64()
	if err != nil {
		return err
	}
	if cnt != 1 {
		return myErrors.ErrRedisLockNotHold
	}
	return nil
}

//#refion 自动续约，阻塞版

// 自动续约阻塞版本， 一直续约中知道主动的释放锁
func (rl *RedisDistributedRenewalAndUnlock) AutoRenewal(
	interval time.Duration,
	timeout time.Duration,
) error {
	timeoutChan := make(chan struct{}, 1)
	ticker := time.NewTicker(interval)
	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			err := rl.Renewal(ctx)
			cancel()
			if err == context.DeadlineExceeded {
				timeoutChan <- struct{}{}
				continue
			}
			if err != nil {
				return err
			}
		case <-timeoutChan:
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			err := rl.Renewal(ctx)
			cancel()
			if err == context.DeadlineExceeded {
				timeoutChan <- struct{}{}
				continue
			}
			if err != nil {
				return err
			}
		case <-rl.unlockChan:
			//清空chananl中的值
			for len(rl.unlockChan) > 0 {
				<-rl.unlockChan
			}
			return nil
		}
	}
}

//#endregion

// 自动续约要进行原子操作： 考虑lua, hook版
// maxRenewingCnt 续约的此时
// expiration 间隔多长时间开始续约
// timeout 超时
// everyRetry 每次续约的重试策略
// callback 钩子函数
func (l *RedisDistributedRenewalAndUnlock) AutoRenewalWithHook(
	maxRenewingCnt uint,
	expiration time.Duration,
	timeout time.Duration,
	everyRetry types.FixIntervalRetry,
	callback types.ErrHookProgress,
) {
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
	l.startCD(expiration, maxRenewingCnt, &curRenewingCnt, &everyRetry, timeout, callback)
}

func (l *RedisDistributedRenewalAndUnlock) startCD(expiration time.Duration, maxRenewingCnt uint, curRenewingCnt *uint, everyRetry *types.FixIntervalRetry, timeout time.Duration, callback types.ErrHookProgress) {
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
					curRenewingCnt,
					timeout,
				},
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
				timeout,
			)
		}
	}
}

func (l *RedisDistributedRenewalAndUnlock) countdown4Renewing(flag string, curCD time.Duration, repeat uint64, isComplete bool, parameterList []any) {
	if flag != "___countdown_cd_redis_unlock_renewing_str"+l.uniqueId {
		return
	}
	everyRetry, _ := parameterList[1].(*types.FixIntervalRetry)
	callback, _ := parameterList[2].(types.ErrHookProgress)
	maxRenewingCnt, _ := parameterList[3].(uint)
	curRenewingCnt, _ := parameterList[4].(*uint)
	timeout, _ := parameterList[5].(time.Duration)
	if l.isStatusRenewing == false {
		if callback != nil {
			callback(int(*curRenewingCnt), int(maxRenewingCnt), context.DeadlineExceeded) //超时
		}
		types.GetInstallFixIntervalRetryManager().Put(everyRetry)
		return
	}
	var cxtSeconds time.Duration = 2*(time.Duration(everyRetry.CurIndex)-1) + timeout //重试的时候加时间
	ctx, cannel := context.WithTimeout(context.Background(), time.Second*cxtSeconds)
	//原子操作
	cnt, err := l.client.Eval(ctx, lua2Renewal, []string{l.key}, []any{l.value, l.expiration.Seconds()}).Int64()
	cannel()
	select {
	case <-ctx.Done(): //超时
		l.startCD(parameterList[0].(time.Duration), maxRenewingCnt, curRenewingCnt, everyRetry, timeout, callback)
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
					l.startCD(parameterList[0].(time.Duration), maxRenewingCnt, curRenewingCnt, everyRetry, timeout, callback)
					return
				}
			} else {
				everyRetry.Clear()
				if callback != nil {
					callback(int(*curRenewingCnt), int(maxRenewingCnt), nil)
				}
				if *curRenewingCnt < maxRenewingCnt {
					//还需要继续续约
					*curRenewingCnt++
					l.startCD(parameterList[0].(time.Duration), maxRenewingCnt, curRenewingCnt, everyRetry, timeout, callback)
				} else {
					types.GetInstallFixIntervalRetryManager().Put(everyRetry)
				}
				return
			}
		}
	}
}

// 解锁要进行原子操作： 考虑lua Api ***********************************************************************
func (l *RedisDistributedRenewalAndUnlock) UnLock(ctx context.Context) error {
	//是否使用的是阻塞式的自动续约方案
	var isUnHookRenewaling bool = true
	if l.isStatusRenewing == true { //通知取消重试Redis分布式锁续约
		l.isStatusRenewing = false
		isUnHookRenewaling = false
		manager.GetInstallCountDownManager().RemoveCd("___countdown_cd_redis_unlock_renewing_str" + l.uniqueId)
	}
	defer func() {
		if isUnHookRenewaling {
			l.unlockChan <- struct{}{}
		}
	}()
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
