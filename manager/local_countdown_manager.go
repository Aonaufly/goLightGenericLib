package manager

import (
	"github.com/Aonaufly/goLightGenericLib/common"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type LocalCountDown struct {
	Flag          string
	IsEveryExec   bool
	IsInitExec    bool
	Duration      time.Duration
	RepeatCnt     uint64
	curCd         time.Duration
	parameterList []any
	hook          func(flag string, curCD time.Duration, repeat uint64, isComplete bool, parameterList []any)
	lastDeadline  time.Duration
}

var (
	instPtr atomic.Pointer[CountDownManager]
)

type CountDownManager struct {
	cdMap           map[string]*LocalCountDown
	pool            *common.LocalPool[*LocalCountDown]
	curTotal        time.Duration
	ticker          *time.Ticker
	isTickerRunning bool
	mutex           sync.Mutex
}

// 获得倒计时单例句柄
func GetInstallCountDownManager() *CountDownManager {
	if p := instPtr.Load(); p != nil {
		return p
	}
	newInst := &CountDownManager{
		cdMap: make(map[string]*LocalCountDown),
	}
	pool := common.NewLocalPool[*LocalCountDown](25, 0, newInst.newItemFunc, newInst.clearItemFunc, newInst.destroyItemFunc)
	newInst.pool = pool
	if instPtr.CompareAndSwap(nil, newInst) {
		return newInst
	}
	return instPtr.Load()
}

func (m *CountDownManager) newItemFunc() *LocalCountDown {
	return &LocalCountDown{}
}

func (m *CountDownManager) clearItemFunc(item *LocalCountDown) {
	item.hook = nil
	item.parameterList = nil
}

func (m *CountDownManager) destroyItemFunc(item *LocalCountDown) {
	item.hook = nil
	item.parameterList = nil
}

func (m *CountDownManager) AddCd(
	flag string,
	isEveryExec bool,
	isInitExec bool,
	duration time.Duration,
	repeatCnt uint64,
	hook func(flag string, curCD time.Duration, repeat uint64, isComplete bool, parameterList []any),
	parameterList ...any) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	v, ok := m.cdMap[flag]
	createFunc := func(item *LocalCountDown) {
		item.Flag = flag
		item.IsEveryExec = isEveryExec
		item.IsInitExec = isInitExec
		item.Duration = duration
		item.RepeatCnt = repeatCnt
		item.hook = hook
		item.parameterList = parameterList
		item.lastDeadline = m.curTotal + duration
		if isEveryExec == true {
			item.curCd = time.Duration(repeatCnt)
		} else {
			item.curCd = duration
		}
		if item.IsInitExec == true { //开始的时候执行一次
			item.hook(
				item.Flag,
				item.curCd,
				repeatCnt,
				item.Duration <= 0,
				parameterList,
			)
		}
	}
	if ok {
		createFunc(v)
		if v.Duration <= 0 {
			m.pool.Put(v)
			delete(m.cdMap, flag)
		}
	} else {
		v = m.pool.Get()
		createFunc(v)
		m.cdMap[flag] = v
		go m.startCD()
	}
}

func (m *CountDownManager) RemoveCd(flag string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	v, ok := m.cdMap[flag]
	if ok {
		if v != nil {
			m.pool.Put(v)
		}
		delete(m.cdMap, flag)
	}
	if len(m.cdMap) == 0 {
		if m.isTickerRunning == true {
			if m.ticker != nil {
				m.ticker.Stop()
			}
			m.isTickerRunning = false
		}
		m.curTotal = 0
	}
}

func (m *CountDownManager) RemoveCdByPrefix(flagPrefix string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for _, v := range m.cdMap {
		if v != nil {
			if strings.HasPrefix(v.Flag, flagPrefix) {
				m.pool.Put(v)
				delete(m.cdMap, v.Flag)
			}
		}
	}
	if len(m.cdMap) == 0 {
		if m.isTickerRunning == true {
			if m.ticker != nil {
				m.ticker.Stop()
			}
			m.isTickerRunning = false
		}
		m.curTotal = 0
	}
}

func (m *CountDownManager) Clear() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for _, v := range m.cdMap {
		if v != nil {
			m.pool.Put(v)
			delete(m.cdMap, v.Flag)
		}
	}
}

// #region
func (m *CountDownManager) startCD() {
	if len(m.cdMap) == 0 {
		m.checkCloseCd()
		return
	}
	if m.ticker == nil {
		m.ticker = time.NewTicker(time.Second)
	} else if m.isTickerRunning == false {
		m.ticker.Reset(time.Second)
	}
	m.isTickerRunning = true
	defer func() {
		if m.isTickerRunning == true {
			m.ticker.Stop()
			m.isTickerRunning = false
			m.curTotal = 0
		}
	}()
	for {
		select {
		case <-m.ticker.C:
			if m.isTickerRunning == false {
				return
			}
			m.curTotal++
			go m.checkHook()
		}
	}
}

func (m *CountDownManager) checkCloseCd() {
	if m.isTickerRunning == true {
		if m.ticker != nil {
			m.ticker.Stop()
		}
		m.isTickerRunning = false
	}
	m.curTotal = 0
}

func (m *CountDownManager) checkHook() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if len(m.cdMap) == 0 {
		m.checkCloseCd()
		return
	}
	for _, v := range m.cdMap {
		if v == nil {
			continue
		}
		v.curCd--
		if m.curTotal >= v.lastDeadline {
			if v.RepeatCnt > 0 {
				v.RepeatCnt--
				if (v.IsEveryExec == true || v.RepeatCnt == 0) && m.isTickerRunning == true && v.hook != nil {
					go v.hook(
						v.Flag,
						v.curCd,
						v.RepeatCnt,
						v.RepeatCnt <= 0,
						v.parameterList,
					)
				}
				if v.RepeatCnt == 0 {
					m.pool.Put(v)
					delete(m.cdMap, v.Flag)
				} else {
					v.lastDeadline = m.curTotal + v.Duration
					if v.IsEveryExec == true {
						v.curCd = time.Duration(v.RepeatCnt)
					} else {
						v.curCd = v.Duration
					}
				}
			} else {
				//永久执行,必须要每duration时间执行一次
				if m.isTickerRunning == true && v.hook != nil {
					go v.hook(
						v.Flag,
						v.curCd,
						v.RepeatCnt,
						false,
						v.parameterList,
					)
				}
			}
		}
	}
}

//#endregion
