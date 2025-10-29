package goroutineUtil

import (
	"fmt"
	"github.com/AMD-AGI/primus-lens/core/pkg/errors"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"runtime"
)

func RecoverFunc(hook func(r any)) func() {
	return func() {
		if r := recover(); r != nil {
			if hook != nil {
				hook(r)
			}
			DefaultRecoveryFunc(r)
		}
	}
}

func DefaultRecoveryFunc(r interface{}) {
	stack := make([]byte, 1<<16)
	stack = stack[:runtime.Stack(stack, false)]
	commonErr := errors.NewError().WithCode(errors.InternalError)
	err, ok := r.(error)
	if ok {
		commonErr = commonErr.WithError(err)
	}
	commonErr = commonErr.WithMessage(fmt.Sprintf("%v", r))
	log.GlobalLogger().Errorf("Panic %v\n%s", commonErr, stack)
}
