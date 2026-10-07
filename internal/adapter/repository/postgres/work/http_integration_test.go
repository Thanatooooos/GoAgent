package work_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	workhttp "local/rag-project/internal/adapter/http/work"
	"local/rag-project/internal/app/work/domain"
	workbootstrap "local/rag-project/internal/bootstrap/work"
	"local/rag-project/internal/framework/contextx"
)

func TestProtectedManualHTTPWithRealPersistence(t *testing.T) {
	db := testDB(t)
	runtime, err := workbootstrap.NewRuntime(db)
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	// Test-only identity injection; the production entrypoint uses the existing
	// login loader and RegisterRoutes always enforces RequireLogin.
	engine.Use(func(c *gin.Context) {
		if id := c.GetHeader("X-Test-User"); id != "" {
			contextx.Set(c, &contextx.LoginUser{UserID: id, Role: "user"})
		}
		c.Next()
	})
	workhttp.RegisterRoutes(engine, runtime.Service)
	server := httptest.NewServer(engine)
	defer server.Close()
	request := func(method, path, user string, input any) (int, json.RawMessage) {
		t.Helper()
		var body []byte
		if input != nil {
			body, _ = json.Marshal(input)
		}
		r, err := http.NewRequest(method, server.URL+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Test-User", user)
		response, err := server.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, _ := io.ReadAll(response.Body)
		var envelope struct{ Data json.RawMessage }
		if err := json.Unmarshal(data, &envelope); err != nil {
			t.Fatalf("invalid response: %s", data)
		}
		return response.StatusCode, envelope.Data
	}
	if status, _ := request("GET", "/work/topics", "", nil); status != 401 {
		t.Fatalf("login guard status %d", status)
	}
	user := "http-work-user"
	key := fmt.Sprintf("http-%d", time.Now().UnixNano())
	status, data := request("POST", "/work/topics", user, domain.CreateTopic{Mutation: mutation(key, 0), Name: "HTTP 主题", Description: "可持久化"})
	if status != 200 {
		t.Fatalf("create status %d %s", status, data)
	}
	var topic domain.Topic
	if err := json.Unmarshal(data, &topic); err != nil || topic.ID == "" {
		t.Fatalf("topic: %s %v", data, err)
	}
	if status, _ := request("GET", "/work/topics/"+topic.ID, "other-user", nil); status != 404 {
		t.Fatalf("cross-user status %d", status)
	}
	input := domain.SaveArtifact{Mutation: mutation(key+"-doc", 0), Title: "正文", Body: domain.EmptyDocument()}
	status, data = request("POST", "/work/topics/"+topic.ID+"/artifacts", user, input)
	if status != 200 {
		t.Fatalf("create artifact status %d %s", status, data)
	}
	var doc domain.ArtifactDetail
	json.Unmarshal(data, &doc)
	path := "/work/topics/" + topic.ID + "/artifacts/" + doc.Artifact.ID
	input.Mutation = mutation(key+"-save", 1)
	input.Body.Root.Content[0].Content = []domain.Node{{Type: "text", Text: "用户正文"}}
	if status, _ := request("PUT", path, user, input); status != 200 {
		t.Fatalf("save status %d", status)
	}
	if status, _ := request("PUT", path, user, input); status != 200 {
		t.Fatalf("replay status %d", status)
	}
	input.Mutation = mutation(key+"-conflict", 1)
	if status, data := request("PUT", path, user, input); status != 409 || !bytes.Contains(data, []byte("currentRevision")) {
		t.Fatalf("conflict status %d %s", status, data)
	}
	status, data = request("GET", path, user, nil)
	var persisted domain.ArtifactDetail
	json.Unmarshal(data, &persisted)
	if status != 200 || persisted.Version.Revision != 2 || persisted.Version.Body.Text() != "用户正文" {
		t.Fatalf("persisted artifact: %d %s", status, data)
	}
	if status, _ := request("POST", "/work/topics", user, map[string]any{"requestId": key + "-bad", "name": "禁止额外字段", "unexpected": true}); status != 400 {
		t.Fatalf("unknown field accepted: %d", status)
	}
}
