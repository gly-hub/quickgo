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
	if streamFunc == nil {
		return errors.New("rpc_stream function is nil")
	}

	// 请求 gRPC 流
	rpcCtx, cancel := context.WithCancel(b.RPCCtx(ctx))
	stream, err := streamFunc(rpcCtx, param)
	if err != nil {
		cancel()
		logger.Error(ctx.Context(), "rpc_stream error: %v", err)
		return err
	}
	recvMethod, closeSendMethod, err := rpcStreamMethods(stream)
	if err != nil {
		cancel()
		logger.Error(ctx.Context(), "rpc_stream signature error: %v", err)
		return err
	}

	// 设置 SSE 相关的响应头
	b.SetSSEStream(ctx)
	ctx.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer cancel()
		defer callCloseSend(closeSendMethod)

		// 监听 stream 流数据
		eventId := 0
		for {
			// 调用 Recv 方法获取流数据
			results := recvMethod.Call(nil)
			// 检查错误
			if recvErr, _ := results[1].Interface().(error); recvErr != nil {
				if errors.Is(recvErr, io.EOF) {
					break
				}
				logger.Error(context.Background(), "rpc_stream receive error: %v", recvErr)
				break
			}

			// 获取内容并发送到客户端
			res := results[0].Interface()
			content := streamContent(res)
			// 格式化为标准 EventStream 格式。
			eventId++
			sseMessage := formatSSEMessage(eventId, content)
			if _, writeErr := fmt.Fprint(w, sseMessage); writeErr != nil {
				logger.Error(context.Background(), "rpc_stream write error: %v", writeErr)
				return
			}
			if flushErr := w.Flush(); flushErr != nil {
				// 检查连接是否已关闭
				if isConnectionClosed(flushErr) {
					logger.Info(context.Background(), "rpc_stream client disconnected: %v", flushErr)
				} else {
					logger.Error(context.Background(), "rpc_stream flush error: %v", flushErr)
				}
				return
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
		if flushErr := w.Flush(); flushErr != nil {
			// 检查连接是否已关闭
			if isConnectionClosed(flushErr) {
				logger.Info(context.Background(), "rpc_stream client disconnected on close: %v", flushErr)
			} else {
				logger.Error(context.Background(), "rpc_stream final flush error: %v", flushErr)
			}
		}
	})
	return nil
}

func formatSSEMessage(eventID int, content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	var message strings.Builder
	fmt.Fprintf(&message, "id: %d\n", eventID)
	for _, line := range strings.Split(content, "\n") {
		message.WriteString("data: ")
		message.WriteString(line)
		message.WriteByte('\n')
	}
	message.WriteByte('\n')
	return message.String()
}

func rpcStreamMethods(stream interface{}) (reflect.Value, reflect.Value, error) {
	streamValue := reflect.ValueOf(stream)
	if !streamValue.IsValid() || isNilReflectValue(streamValue) {
		return reflect.Value{}, reflect.Value{}, errors.New("rpc_stream is nil")
	}

	recvMethod := streamValue.MethodByName("Recv")
	if !recvMethod.IsValid() {
		return reflect.Value{}, reflect.Value{}, errors.New("rpc_stream must expose Recv")
	}
	recvType := recvMethod.Type()
	if recvType.NumIn() != 0 || recvType.NumOut() != 2 || !recvType.Out(1).Implements(errorType) {
		return reflect.Value{}, reflect.Value{}, errors.New("rpc_stream Recv must have signature func() (response, error)")
	}

	closeSendMethod := streamValue.MethodByName("CloseSend")
	if closeSendMethod.IsValid() {
		closeSendType := closeSendMethod.Type()
		if closeSendType.NumIn() != 0 || (closeSendType.NumOut() == 1 && !closeSendType.Out(0).Implements(errorType)) || closeSendType.NumOut() > 1 {
			return reflect.Value{}, reflect.Value{}, errors.New("rpc_stream CloseSend must have signature func() or func() error")
		}
	}
	return recvMethod, closeSendMethod, nil
}

func callCloseSend(closeSendMethod reflect.Value) {
	if !closeSendMethod.IsValid() {
		return
	}
	results := closeSendMethod.Call(nil)
	if len(results) == 1 {
		if err, _ := results[0].Interface().(error); err != nil {
			logger.Error(context.Background(), "rpc_stream close send error: %v", err)
		}
	}
}

func streamContent(response interface{}) string {
	responseValue := reflect.ValueOf(response)
	if responseValue.IsValid() && !isNilReflectValue(responseValue) {
		getContent := responseValue.MethodByName("GetContent")
		if getContent.IsValid() {
			getContentType := getContent.Type()
			if getContentType.NumIn() == 0 && getContentType.NumOut() == 1 && getContentType.Out(0).Kind() == reflect.String {
				return getContent.Call(nil)[0].String()
			}
		}
	}
	jsonData, err := jsoniter.Marshal(response)
	if err == nil {
		return string(jsonData)
	}
	return fmt.Sprintf("%v", response)
}

func isNilReflectValue(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func (h *BaseHandler) ResponseDecorator(byteData []byte, traceID string) string {
	// 只解码顶层字段；未处理的 data 保持原始 JSON，避免完整响应二次反序列化。
	var dataMap map[string]jsoniter.RawMessage
	var code int32 = SuccessCode
	var msg string = SuccessDesc
	var hasCommonResp bool
	var hasCodeAndMsg bool

	if err := jsoniter.Unmarshal(byteData, &dataMap); err == nil {
		// 成功解析为 map，检查是否存在 CommonResp 字段
		if commonRespVal, exists := dataMap[CommonRespKey]; exists {
			hasCommonResp = true
			code, msg = decodeCommonResp(commonRespVal, code, msg)
			// 移除 CommonResp 字段，因为它不应该出现在最终的 data 中
			delete(dataMap, CommonRespKey)
		} else if commonRespVal, exists := dataMap[CommonRespKeyV2]; exists {
			// 检查是否存在 common_resp 字段（小写版本）
			hasCommonResp = true
			code, msg = decodeCommonResp(commonRespVal, code, msg)
			// 移除 common_resp 字段，因为它不应该出现在最终的 data 中
			delete(dataMap, CommonRespKeyV2)
		} else {
			// 如果没有 CommonResp，检查是否有 code 和 message 字段（proto 响应格式）
			if codeVal, exists := dataMap["code"]; exists {
				var ok bool
				code, ok = decodeJSONCode(codeVal, code)
				if ok {
					hasCodeAndMsg = true
				}
			}
			if msgVal, exists := dataMap["message"]; exists {
				var ok bool
				msg, ok = decodeJSONString(msgVal, msg)
				if ok {
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
		jsonResp.Data = jsoniter.RawMessage(byteData)
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

func decodeCommonResp(data jsoniter.RawMessage, defaultCode int32, defaultMsg string) (int32, string) {
	var commonResp map[string]jsoniter.RawMessage
	if err := jsoniter.Unmarshal(data, &commonResp); err != nil {
		return defaultCode, defaultMsg
	}
	if code, ok := decodeJSONCode(commonResp["code"], defaultCode); ok {
		defaultCode = code
	}
	if msg, ok := decodeJSONString(commonResp["msg"], defaultMsg); ok {
		defaultMsg = msg
	}
	return defaultCode, defaultMsg
}

func decodeJSONCode(data jsoniter.RawMessage, defaultCode int32) (int32, bool) {
	if len(data) == 0 || string(data) == "null" {
		return defaultCode, false
	}
	var code float64
	if err := jsoniter.Unmarshal(data, &code); err != nil {
		return defaultCode, false
	}
	return int32(code), true
}

func decodeJSONString(data jsoniter.RawMessage, defaultMsg string) (string, bool) {
	if len(data) == 0 || string(data) == "null" {
		return defaultMsg, false
	}
	var msg string
	if err := jsoniter.Unmarshal(data, &msg); err != nil {
		return defaultMsg, false
	}
	return msg, true
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

	// 2. 只转发明确允许的请求头和 grpc-metadata-* Locals。
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	for _, header := range []string{"authorization", "x-request-id", "x-trace-id"} {
		if value := c.Get(header); value != "" {
			md.Set(header, value)
		}
	}
	if fctx := c.Context(); fctx != nil {
		fctx.VisitUserValues(func(key []byte, value interface{}) {
			metadataKey := strings.TrimPrefix(strings.ToLower(string(key)), "grpc-metadata-")
			if metadataKey != strings.ToLower(string(key)) && validMetadataKey(metadataKey) {
				md.Set(metadataKey, cast.ToString(value))
			}
		})
	}
	if len(md) > 0 {
		ctx = metadata.NewOutgoingContext(ctx, md)
	}

	// 4. 注入 OpenTelemetry trace context 到 gRPC metadata
	if tracing.IsEnabled() {
		ctx = tracing.InjectTraceContext(ctx)
	}

	return ctx
}

func validMetadataKey(key string) bool {
	if key == "" {
		return false
	}
	for _, char := range key {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return true
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
		switch typedErr := err.(type) {
		case *gerr.GErr:
			errCode = typedErr.GetCode()
			errMsg = typedErr.GetMsg()
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
