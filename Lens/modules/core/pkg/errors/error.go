package errors

import (
	"fmt"
	"runtime"
	"strings"
)

type Error struct {
	Stack      []runtime.Frame
	InnerError error
	Code       int
	Message    string
}

func (e *Error) Error() string {
	if e.InnerError == nil {
		return fmt.Sprintf(" code %d.message %s \nstack %s", e.Code, e.Message, e.GetStackString())
	}
	return fmt.Sprintf("error %s code %d message %s \nstack %s", e.InnerError.Error(), e.Code, e.Message, e.GetStackString())
}

func (e *Error) GetStackString() string {
	result := ""
	for _, frame := range e.Stack {
		funcName := ""
		if frame.Func != nil {
			funcName = frame.Func.Name()
		}
		funcNames := strings.Split(funcName, "/")
		if len(funcNames) > 0 {
			funcName = funcNames[len(funcNames)-1]
		}
		result = fmt.Sprintf("%s%s:%d %s\n", result, frame.File, frame.Line, funcName)
	}
	return result
}

func (e *Error) WithCode(code int) *Error {
	e.Code = code
	return e
}

func (e *Error) WithMessage(message string) *Error {
	e.Message = message
	return e
}

func (e *Error) WithMessagef(message string, args ...interface{}) *Error {
	e.Message = fmt.Sprintf(message, args...)
	return e
}

func (e *Error) WithError(err error) *Error {
	e.InnerError = err
	return e
}

func NewError() *Error {
	return newError(2)
}

func newError(callerSkip int) *Error {
	return &Error{
		Stack:      callers(callerSkip),
		InnerError: nil,
		Code:       0,
		Message:    "",
	}
}

func WrapError(err error, message string, code int) *Error {
	return newError(2).WithCode(code).WithMessage(message).WithError(err)
}

func WrapMessage(message string, code int) *Error {
	return newError(2).WithCode(code).WithMessage(message)
}

func callers(callerSkip int) []runtime.Frame {
	rpc := make([]uintptr, 10)
	result := []runtime.Frame{}
	n := runtime.Callers(callerSkip+2, rpc)
	if n < 1 {
		return result
	}
	frames := runtime.CallersFrames(rpc)
	if frames == nil {
		return result
	}
	for frame, more := frames.Next(); more; {
		result = append(result, frame)
		frame, more = frames.Next()
	}
	return result
}
