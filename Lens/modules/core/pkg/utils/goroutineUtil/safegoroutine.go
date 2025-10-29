package goroutineUtil

import (
	"log"
	"runtime"
)

func RunGoroutineWithLog(f func()) {
	go SafeGoroutineWithLog(f)
}

func SafeGoroutineWithLog(f func()) {
	SafeGoroutine(f, DefaultRecoveryFunc)
}

func SafeGoroutine(f func(), recoveryCallBack ...RecoveryFallBackFunc) {
	defer Recovery(recoveryCallBack...)
	f()
}

func Recovery(funcs ...RecoveryFallBackFunc) {
	if r := recover(); r != nil {
		recovered := false
		if len(funcs) > 0 {
			for _, fun := range funcs {
				if fun != nil {
					fun(r)
					recovered = true
				}
			}
		}
		if !recovered {
			buf := make([]byte, 1<<18)
			n := runtime.Stack(buf, false)
			log.Printf("%v, STACK: %s", r, buf[0:n])
		}
	}
}

type RecoveryFallBackFunc func(interface{})
