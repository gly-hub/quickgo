package grpcep

import (
	"context"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
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

func TestSetSSEStreamDoesNotChangeServerTimeouts(t *testing.T) {
	app := fiber.New(fiber.Config{
		ReadTimeout:           3 * time.Second,
		WriteTimeout:          4 * time.Second,
		DisableStartupMessage: true,
	})
	app.Get("/stream", func(ctx *fiber.Ctx) error {
		(&BaseHandler{}).SetSSEStream(ctx)
		return ctx.SendStatus(fiber.StatusNoContent)
	})

	if _, err := app.Test(httptest.NewRequest("GET", "/stream", nil)); err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if got := app.Server().ReadTimeout; got != 3*time.Second {
		t.Fatalf("expected read timeout to remain unchanged, got %s", got)
	}
	if got := app.Server().WriteTimeout; got != 4*time.Second {
		t.Fatalf("expected write timeout to remain unchanged, got %s", got)
	}
	if got := app.Server().MaxKeepaliveDuration; got != 0 {
		t.Fatalf("expected max keepalive duration to remain unchanged, got %s", got)
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
