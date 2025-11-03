// 简单 string 分布式锁操作
package cache

import (
	"context"
	_ "embed"
	"github.com/Aonaufly/goLightGenericLib/manager"
	"github.com/Aonaufly/goLightGenericLib/myErrors"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"time"
)

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
}

func (rl *RedisDistributedLockStr) TryLock(ctx context.Context, key string, expiration time.Duration) (*Lock, error) {
	val := uuid.New().String()
	ok, err := rl.client.SetNX(ctx, key, val, expiration).Result()
	if err != nil {
		return nil, err
	}
	if !ok {
		//别人抢到了锁
		return nil, myErrors.ErrFailedToRedisPreemptLock
	}
	return &Lock{
		rl.client,
		uuid.New().String(),
		key,
		val,
		expiration,
		false,
	}, nil
}

type Lock struct {
	client redis.Cmdable
	//唯一ID，countdown有效
	uniqueId   string
	key        string
	value      string
	expiration time.Duration
	//是不是自动续约中
	isStatusRenewing bool
}

// 续约要进行原子操作： 考虑lua, 只能续约1次
func (l *Lock) Renewal(ctx context.Context) error {
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
// maxRenewingCnt 本次续约的次数
// tryTotal 每次需要尝试次数
// expiration 间隔多长时间开始续约
// callback 钩子函数
func (l *Lock) AutoRenewal(maxRenewingCnt uint, tryTotal uint, expiration time.Duration, callback func(maxRenewingCnt uint, curRenewingCnt uint, err error)) {
	if l.isStatusRenewing == false {
		l.isStatusRenewing = true
	} else {
		if callback != nil {
			callback(maxRenewingCnt, 0, myErrors.ErrRedisAtRenewingStatusFailed)
		}
		return
	}
	var tryIndex uint = 0
	var curRenewingCnt uint = 0
	l.startCD(expiration, maxRenewingCnt, &curRenewingCnt, tryTotal, &tryIndex, callback)
}

func (l *Lock) startCD(expiration time.Duration, maxRenewingCnt uint, curRenewingCnt *uint, tryTotal uint, tryIndex *uint, callback func(maxRenewingCnt uint, curRenewingCnt uint, err error)) {
	if *tryIndex == 0 {
		manager.GetInstallCountDownManager().AddCd(
			"___countdown_cd_redis_lock_renewing_"+l.uniqueId,
			false,
			false,
			expiration,
			1,
			l.countdown4Renewing, //提供钩子函数
			expiration,
			tryTotal,
			tryIndex,
			callback,
			maxRenewingCnt,
			curRenewingCnt,
		)
	} else {
		//重新尝试续约
		l.countdown4Renewing(
			"___countdown_cd_redis_lock_renewing_"+l.uniqueId,
			0,
			1,
			true,
			[]any{
				expiration,
				tryTotal,
				tryIndex,
				callback,
				maxRenewingCnt,
				curRenewingCnt},
		)
	}
}

func (l *Lock) countdown4Renewing(flag string, curCD time.Duration, repeat uint64, isComplete bool, parameterList []any) {
	if flag != "___countdown_cd_redis_lock_renewing_"+l.uniqueId {
		return
	}
	if l.isStatusRenewing == false {
		return
	}
	curIndexPtr := parameterList[2].(*uint)
	*curIndexPtr = *curIndexPtr + 1
	var cxtSeconds time.Duration = 2 + (time.Duration)(*curIndexPtr) //重试的时候加时间
	ctx, cannel := context.WithTimeout(context.Background(), time.Second*cxtSeconds)
	cnt, err := l.client.Eval(ctx, lua2Renewal, []string{l.key}, []any{l.value, l.expiration.Seconds()}).Int64()
	cannel()
	callback := parameterList[3].(func(maxRenewingCnt uint, curRenewingCnt uint, err error))
	isCompleteCnt := *curIndexPtr >= parameterList[1].(uint)
	maxRenewingCnt := parameterList[4].(uint)
	curRenewingCnt := parameterList[5].(*uint)
	if isCompleteCnt {
		if err != nil {
			if callback != nil {
				callback(maxRenewingCnt, *curRenewingCnt, err)
			}
			return
		}
		if cnt != 1 {
			if callback != nil {
				callback(maxRenewingCnt, *curRenewingCnt, myErrors.ErrRedisLockNotHold)
			}
			return
		}
	} else {
		if err != nil {
			if err == context.DeadlineExceeded {
				//重试,续约失败
				l.startCD(parameterList[0].(time.Duration), maxRenewingCnt, curRenewingCnt, parameterList[1].(uint), curIndexPtr, callback)
				return
			}
		} else {
			*curIndexPtr = 0 //重置try次数
			if callback != nil {
				callback(maxRenewingCnt, *curRenewingCnt, nil)
			}
			if *curRenewingCnt < maxRenewingCnt {
				//还需要继续续约
				*curRenewingCnt++
				l.startCD(parameterList[0].(time.Duration), maxRenewingCnt, curRenewingCnt, parameterList[1].(uint), curIndexPtr, callback)
			}
			return
		}
	}
}

// 解锁要进行原子操作： 考虑lua
func (l *Lock) UnLock(ctx context.Context) error {
	if l.isStatusRenewing == true { //通知取消重试Redis分布式锁续约
		l.isStatusRenewing = false
		manager.GetInstallCountDownManager().RemoveCd("___countdown_cd_redis_lock_renewing_" + l.uniqueId)
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
