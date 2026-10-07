package scheduledtask

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	storepkg "local/rag-project/internal/adapter/repository/postgres/scheduledtask"
	scheduledservice "local/rag-project/internal/app/scheduledtask/service"
)

// TestDraftListingRouteStaysUnderConversations guards the restore path against
// being swallowed by the /scheduled-tasks/:taskId detail route.
func TestDraftListingRouteStaysUnderConversations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterRoutes(engine, storepkg.NewStore(nil), scheduledservice.Proposer{})

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/scheduled-tasks/conversations/conv-1/drafts", nil))
	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unreadable response: %d %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Code != http.StatusBadRequest || !strings.Contains(body.Message, "invalid draft query") {
		t.Fatalf("draft listing is not served by the conversation route: %d %s", recorder.Code, recorder.Body.String())
	}
}
