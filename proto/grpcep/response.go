package grpcep

import (
	"reflect"
	"unsafe"
)

func InitResponse(in interface{}) {
	rt := reflect.TypeOf(in)
	rt = rt.Elem().Elem()

	v := reflect.New(rt)
	dataPointer := v.Pointer()
	status, exists := rt.FieldByName(CommonRespKey)
	if !exists {
		panic("`CommonResp` field not found")
	}

	*(**CommonResp)((unsafe.Pointer)(dataPointer + status.Offset)) = &CommonResp{
		Code: SuccessCode,
		Msg:  SuccessDesc,
	}

	iPointer := *(*Iface)(unsafe.Pointer(&in))
	*(*uintptr)(iPointer.Value) = dataPointer

	return
}
