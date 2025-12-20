package grpcep

import (
	"reflect"
	"unsafe"

	"github.com/team-dandelion/quickgo/gerr"
)

var statusCodeOffset = uintptr(0)
var statusMsgOffset = unsafe.Sizeof(0)

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

func WithError(ret interface{}, err error) (ok bool) {
	defer func() {
		if err1 := recover(); err1 != nil {
			ok = false
			return
		}
	}()
	ok = true
	if IsNilValue(ret) {
		return false
	}

	if err == nil {
		return false
	}

	newErr := gerr.Parse(err)
	var (
		code = newErr.GetCode()
		msg  = newErr.GetMsg()
	)
	if code == 0 {
		code = InternalErrCode
	}
	if msg == "" {
		msg = InternalErrDesc
	}

	rt := reflect.TypeOf(ret)
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}

	statusField, exists := rt.FieldByName(CommonRespKey)
	if !exists {
		statusField, exists = rt.FieldByName(CommonRespKeyV2)
	}
	iVal := *(*Iface)(unsafe.Pointer(&ret))
	valPos := uintptr(iVal.Value)
	if exists {
		// 替换赋值
		status := *(*uintptr)(unsafe.Pointer(valPos + statusField.Offset))
		if status == 0 {
			return false
		}
		*(*int32)(unsafe.Pointer(status + statusCodeOffset)) = code
		*(*string)(unsafe.Pointer(status + statusMsgOffset)) = msg
	} else {
		// 类型推断
		if _, ok = ret.(*CommonResp); ok {
			*(*int32)(unsafe.Pointer(valPos + statusCodeOffset)) = code
			*(*string)(unsafe.Pointer(valPos + statusMsgOffset)) = msg
		} else {
			return false
		}
	}

	return ok
}
