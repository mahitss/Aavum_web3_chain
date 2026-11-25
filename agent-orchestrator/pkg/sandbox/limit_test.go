package sandbox

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestSixthRequestFromSameIPIsRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := NewLimiter(5, time.Hour)
	router := gin.New()
	router.POST("/api/jobs", Gate(true, limiter), func(c *gin.Context) {
		c.Status(http.StatusAccepted)
	})

	for n := 1; n <= 5; n++ {
		response := post(router)
		if response.Code != http.StatusAccepted {
			t.Fatalf("request %d status %d", n, response.Code)
		}
	}
	blocked := post(router)
	if blocked.Code != http.StatusTooManyRequests {
		t.Fatalf("sixth status %d", blocked.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(blocked.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["remainingIpQuota"].(float64) != 0 {
		t.Fatalf("quota %#v", body["remainingIpQuota"])
	}
	if limiter.Remaining("203.0.113.8") != 0 {
		t.Fatal("expected an empty remaining quota")
	}
}

func TestDisabledGateDoesNotCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := NewLimiter(1, time.Hour)
	router := gin.New()
	router.POST("/api/jobs", Gate(false, limiter), func(c *gin.Context) {
		c.Status(http.StatusAccepted)
	})
	if post(router).Code != http.StatusAccepted || post(router).Code != http.StatusAccepted {
		t.Fatal("disabled sandbox should not rate limit")
	}
}

func post(router *gin.Engine) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/jobs", nil)
	request.Header.Set("X-Forwarded-For", "203.0.113.8")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
