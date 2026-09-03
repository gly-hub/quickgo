package http

import (
	"net/http/httptest"
	"testing"

	qmetrics "github.com/gly-hub/quickgo/metrics"
	"github.com/gofiber/fiber/v2"
)

func BenchmarkLocalHTTP(b *testing.B) {
	b.Run("baseline", func(b *testing.B) {
		app := benchmarkHTTPApp()
		benchmarkHTTPRequests(b, app)
	})
	b.Run("trace", func(b *testing.B) {
		app := benchmarkHTTPApp(TraceMiddleware())
		benchmarkHTTPRequests(b, app)
	})
	b.Run("metrics", func(b *testing.B) {
		app := benchmarkHTTPApp(qmetrics.FiberMiddleware(qmetrics.New(qmetrics.Config{Namespace: "http_benchmark"})))
		benchmarkHTTPRequests(b, app)
	})
	b.Run("trace_metrics", func(b *testing.B) {
		app := benchmarkHTTPApp(
			TraceMiddleware(),
			qmetrics.FiberMiddleware(qmetrics.New(qmetrics.Config{Namespace: "http_trace_benchmark"})),
		)
		benchmarkHTTPRequests(b, app)
	})
}

func benchmarkHTTPApp(middlewares ...fiber.Handler) *fiber.App {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	for _, middleware := range middlewares {
		app.Use(middleware)
	}
	app.Get("/users/:id", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})
	return app
}

func benchmarkHTTPRequests(b *testing.B, app *fiber.App) {
	b.Helper()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resp, err := app.Test(httptest.NewRequest("GET", "/users/42", nil))
		if err != nil {
			b.Fatal(err)
		}
		resp.Body.Close()
	}
}
