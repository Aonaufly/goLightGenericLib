package common

import (
	"math"
)

type LocalPool[T any] struct {
	buf         []T
	cap         uint
	newFunc     func() T
	clearFunc   func(item T)
	destroyFunc func(item T)
}

func NewLocalPool[T any](cap uint, initCreateNum uint, newFunc func() T, clearFunc func(item T), destroyFunc func(item T)) *LocalPool[T] {
	p := &LocalPool[T]{
		cap:         cap,
		newFunc:     newFunc,
		clearFunc:   clearFunc,
		destroyFunc: destroyFunc,
	}
	if initCreateNum > 0 && newFunc != nil {
		targetNum := math.Min(float64(initCreateNum), float64(cap))
		p.buf = make([]T, int(targetNum), cap)
		for i := 0; i < int(targetNum); i++ {
			p.buf[i] = p.newFunc()
		}
	} else {
		p.buf = make([]T, 0, cap)
	}
	return p
}

func (p *LocalPool[T]) Get() (v T) {
	if n := len(p.buf); n > 0 {
		v = p.buf[n-1]
		p.buf = p.buf[:n-1]
		return
	}
	if p.newFunc != nil {
		return p.newFunc()
	}
	return
}

func (p *LocalPool[T]) Put(v T) {
	if len(p.buf) >= int(p.cap) {
		if p.destroyFunc != nil {
			p.destroyFunc(v)
		}
	} else {
		if p.clearFunc != nil {
			p.clearFunc(v)
		}
		p.buf = append(p.buf, v)
	}
}

func (p *LocalPool[T]) Clear() {
	if n := len(p.buf); n > 0 {
		var item T
		for i := 0; i < n; i++ {
			item = p.buf[i]
			if p.destroyFunc != nil {
				p.destroyFunc(item)
			}
		}
		p.buf = p.buf[:0]
	}
}
