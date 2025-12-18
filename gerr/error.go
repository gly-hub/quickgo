package gerr

import "fmt"

type GErr struct {
	Code int32
	Msg  string
}

func (e *GErr) Error() string {
	return fmt.Sprintf("code: %d, msg: %s", e.Code, e.Msg)
}

func (e *GErr) GetCode() int32 {
	return e.Code
}

func (e *GErr) GetMsg() string {
	return e.Msg
}

func NewGErr(code int32, msg string) *GErr {
	return &GErr{
		Code: code,
		Msg:  msg,
	}
}
