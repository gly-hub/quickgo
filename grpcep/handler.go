package grpcep

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/team-dandelion/quickgo/gerr"
	"github.com/team-dandelion/quickgo/http"
	"github.com/team-dandelion/quickgo/logger"
	"github.com/team-dandelion/quickgo/tracing"

	jsoniter "github.com/json-iterator/go"

	"github.com/gofiber/fiber/v2"
	"github.com/spf13/cast"
	"google.golang.org/grpc/metadata"
)

type BaseHandler struct {
}

var (
	contextType = reflect.TypeOf((*context.Context)(nil)).Elem()
	errorType   = reflect.TypeOf((*error)(nil)).Elem()
)

// isConnectionClosed 检查错误是否表示连接已关闭
// 用于 SSE 流式响应中检测客户端断开连接
func isConnectionClosed(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.ErrClosedPipe) {
		return true
	}
	errMsg := err.Error()
	return strings.Contains(errMsg, "broken pipe") ||
		strings.Contains(errMsg, "connection reset by peer") ||
		strings.Contains(errMsg, "write: connection closed") ||
		strings.Contains(errMsg, "client disconnected")
}

func (h *BaseHandler) GRPCCall(ctx *fiber.Ctx, param interface{}, handler interface{}) error {
	c := ctx.Context()

	refParam := reflect.ValueOf(param)
	if !refParam.IsValid() || refParam.Kind() != reflect.Ptr {
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
	if err := validateGRPCCallHandler(refParam, refHandler); err != nil {
		logger.Error(c, "rpc_call handler signature error: %v", err)
		return h.Response(ctx, JsonResponse{}, gerr.NewGErr(FailCode, err.Error()))
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

func validateGRPCCallHandler(refParam reflect.Value, refHandler reflect.Value) error {
	if !refHandler.IsValid() || refHandler.Kind() != reflect.Func {
		return errors.New("rpc_call handler is not a function")
	}

	handlerType := refHandler.Type()
	if handlerType.NumIn() != 2 {
		return fmt.Errorf("rpc_call handler must accept 2 args, got %d", handlerType.NumIn())
	}
	if !handlerType.In(0).Implements(contextType) {
		return fmt.Errorf("rpc_call handler first arg must implement context.Context, got %s", handlerType.In(0))
	}
	if !refParam.Type().AssignableTo(handlerType.In(1)) {
		return fmt.Errorf("rpc_call handler second arg must accept %s, got %s", refParam.Type(), handlerType.In(1))
	}

	if handlerType.NumOut() != 2 {
		return fmt.Errorf("rpc_call handler must return 2 values, got %d", handlerType.NumOut())
	}
	if !handlerType.Out(1).Implements(errorType) {
		return fmt.Errorf("rpc_call handler second return must implement error, got %s", handlerType.Out(1))
	}

	return nil
}

func (b *BaseHandler) RPCStream(ctx *fiber.Ctx, param interface{}, streamFunc func(context.Context, interface{}) (interface{}, error)) error {
	// 设置 SSE 相关的响应头
	b.SetSSEStream(ctx)
	// 请求 gRPC 流
	rpcCtx := b.RPCCtx(ctx)
	stream, err := streamFunc(rpcCtx, param)
	if err != nil {
		logger.Error(ctx.Context(), "rpc_stream error: %v", err)
		return err
	}
	// 获取 stream 的 Recv 和 CloseSend 方法
	streamValue := reflect.ValueOf(stream)
	recvMethod := streamValue.MethodByName("Recv")
	closeSendMethod := streamValue.MethodByName("CloseSend")

	// 确保流在不再需要时关闭
	if closeSendMethod.IsValid() {
		defer closeSendMethod.Call(nil)
	}

	ctx.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		// 监听 stream 流数据
		eventId := 0
		for {
			if !recvMethod.IsValid() {
				logger.Error(context.Background(), "rpc_stream receive method is invalid")
				break
			}

			// 调用 Recv 方法获取流数据
			results := recvMethod.Call(nil)
			if len(results) != 2 {
				logger.Error(context.Background(), "rpc_stream receive method return length error")
				break
			}

			// 检查错误
			if !results[1].IsNil() {
				err = results[1].Interface().(error)
				if err == io.EOF {
					break
				}
				logger.Error(context.Background(), "rpc_stream receive method return io.EOF")
				break
			}

			// 获取内容并发送到客户端
			var content string
			// 尝试不同的方法获取内容
			res := results[0].Interface()
			contentValue := reflect.ValueOf(res).MethodByName("GetContent")
			if contentValue.IsValid() {
				content = contentValue.Call(nil)[0].String()
			} else {
				// 尝试直接将结果转换为JSON
				jsonData, jsonErr := jsoniter.Marshal(res)
				if jsonErr == nil {
					content = string(jsonData)
				} else {
					content = fmt.Sprintf("%v", res)
				}
			}
			// 格式化为标准EventStream格式
			eventId++
			sseMessage := fmt.Sprintf("id: %d\ndata: %s\n\n", eventId, content)
			_, err = fmt.Fprint(w, sseMessage)
			if err != nil {
				logger.Error(context.Background(), "rpc_stream write error: %v", err)
				break
			}
			err = w.Flush()
			if err != nil {
				// 检查连接是否已关闭
				if isConnectionClosed(err) {
					logger.Info(context.Background(), "rpc_stream client disconnected: %v", err)
				} else {
					logger.Error(context.Background(), "rpc_stream flush error: %v", err)
				}
				break
			}
		}

		// 发送结束事件
		_, writeErr := fmt.Fprint(w, "event: close\ndata: {\"close\":true}\n\n")
		if writeErr != nil {
			// 连接可能已关闭，无需继续尝试写入
			if isConnectionClosed(writeErr) {
				logger.Info(context.Background(), "rpc_stream client disconnected before close event: %v", writeErr)
			} else {
				logger.Error(context.Background(), "rpc_stream close event write error: %v", writeErr)
			}
			return
		}
		err = w.Flush()
		if err != nil {
			// 检查连接是否已关闭
			if isConnectionClosed(err) {
				logger.Info(context.Background(), "rpc_stream client disconnected on close: %v", err)
			} else {
				logger.Error(context.Background(), "rpc_stream final flush error: %v", err)
			}
		}
	})
	return nil
}

func (h *BaseHandler) ResponseDecorator(byteData []byte, traceID string) string {
	// 先尝试解析为 map，检查是否包含 CommonResp 或 common_resp 字段
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
		} else if commonRespVal, exists := dataMap[CommonRespKeyV2]; exists {
			// 检查是否存在 common_resp 字段（小写版本）
			hasCommonResp = true
			// 解析 common_resp
			if commonRespMap, ok := commonRespVal.(map[string]interface{}); ok {
				if codeVal, ok := commonRespMap["code"].(float64); ok {
					code = int32(codeVal)
				}
				if msgVal, ok := commonRespMap["msg"].(string); ok {
					msg = msgVal
				}
			}
			// 移除 common_resp 字段，因为它不应该出现在最终的 data 中
			delete(dataMap, CommonRespKeyV2)
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

func (h *BaseHandler) RPCCtx(c *fiber.Ctx) context.Context {
	// 1. 获取基础 context（优先级：trace_ctx > UserContext > Context）
	var ctx context.Context

	// 优先从 Locals 中获取 trace context（由 tracing middleware 设置）
	if traceCtx, ok := c.Locals("trace_ctx").(context.Context); ok && traceCtx != nil {
		ctx = traceCtx
	} else {
		// 从 UserContext 获取（Fiber 的标准方式）
		ctx = c.UserContext()
		if ctx == nil {
			// 最后使用 fasthttp context
			ctx = c.Context()
		}
	}

	// 确保 context 不为 nil
	if ctx == nil {
		ctx = context.Background()
	}

	// 2. 收集 UserValues 并创建 gRPC metadata
	userValues := make(map[string]string)
	if fctx := c.Context(); fctx != nil {
		fctx.VisitUserValues(func(key []byte, value interface{}) {
			userValues[string(key)] = cast.ToString(value)
		})
	}

	// 3. 将 UserValues 添加到 outgoing metadata
	if len(userValues) > 0 {
		md := metadata.New(userValues)
		ctx = metadata.NewOutgoingContext(ctx, md)
	}

	// 4. 注入 OpenTelemetry trace context 到 gRPC metadata
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

func (b *BaseHandler) SetSSEStream(ctx *fiber.Ctx) {
	ctx.Set("Content-Type", "text/event-stream")
	ctx.Set("Cache-Control", "no-cache")
	ctx.Set("Connection", "keep-alive")
	ctx.Set("Access-Control-Allow-Origin", "*")
	ctx.Set("Transfer-Encoding", "chunked")
	ctx.Set("X-Accel-Buffering", "no")
}
