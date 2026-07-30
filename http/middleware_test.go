package http

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

func TestTimeoutMiddlewareUsesRequestContext(t *testing.T) {
	app := fiber.New()
	app.Use(TimeoutMiddleware(10 * time.Millisecond))
	app.Get("/slow", func(c *fiber.Ctx) error {
		<-c.UserContext().Done()
		return c.UserContext().Err()
	})

	response, err := app.Test(httptest.NewRequest("GET", "/slow", nil), 1000)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if response.StatusCode != fiber.StatusRequestTimeout {
		t.Fatalf("expected status 408, got %d", response.StatusCode)
	}
}
