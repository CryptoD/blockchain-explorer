package apiutil

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestErrorEnvelopeJSON_Shape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("correlation_id", "corr-xyz")

	env := ErrorEnvelopeJSON(c, "bad_request", "nope")
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"code", "message", "correlation_id", "timestamp"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("missing key %q in %v", key, body)
		}
	}
	if body["code"] != "bad_request" || body["message"] != "nope" || body["correlation_id"] != "corr-xyz" {
		t.Fatalf("unexpected values: %#v", body)
	}
}
