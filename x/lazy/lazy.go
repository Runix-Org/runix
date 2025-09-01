package lazy

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type state uint32

const (
	stNew state = iota
	stOK
	stFailed
)

type Value[T any] struct {
	mu   sync.Mutex
	st   atomic.Uint32
	val  *T
	name string
}

func New[T any](name string) *Value[T] {
	return &Value[T]{name: name}
}

func (v *Value[T]) Init(initFn func() (*T, error)) (err error) {
	if state(v.st.Load()) != stNew {
		panic(fmt.Sprintf("double init of module %s", v.name))
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if state(v.st.Load()) != stNew {
		panic(fmt.Sprintf("double init of module %s", v.name))
	}

	defer func() {
		if r := recover(); r != nil {
			v.st.Store(uint32(stFailed))
			panic(r)
		}
	}()

	val, err := initFn()
	if err != nil {
		v.st.Store(uint32(stFailed))
		return err
	}

	v.val = val
	v.st.Store(uint32(stOK))

	return nil
}

func (v *Value[T]) Get() *T {
	switch state(v.st.Load()) {
	case stOK:
		return v.val
	case stFailed:
		panic(fmt.Sprintf("module %s initialization failed", v.name))
	default:
		panic(fmt.Sprintf("using the %s module before init", v.name))
	}
}
