package grpcep

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

type testGRPCReq struct {
	Name string
}

type testGRPCResp struct {
	Data string
}

func TestValidateGRPCCallHandler(t *testing.T) {
	req := &testGRPCReq{}
	handler := func(context.Context, *testGRPCReq) (*testGRPCResp, error) {
		return &testGRPCResp{Data: "ok"}, nil
	}

	if err := validateGRPCCallHandler(reflect.ValueOf(req), reflect.ValueOf(handler)); err != nil {
		t.Fatalf("expected valid handler signature: %v", err)
	}
}

func TestValidateGRPCCallHandlerRejectsWrongReturnShape(t *testing.T) {
	req := &testGRPCReq{}
	handler := func(context.Context, *testGRPCReq) *testGRPCResp {
		return &testGRPCResp{Data: "ok"}
	}

	err := validateGRPCCallHandler(reflect.ValueOf(req), reflect.ValueOf(handler))
	if err == nil {
		t.Fatal("expected invalid handler signature error")
	}
	if !strings.Contains(err.Error(), "return 2 values") {
		t.Fatalf("expected return count error, got %v", err)
	}
}

func TestValidateGRPCCallHandlerRejectsWrongParam(t *testing.T) {
	req := &testGRPCReq{}
	handler := func(context.Context, string) (*testGRPCResp, error) {
		return &testGRPCResp{Data: "ok"}, nil
	}

	err := validateGRPCCallHandler(reflect.ValueOf(req), reflect.ValueOf(handler))
	if err == nil {
		t.Fatal("expected invalid handler signature error")
	}
	if !strings.Contains(err.Error(), "second arg") {
		t.Fatalf("expected second arg error, got %v", err)
	}
}
