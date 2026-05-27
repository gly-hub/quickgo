package grpcep

import (
	"errors"
	"testing"

	"github.com/team-dandelion/quickgo/gerr"
)

// 模拟一个带 CommonResp 字段的响应结构（使用 CommonResp 字段名）
type TestResponseWithCommonResp struct {
	CommonResp *CommonResp
	Data       string
}

// 模拟一个带 common_resp 字段的响应结构（使用 common_resp 字段名）
type TestResponseWithCommonRespV2 struct {
	common_resp *CommonResp //nolint:stylecheck,revive
	Data        string
}

// 模拟一个没有 CommonResp 字段的响应结构
type TestResponseWithoutCommonResp struct {
	Data   string
	Status int
}

func TestInitResponse_WithCommonResp(t *testing.T) {
	var resp *TestResponseWithCommonResp
	InitResponse(&resp)

	if resp == nil {
		t.Fatal("InitResponse should initialize resp to non-nil")
	}

	if resp.CommonResp == nil {
		t.Fatal("InitResponse should initialize CommonResp field")
	}

	if resp.CommonResp.Code != SuccessCode {
		t.Errorf("Expected Code=%d, got %d", SuccessCode, resp.CommonResp.Code)
	}

	if resp.CommonResp.Msg != SuccessDesc {
		t.Errorf("Expected Msg=%s, got %s", SuccessDesc, resp.CommonResp.Msg)
	}
}

func TestInitResponse_WithoutCommonResp(t *testing.T) {
	var resp *TestResponseWithoutCommonResp
	InitResponse(&resp)

	if resp == nil {
		t.Fatal("InitResponse should initialize resp to non-nil even without CommonResp field")
	}

	// 没有 CommonResp 字段，Data 应该是零值
	if resp.Data != "" {
		t.Errorf("Expected Data to be empty, got %s", resp.Data)
	}
}

func TestInitResponse_NilInput(t *testing.T) {
	// 测试 nil 输入不会 panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("InitResponse should not panic with nil input, got: %v", r)
		}
	}()

	InitResponse(nil)
}

func TestInitResponse_NonPointerInput(t *testing.T) {
	// 测试非指针输入不会 panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("InitResponse should not panic with non-pointer input, got: %v", r)
		}
	}()

	var resp TestResponseWithCommonResp
	InitResponse(resp)
}

func TestWithError_Success(t *testing.T) {
	resp := &TestResponseWithCommonResp{
		CommonResp: &CommonResp{
			Code: SuccessCode,
			Msg:  SuccessDesc,
		},
	}

	testErr := gerr.NewGErr(40001, "test error message")
	ok := WithError(resp, testErr)

	if !ok {
		t.Fatal("WithError should return true when successfully setting error")
	}

	if resp.CommonResp.Code != 40001 {
		t.Errorf("Expected Code=40001, got %d", resp.CommonResp.Code)
	}

	if resp.CommonResp.Msg != "test error message" {
		t.Errorf("Expected Msg='test error message', got %s", resp.CommonResp.Msg)
	}
}

func TestWithError_NilError(t *testing.T) {
	resp := &TestResponseWithCommonResp{
		CommonResp: &CommonResp{
			Code: SuccessCode,
			Msg:  SuccessDesc,
		},
	}

	ok := WithError(resp, nil)

	if ok {
		t.Fatal("WithError should return false when error is nil")
	}

	// 原值应该保持不变
	if resp.CommonResp.Code != SuccessCode {
		t.Errorf("Expected Code to remain %d, got %d", SuccessCode, resp.CommonResp.Code)
	}
}

func TestWithError_NilResponse(t *testing.T) {
	var resp *TestResponseWithCommonResp

	ok := WithError(resp, errors.New("test error"))

	if ok {
		t.Fatal("WithError should return false when response is nil")
	}
}

func TestWithError_NilCommonResp(t *testing.T) {
	resp := &TestResponseWithCommonResp{
		CommonResp: nil, // CommonResp 未初始化
	}

	ok := WithError(resp, errors.New("test error"))

	if ok {
		t.Fatal("WithError should return false when CommonResp is nil")
	}
}

func TestWithError_StandardError(t *testing.T) {
	resp := &TestResponseWithCommonResp{
		CommonResp: &CommonResp{
			Code: SuccessCode,
			Msg:  SuccessDesc,
		},
	}

	// 使用标准错误（非 GErr）
	standardErr := errors.New("standard error")
	ok := WithError(resp, standardErr)

	if !ok {
		t.Fatal("WithError should return true for standard errors")
	}

	// 标准错误应该使用内部错误码
	if resp.CommonResp.Code != InternalErrCode {
		t.Errorf("Expected Code=%d for standard error, got %d", InternalErrCode, resp.CommonResp.Code)
	}
}

func TestWithError_WithoutCommonRespField(t *testing.T) {
	resp := &TestResponseWithoutCommonResp{
		Data:   "test",
		Status: 200,
	}

	ok := WithError(resp, errors.New("test error"))

	if ok {
		t.Fatal("WithError should return false when response has no CommonResp field")
	}
}

func TestWithError_DirectCommonResp(t *testing.T) {
	resp := &CommonResp{
		Code: SuccessCode,
		Msg:  SuccessDesc,
	}

	testErr := gerr.NewGErr(40002, "direct error")
	ok := WithError(resp, testErr)

	if !ok {
		t.Fatal("WithError should return true for direct CommonResp pointer")
	}

	if resp.Code != 40002 {
		t.Errorf("Expected Code=40002, got %d", resp.Code)
	}

	if resp.Msg != "direct error" {
		t.Errorf("Expected Msg='direct error', got %s", resp.Msg)
	}
}

func TestWithError_DoublePointer(t *testing.T) {
	inner := &TestResponseWithCommonResp{
		CommonResp: &CommonResp{
			Code: SuccessCode,
			Msg:  SuccessDesc,
		},
	}
	resp := &inner

	testErr := gerr.NewGErr(40003, "double pointer error")
	ok := WithError(resp, testErr)

	if !ok {
		t.Fatal("WithError should handle double pointers")
	}

	if inner.CommonResp.Code != 40003 {
		t.Errorf("Expected Code=40003, got %d", inner.CommonResp.Code)
	}
}

func TestWithError_ZeroCode(t *testing.T) {
	resp := &TestResponseWithCommonResp{
		CommonResp: &CommonResp{
			Code: SuccessCode,
			Msg:  SuccessDesc,
		},
	}

	// 创建一个 code 为 0 的 GErr
	testErr := gerr.NewGErr(0, "zero code error")
	ok := WithError(resp, testErr)

	if !ok {
		t.Fatal("WithError should return true")
	}

	// code 为 0 时应该使用 InternalErrCode
	if resp.CommonResp.Code != InternalErrCode {
		t.Errorf("Expected Code=%d when error code is 0, got %d", InternalErrCode, resp.CommonResp.Code)
	}
}

func TestWithError_EmptyMsg(t *testing.T) {
	resp := &TestResponseWithCommonResp{
		CommonResp: &CommonResp{
			Code: SuccessCode,
			Msg:  SuccessDesc,
		},
	}

	// 创建一个 msg 为空的 GErr
	testErr := gerr.NewGErr(40004, "")
	ok := WithError(resp, testErr)

	if !ok {
		t.Fatal("WithError should return true")
	}

	// msg 为空时应该使用 InternalErrDesc
	if resp.CommonResp.Msg != InternalErrDesc {
		t.Errorf("Expected Msg=%s when error msg is empty, got %s", InternalErrDesc, resp.CommonResp.Msg)
	}
}

// 基准测试
func BenchmarkInitResponse(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var resp *TestResponseWithCommonResp
		InitResponse(&resp)
	}
}

func BenchmarkWithError(b *testing.B) {
	testErr := gerr.NewGErr(40001, "benchmark error")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		resp := &TestResponseWithCommonResp{
			CommonResp: &CommonResp{
				Code: SuccessCode,
				Msg:  SuccessDesc,
			},
		}
		WithError(resp, testErr)
	}
}
