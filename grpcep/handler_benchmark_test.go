package grpcep

import (
	"context"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gofiber/fiber/v2"
)

type benchmarkRequest struct {
	ID int `json:"id"`
}

type benchmarkResponse struct {
	CommonResp *CommonResp `json:"CommonResp,omitempty"`
	Payload    string      `json:"payload"`
}

var benchmarkResponseJSON = []byte(`{"CommonResp":{"code":0,"msg":"success"},"payload":"ok"}`)

func BenchmarkValidateGRPCCallHandler(b *testing.B) {
	param := reflect.ValueOf(&benchmarkRequest{})
	handler := reflect.ValueOf(func(context.Context, *benchmarkRequest) (*benchmarkResponse, error) {
		return nil, nil
	})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := validateGRPCCallHandler(param, handler); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFormatSSEMessage(b *testing.B) {
	content := "first line\nsecond line\nthird line"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = formatSSEMessage(i, content)
	}
}

func BenchmarkGRPCCallLocal(b *testing.B) {
	h := &BaseHandler{}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Post("/call", func(c *fiber.Ctx) error {
		return h.GRPCCall(c, &benchmarkRequest{}, func(context.Context, *benchmarkRequest) (*benchmarkResponse, error) {
			return &benchmarkResponse{CommonResp: &CommonResp{Code: SuccessCode, Msg: SuccessDesc}, Payload: "ok"}, nil
		})
	})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := app.Test(httptest.NewRequest("POST", "/call", nil))
		if err != nil {
			b.Fatal(err)
		}
		resp.Body.Close()
	}
}

func BenchmarkResponseDecoratorBytesParallel(b *testing.B) {
	h := &BaseHandler{}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = h.responseDecoratorBytes(benchmarkResponseJSON, "trace-id")
		}
	})
}

func BenchmarkGRPCCallLocalParallel(b *testing.B) {
	h := &BaseHandler{}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Post("/call", func(c *fiber.Ctx) error {
		return h.GRPCCall(c, &benchmarkRequest{}, func(context.Context, *benchmarkRequest) (*benchmarkResponse, error) {
			return &benchmarkResponse{CommonResp: &CommonResp{Code: SuccessCode, Msg: SuccessDesc}, Payload: "ok"}, nil
		})
	})
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := app.Test(httptest.NewRequest("POST", "/call", nil))
			if err != nil {
				b.Error(err)
				continue
			}
			_ = resp.Body.Close()
		}
	})
}
