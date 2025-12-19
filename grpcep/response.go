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

	// 首先尝试查找 common_resp 字段
	status, exists := rt.FieldByName("common_resp")
	if !exists {
		// 如果没有找到 common_resp，尝试查找 CommonResp 字段
		status, exists = rt.FieldByName("CommonResp")
		if !exists {
			// 如果都没有找到，直接返回，不设置元数据字段
			iPointer := *(*Iface)(unsafe.Pointer(&in))
			*(*uintptr)(iPointer.Value) = dataPointer
			return
		}
	}

	*(**CommonResp)((unsafe.Pointer)(dataPointer + status.Offset)) = &CommonResp{
		Code: SuccessCode,
		Msg:  SuccessDesc,
	}

	iPointer := *(*Iface)(unsafe.Pointer(&in))
	*(*uintptr)(iPointer.Value) = dataPointer

	return
}
