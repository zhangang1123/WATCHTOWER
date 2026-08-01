package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestAlertStormReportsDeduplication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(zap.NewNop(), "", nil)
	defer handler.Close()
	router := gin.New()
	handler.RegisterRoutes(router)
	body := `{"count":100,"duplicate_ratio":0.9,"service":"storm-test"}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/mock/storm", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", recorder.Code, recorder.Body.String())
	}
	var result struct {
		Requested    int `json:"requested"`
		Accepted     int `json:"accepted"`
		Deduplicated int `json:"deduplicated"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Requested != 100 || result.Accepted == 0 || result.Deduplicated == 0 || result.Accepted+result.Deduplicated != 100 {
		t.Fatalf("unexpected storm result: %#v", result)
	}
}
