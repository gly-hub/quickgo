package grpcep

import (
	"context"
	"errors"
	"github.com/team-dandelion/quickgo/gerr"
	"github.com/team-dandelion/quickgo/http"
	"github.com/team-dandelion/quickgo/logger"
	"github.com/team-dandelion/quickgo/tracing"
	"reflect"

	jsoniter "github.com/json-iterator/go"

	"github.com/gofiber/fiber/v2"
	"github.com/spf13/cast"
	"google.golang.org/grpc/metadata"
)

type BaseHandler struct {
}

func (h *BaseHandler) GRPCCall(ctx *fiber.Ctx, param interface{}, handler interface{}) error {
	c := ctx.Context()

	refParam := reflect.ValueOf(param)
	if refParam.Kind() != reflect.Ptr {
		logger.Error(c, "rpc_call param is not a pointer")
		return errors.New("rpc_call param is not a pointer")
	}

	if len(ctx.Body()) > 0 {
		err := h.ParseJson(ctx, param)
		if err != nil {
			logger.Error(c, "parse json error: %v", err)
			return h.Response(ctx, JsonResponse{
				Code: ParamsErrCode,
				Msg:  err.Error(),
			}, err)
		}
	}

	refHandler := reflect.ValueOf(handler)
	if refHandler.Kind() != reflect.Func {
		return gerr.NewGErr(FailCode, "WRONG handler")
	}

	rpcCxt := h.RPCCtx(ctx)
	var rets []reflect.Value
	inParam := []reflect.Value{reflect.ValueOf(rpcCxt), refParam}
	rets = refHandler.Call(inParam)

	if !rets[1].IsNil() {
		err := rets[1].Interface().(error)
		return h.Response(ctx, JsonResponse{}, gerr.NewGErr(InternalErrCode, err.Error()))
	}

	// 对rpc响应内容进行处理
	byteData, _ := jsoniter.Marshal(rets[0].Interface())
	resp := h.ResponseDecorator(byteData, http.GetTraceID(ctx))
	ctx.Response().Header.Add("Content-Type", fiber.MIMEApplicationJSON)
	_, err := ctx.WriteString(resp)
	return err
}

func (h *BaseHandler) ResponseDecorator(byteData []byte, traceID string) string {
	// 先尝试解析为 map，检查是否包含 CommonResp 或 code/message 字段
	var dataMap map[string]interface{}
	var code int32 = SuccessCode
	var msg string = SuccessDesc
	var hasCommonResp bool
	var hasCodeAndMsg bool

	if err := jsoniter.Unmarshal(byteData, &dataMap); err == nil {
		// 成功解析为 map，检查是否存在 CommonResp 字段
		if commonRespVal, exists := dataMap[CommonRespKey]; exists {
			hasCommonResp = true
			// 解析 CommonResp
			if commonRespMap, ok := commonRespVal.(map[string]interface{}); ok {
				if codeVal, ok := commonRespMap["code"].(float64); ok {
					code = int32(codeVal)
				}
				if msgVal, ok := commonRespMap["msg"].(string); ok {
					msg = msgVal
				}
			}
			// 移除 CommonResp 字段，因为它不应该出现在最终的 data 中
			delete(dataMap, CommonRespKey)
		} else {
			// 如果没有 CommonResp，检查是否有 code 和 message 字段（proto 响应格式）
			if codeVal, exists := dataMap["code"]; exists {
				if codeFloat, ok := codeVal.(float64); ok {
					code = int32(codeFloat)
					hasCodeAndMsg = true
				}
			}
			if msgVal, exists := dataMap["message"]; exists {
				if msgStr, ok := msgVal.(string); ok {
					msg = msgStr
					hasCodeAndMsg = true
				}
			}

			// 如果提取了 code 和 message，从 dataMap 中移除它们
			if hasCodeAndMsg {
				delete(dataMap, "code")
				delete(dataMap, "message")
			}
		}
	}

	// 构建 JsonResponse
	jsonResp := JsonResponse{
		Code:      code,
		Msg:       msg,
		RequestId: traceID,
	}

	// 设置 Data 字段
	if hasCommonResp || hasCodeAndMsg {
		// 如果存在 CommonResp 或提取了 code/message，使用处理后的 dataMap
		if len(dataMap) == 0 {
			jsonResp.Data = nil
		} else {
			jsonResp.Data = dataMap
		}
	} else {
		// 如果没有 CommonResp 也没有 code/message，将原始数据解析为任意类型作为 data
		var rawData interface{}
		if err := jsoniter.Unmarshal(byteData, &rawData); err == nil {
			jsonResp.Data = rawData
		} else {
			// 如果解析失败，使用原始字节数据
			jsonResp.Data = jsoniter.RawMessage(byteData)
		}
	}

	// 序列化为 JSON 字符串
	result, err := jsoniter.Marshal(jsonResp)
	if err != nil {
		// 如果序列化失败，返回错误响应
		errorResp := JsonResponse{
			Code:      InternalErrCode,
			Msg:       InternalErrDesc,
			Data:      nil,
			RequestId: traceID,
		}
		result, _ = jsoniter.Marshal(errorResp)
		return string(result)
	}

	return string(result)
}

func (h *BaseHandler) RPCCtx(c *fiber.Ctx) (ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			ctx = c.Context()
			return
		}
	}()

	// 1. 优先从 Locals 中获取 trace context（由 tracing middleware 设置）
	if traceCtx, ok := c.Locals("trace_ctx").(context.Context); ok && traceCtx != nil {
		ctx = traceCtx
	} else {
		// 2. 如果没有，从 UserContext 获取（Fiber 的标准方式）
		ctx = c.UserContext()
		if ctx == nil {
			ctx = c.Context()
		}
	}

	// 3. 收集 UserValues 到 metadata（保留原有逻辑）
	param := map[string]string{}
	c.Context().VisitUserValues(func(bytes []byte, i interface{}) {
		k := string(bytes)
		v := cast.ToString(i)
		param[k] = v
	})

	// 4. 创建 gRPC metadata，包含 UserValues
	userValuesMD := metadata.New(param)

	// 5. 将 UserValues 的 metadata 设置到 context 中
	ctx = metadata.NewOutgoingContext(ctx, userValuesMD)

	// 6. 将 trace context 注入到 gRPC metadata 中（用于 OpenTelemetry 链路追踪）
	// InjectTraceContext 会将 trace context 添加到已有的 metadata 中
	if tracing.IsEnabled() {
		ctx = tracing.InjectTraceContext(ctx)
	}

	return ctx
}

func (h *BaseHandler) ParseJson(c *fiber.Ctx, param interface{}) error {
	err := c.BodyParser(param)
	if err != nil {
		return gerr.NewGErr(ParamsErrCode, ParamsErrDesc+"err:"+err.Error())
	}
	return StructValidator(param)
}

func (h *BaseHandler) Response(ctx *fiber.Ctx, respData JsonResponse, err error) error {
	if respData.HttpStatus > 0 {
		ctx.Status(respData.HttpStatus)
	}

	respData.Code, respData.Msg = h.msgAndCodeParser(respData.Code, respData.Msg, err)
	respData.RequestId = http.GetTraceID(ctx)

	return ctx.JSON(respData)
}

func (h *BaseHandler) msgAndCodeParser(code int32, msg string, err error) (int32, string) {
	if code > 0 && msg != "" {
		return code, msg
	}
	var errCode int32
	var errMsg string
	if err != nil {
		switch err.(type) {
		case *gerr.GErr:
			errCode = err.(*gerr.GErr).GetCode()
			errMsg = err.(*gerr.GErr).GetMsg()
		default:
			errCode = FailCode
			errMsg = err.Error()
		}
	}

	if code == 0 {
		if errCode > 0 {
			code = errCode
		} else {
			code = SuccessCode
		}
	}

	if msg == "" {
		if errMsg != "" {
			msg = errMsg
		} else {
			msg = SuccessDesc
		}
	}

	return code, msg
}
