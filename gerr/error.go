package gerr

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
)

// ErrorType 错误类型
type ErrorType int

const (
	// TypeUnknown 未知错误
	TypeUnknown ErrorType = iota
	// TypeBusiness 业务错误（可预期的业务逻辑错误）
	TypeBusiness
	// TypeValidation 参数验证错误
	TypeValidation
	// TypeNotFound 资源未找到
	TypeNotFound
	// TypeUnauthorized 未授权
	TypeUnauthorized
	// TypeForbidden 禁止访问
	TypeForbidden
	// TypeInternal 内部系统错误
	TypeInternal
	// TypeNetwork 网络错误
	TypeNetwork
	// TypeTimeout 超时错误
	TypeTimeout
	// TypeDatabase 数据库错误
	TypeDatabase
	// TypeThirdParty 第三方服务错误
	TypeThirdParty
)

// String 返回错误类型的字符串表示
func (t ErrorType) String() string {
	switch t {
	case TypeBusiness:
		return "BUSINESS"
	case TypeValidation:
		return "VALIDATION"
	case TypeNotFound:
		return "NOT_FOUND"
	case TypeUnauthorized:
		return "UNAUTHORIZED"
	case TypeForbidden:
		return "FORBIDDEN"
	case TypeInternal:
		return "INTERNAL"
	case TypeNetwork:
		return "NETWORK"
	case TypeTimeout:
		return "TIMEOUT"
	case TypeDatabase:
		return "DATABASE"
	case TypeThirdParty:
		return "THIRD_PARTY"
	default:
		return "UNKNOWN"
	}
}

// GErr 增强的错误结构
type GErr struct {
	Code     int32             // 错误码
	Msg      string            // 错误消息
	Type     ErrorType         // 错误类型
	Cause    error             // 原始错误（错误链）
	Stack    []string          // 堆栈信息
	Metadata map[string]string // 额外的元数据信息
}

// Error 实现 error 接口
func (e *GErr) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("code: %d, type: %s, msg: %s, cause: %v", e.Code, e.Type.String(), e.Msg, e.Cause)
	}
	return fmt.Sprintf("code: %d, type: %s, msg: %s", e.Code, e.Type.String(), e.Msg)
}

// GetCode 获取错误码
func (e *GErr) GetCode() int32 {
	return e.Code
}

// GetMsg 获取错误消息
func (e *GErr) GetMsg() string {
	return e.Msg
}

// GetType 获取错误类型
func (e *GErr) GetType() ErrorType {
	return e.Type
}

// GetCause 获取原始错误
func (e *GErr) GetCause() error {
	return e.Cause
}

// GetStack 获取堆栈信息
func (e *GErr) GetStack() []string {
	return e.Stack
}

// GetMetadata 获取元数据
func (e *GErr) GetMetadata() map[string]string {
	return e.Metadata
}

// GetMetadataValue 获取指定 key 的元数据
func (e *GErr) GetMetadataValue(key string) string {
	if e.Metadata == nil {
		return ""
	}
	return e.Metadata[key]
}

// Unwrap 实现 errors.Unwrap 接口，支持错误链
func (e *GErr) Unwrap() error {
	return e.Cause
}

// Is 实现 errors.Is 接口
func (e *GErr) Is(target error) bool {
	if t, ok := target.(*GErr); ok {
		return e.Code == t.Code
	}
	return false
}

// WithMetadata 添加元数据（链式调用）
func (e *GErr) WithMetadata(key, value string) *GErr {
	if e.Metadata == nil {
		e.Metadata = make(map[string]string)
	}
	e.Metadata[key] = value
	return e
}

// WithCause 添加原因错误（链式调用）
func (e *GErr) WithCause(cause error) *GErr {
	e.Cause = cause
	return e
}

// WithType 设置错误类型（链式调用）
func (e *GErr) WithType(t ErrorType) *GErr {
	e.Type = t
	return e
}

// IsType 检查是否为指定类型的错误
func (e *GErr) IsType(t ErrorType) bool {
	return e.Type == t
}

// IsRetryable 判断错误是否可重试
func (e *GErr) IsRetryable() bool {
	switch e.Type {
	case TypeNetwork, TypeTimeout, TypeThirdParty:
		return true
	default:
		return false
	}
}

// StackTrace 格式化堆栈信息为字符串
func (e *GErr) StackTrace() string {
	if len(e.Stack) == 0 {
		return ""
	}
	return strings.Join(e.Stack, "\n")
}

// ==================== 构造函数 ====================

// NewGErr 创建基本错误（向后兼容）
func NewGErr(code int32, msg string) *GErr {
	return &GErr{
		Code:  code,
		Msg:   msg,
		Type:  TypeBusiness,
		Stack: captureStack(2),
	}
}

// New 创建带类型的错误
func New(code int32, errType ErrorType, msg string) *GErr {
	return &GErr{
		Code:  code,
		Msg:   msg,
		Type:  errType,
		Stack: captureStack(2),
	}
}

// Newf 创建带格式化消息的错误
func Newf(code int32, errType ErrorType, format string, args ...interface{}) *GErr {
	return &GErr{
		Code:  code,
		Msg:   fmt.Sprintf(format, args...),
		Type:  errType,
		Stack: captureStack(2),
	}
}

// Wrap 包装已有错误
func Wrap(err error, code int32, msg string) *GErr {
	if err == nil {
		return nil
	}
	return &GErr{
		Code:  code,
		Msg:   msg,
		Type:  TypeInternal,
		Cause: err,
		Stack: captureStack(2),
	}
}

// Wrapf 包装已有错误（带格式化消息）
func Wrapf(err error, code int32, format string, args ...interface{}) *GErr {
	if err == nil {
		return nil
	}
	return &GErr{
		Code:  code,
		Msg:   fmt.Sprintf(format, args...),
		Type:  TypeInternal,
		Cause: err,
		Stack: captureStack(2),
	}
}

// ==================== 快捷构造函数 ====================

// NewBusiness 创建业务错误
func NewBusiness(code int32, msg string) *GErr {
	return New(code, TypeBusiness, msg)
}

// NewValidation 创建参数验证错误
func NewValidation(code int32, msg string) *GErr {
	return New(code, TypeValidation, msg)
}

// NewNotFound 创建资源未找到错误
func NewNotFound(code int32, msg string) *GErr {
	return New(code, TypeNotFound, msg)
}

// NewUnauthorized 创建未授权错误
func NewUnauthorized(code int32, msg string) *GErr {
	return New(code, TypeUnauthorized, msg)
}

// NewForbidden 创建禁止访问错误
func NewForbidden(code int32, msg string) *GErr {
	return New(code, TypeForbidden, msg)
}

// NewInternal 创建内部系统错误
func NewInternal(code int32, msg string) *GErr {
	return New(code, TypeInternal, msg)
}

// NewNetwork 创建网络错误
func NewNetwork(code int32, msg string) *GErr {
	return New(code, TypeNetwork, msg)
}

// NewTimeout 创建超时错误
func NewTimeout(code int32, msg string) *GErr {
	return New(code, TypeTimeout, msg)
}

// NewDatabase 创建数据库错误
func NewDatabase(code int32, msg string) *GErr {
	return New(code, TypeDatabase, msg)
}

// NewThirdParty 创建第三方服务错误
func NewThirdParty(code int32, msg string) *GErr {
	return New(code, TypeThirdParty, msg)
}

// ==================== 工具函数 ====================

// Parse 解析错误为 GErr
func Parse(err error) *GErr {
	if err == nil {
		return nil
	}

	// 已经是 GErr
	if e, ok := err.(*GErr); ok {
		return e
	}

	// 尝试从错误链中查找 GErr
	var gErr *GErr
	if errors.As(err, &gErr) {
		return gErr
	}

	// 返回包装后的通用错误
	return &GErr{
		Code:  0,
		Msg:   err.Error(),
		Type:  TypeUnknown,
		Cause: err,
		Stack: captureStack(2),
	}
}

// IsGErr 检查是否为 GErr 类型
func IsGErr(err error) bool {
	var gErr *GErr
	return errors.As(err, &gErr)
}

// GetCode 从错误中提取错误码
func GetCode(err error) int32 {
	if gErr := Parse(err); gErr != nil {
		return gErr.Code
	}
	return 0
}

// GetType 从错误中提取错误类型
func GetType(err error) ErrorType {
	if gErr := Parse(err); gErr != nil {
		return gErr.Type
	}
	return TypeUnknown
}

// IsType 检查错误是否为指定类型
func IsType(err error, t ErrorType) bool {
	if gErr := Parse(err); gErr != nil {
		return gErr.Type == t
	}
	return false
}

// IsRetryable 检查错误是否可重试
func IsRetryable(err error) bool {
	if gErr := Parse(err); gErr != nil {
		return gErr.IsRetryable()
	}
	return false
}

// captureStack 捕获调用堆栈
func captureStack(skip int) []string {
	const maxDepth = 10
	stack := make([]string, 0, maxDepth)

	for i := skip; i < skip+maxDepth; i++ {
		pc, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}

		fn := runtime.FuncForPC(pc)
		funcName := "unknown"
		if fn != nil {
			funcName = fn.Name()
		}

		// 过滤掉 runtime 相关的调用
		if strings.HasPrefix(funcName, "runtime.") {
			continue
		}

		stack = append(stack, fmt.Sprintf("%s:%d %s", file, line, funcName))
	}

	return stack
}
