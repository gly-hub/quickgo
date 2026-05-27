package grpcep

import (
	"reflect"

	"github.com/team-dandelion/quickgo/gerr"
)

func InitResponse(in interface{}) {
	rv := reflect.ValueOf(in)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return
	}

	// 获取指针指向的元素（应该也是一个指针）
	elem := rv.Elem()
	if elem.Kind() != reflect.Ptr {
		return
	}

	// 获取实际类型并创建新实例
	elemType := elem.Type().Elem()
	newVal := reflect.New(elemType)

	// 查找 CommonResp 字段（支持 proto 生成的 snake_case 和 CamelCase）
	fieldName := ""
	if _, exists := elemType.FieldByName("CommonResp"); exists {
		fieldName = "CommonResp"
	} else if _, exists := elemType.FieldByName("common_resp"); exists {
		fieldName = "common_resp"
	}

	// 如果找到字段，设置默认值
	if fieldName != "" {
		field := newVal.Elem().FieldByName(fieldName)
		if field.IsValid() && field.CanSet() && field.Kind() == reflect.Ptr {
			// 获取字段的实际类型（例如 *gen.CommonResp）
			fieldType := field.Type()
			// 创建该类型的新实例
			newFieldVal := reflect.New(fieldType.Elem())
			// 设置 Code 和 Msg 字段
			if codeField := newFieldVal.Elem().FieldByName("Code"); codeField.IsValid() && codeField.CanSet() {
				codeField.SetInt(int64(SuccessCode))
			}
			if msgField := newFieldVal.Elem().FieldByName("Msg"); msgField.IsValid() && msgField.CanSet() {
				msgField.SetString(SuccessDesc)
			}
			// 设置字段值
			field.Set(newFieldVal)
		}
	}

	// 设置指针值
	elem.Set(newVal)
}

func WithError(ret interface{}, err error) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()

	if IsNilValue(ret) {
		return false
	}

	if err == nil {
		return false
	}

	newErr := gerr.Parse(err)
	code := newErr.GetCode()
	msg := newErr.GetMsg()

	if code == 0 {
		code = InternalErrCode
	}
	if msg == "" {
		msg = InternalErrDesc
	}

	rv := reflect.ValueOf(ret)
	// 解引用指针
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return false
		}
		rv = rv.Elem()
	}

	// 查找 CommonResp 字段
	var field reflect.Value
	if f := rv.FieldByName(CommonRespKey); f.IsValid() {
		field = f
	} else if f := rv.FieldByName(CommonRespKeyV2); f.IsValid() {
		field = f
	}

	if field.IsValid() {
		// 字段存在，获取 CommonResp 指针
		if field.Kind() == reflect.Ptr {
			if field.IsNil() {
				return false
			}
			commonResp := field.Elem()
			setCommonRespFields(commonResp, code, msg)
			return true
		}
	}

	// 类型推断：直接是 CommonResp 类型
	if rv.Type() == reflect.TypeOf(CommonResp{}) {
		setCommonRespFields(rv, code, msg)
		return true
	}

	return false
}

func setCommonRespFields(rv reflect.Value, code int32, msg string) {
	if codeField := rv.FieldByName("Code"); codeField.IsValid() && codeField.CanSet() {
		codeField.SetInt(int64(code))
	}
	if msgField := rv.FieldByName("Msg"); msgField.IsValid() && msgField.CanSet() {
		msgField.SetString(msg)
	}
}
