package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	briefrepo "local/rag-project/internal/adapter/repository/postgres/dailybrief"
	storepkg "local/rag-project/internal/adapter/repository/postgres/scheduledtask"
	userrepo "local/rag-project/internal/adapter/repository/postgres/user"
	briefdomain "local/rag-project/internal/app/dailybrief/domain"
	briefservice "local/rag-project/internal/app/dailybrief/service"
	taskdomain "local/rag-project/internal/app/scheduledtask/domain"
	userservice "local/rag-project/internal/app/user/service"
	"local/rag-project/internal/framework/config"
)

// Opt-in acceptance starts the real server, uses the configured external model
// and tools, and waits for a real daily tick. It never substitutes TaskRuntime.
// A dedicated empty codex_dailybrief_ database and normal model/storage config
// are required; the ordinary suite does not spend tokens or start processes.
func TestDailyBriefMigrationLiveServer(t *testing.T) {
	if os.Getenv("DAILYBRIEF_LIVE_ACCEPTANCE") != "1" {
		t.Skip("set DAILYBRIEF_LIVE_ACCEPTANCE=1 and DAILYBRIEF_LIVE_ACCEPTANCE_DSN for real-model acceptance")
	}
	dsn := os.Getenv("DAILYBRIEF_LIVE_ACCEPTANCE_DSN")
	connection, err := pgx.ParseConfig(dsn)
	if dsn == "" || err != nil || !strings.HasPrefix(connection.Database, "codex_dailybrief_") {
		t.Fatal("a dedicated codex_dailybrief_ DSN is required")
	}
	if connection.Host != "127.0.0.1" && connection.Host != "localhost" && connection.Host != "::1" {
		t.Fatal("live acceptance only starts services against a local dedicated database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("cannot connect to the acceptance database")
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	var database string
	if err := db.Raw(`SELECT current_database()`).Scan(&database).Error; err != nil || database != connection.Database {
		t.Fatal("acceptance database identity mismatch")
	}
	if err := postgresrepo.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	var existing int64
	if err := db.Raw(`SELECT (SELECT count(*) FROM t_daily_brief_subscription) +
		(SELECT count(*) FROM t_scheduled_task WHERE deleted_at IS NULL)`).Scan(&existing).Error; err != nil || existing != 0 {
		t.Fatal("use a fresh dedicated database; acceptance never clears existing subscriptions/tasks")
	}
	ctx := context.Background()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if err := config.LoadConfig(filepath.Join(root, "configs")); err != nil {
		t.Fatal(err)
	}
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	directory, err := os.MkdirTemp(filepath.Join(root, "tmp"), "dailybrief-live-")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("acceptance artifacts: %s", directory)
	serverBinary := filepath.Join(directory, "server"+suffix)
	cutoverBinary := filepath.Join(directory, "cutover"+suffix)
	for _, build := range []struct{ binary, pkg string }{{serverBinary, "./cmd/server"}, {cutoverBinary, "./cmd/dailybrief-cutover"}} {
		command := exec.Command("go", "build", "-o", build.binary, build.pkg)
		command.Dir = root
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", build.pkg, err, output)
		}
	}
	configDirectory := filepath.Join(directory, "configs")
	if err := os.Mkdir(configDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	yaml, err := os.ReadFile(filepath.Join(root, "configs", "application.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDirectory, "application.yaml"), yaml, 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	// Explicit child environment wins over any .env defaults. Credentials stay
	// in the environment, never command arguments, test output, or artifacts.
	t.Setenv("SPRING_DATASOURCE_URL", fmt.Sprintf("jdbc:postgresql://%s/%s?sslmode=%s", net.JoinHostPort(connection.Host, fmt.Sprint(connection.Port)), url.PathEscape(database), "disable"))
	t.Setenv("SPRING_DATASOURCE_USERNAME", connection.User)
	t.Setenv("SPRING_DATASOURCE_PASSWORD", connection.Password)
	t.Setenv("APP_EXPECTED_DATABASE", database)
	t.Setenv("APP_DEMO_MODE", "false")
	t.Setenv("APP_DISABLE_SCHEDULED_JOBS", "false")
	t.Setenv("SERVER_PORT", fmt.Sprint(port))
	client := &http.Client{Timeout: 15 * time.Second}
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	token := ""
	request := func(method, path string, input, output any) {
		t.Helper()
		var body io.Reader
		if input != nil {
			data, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			body = bytes.NewReader(data)
		}
		req, err := http.NewRequest(method, base+"/api/ragent"+path, body)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		response, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer response.Body.Close()
		var envelope struct {
			Code string          `json:"code"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil || response.StatusCode != http.StatusOK || envelope.Code != "0" {
			t.Fatalf("%s %s: HTTP %d code=%s", method, path, response.StatusCode, envelope.Code)
		}
		if output != nil {
			if err := json.Unmarshal(envelope.Data, output); err != nil {
				t.Fatal(err)
			}
		}
	}
	start := func(mode string) func() {
		t.Helper()
		t.Setenv("APP_DAILY_BRIEF_SCHEDULER", mode)
		logFile, err := os.Create(filepath.Join(directory, "server-"+mode+"-"+fmt.Sprint(time.Now().UnixNano())+".log"))
		if err != nil {
			t.Fatal(err)
		}
		command := exec.Command(serverBinary)
		command.Dir, command.Stdout, command.Stderr = directory, logFile, logFile
		if err := command.Start(); err != nil {
			logFile.Close()
			t.Fatal(err)
		}
		stopped := false
		stop := func() {
			if !stopped {
				stopped = true
				command.Process.Kill() // Only this test's owned child process.
				command.Wait()
				logFile.Close()
			}
		}
		t.Cleanup(stop)
		deadline := time.Now().Add(45 * time.Second)
		for time.Now().Before(deadline) {
			response, err := client.Get(base + "/ping")
			if err == nil {
				response.Body.Close()
				if response.StatusCode == http.StatusOK {
					return stop
				}
			}
			time.Sleep(250 * time.Millisecond)
		}
		t.Fatal("owned server did not become ready; inspect its local log")
		return stop
	}
	users := userservice.NewUserService(userrepo.NewUserRepository(db), userrepo.NewUserSessionRepository(db))
	username := "brief-live-" + fmt.Sprint(time.Now().UnixMilli())
	// Only this guarded, isolated fixture uses the public test password, so a
	// follow-up browser check can log in without exporting a session token.
	password := "Dailybrief-Live1!"
	user, err := users.Create(ctx, userservice.CreateUserInput{Username: username, Password: password, Role: "user", OperatorID: "dailybrief-live-test"})
	if err != nil {
		t.Fatal(err)
	}
	var taskID string
	t.Cleanup(func() {
		if taskID != "" {
			if err := storepkg.NewStore(db).SetStatus(ctx, user.ID, taskID, taskdomain.TaskPaused, time.Time{}); err != nil {
				t.Error(err)
			}
		} else {
			db.Exec(`UPDATE t_daily_brief_subscription SET enabled=0 WHERE user_id=?`, user.ID)
		}
	})
	stop := start("mixed")
	t.Log("mixed server ready; creating an unmigrated subscription through HTTP")
	var login struct {
		Token string `json:"token"`
	}
	request("POST", "/auth/login", map[string]string{"username": username, "password": password}, &login)
	token = login.Token
	if token == "" {
		t.Fatal("login returned no token")
	}
	due := time.Now().UTC().Add(2 * time.Minute).Truncate(time.Minute)
	request("PUT", "/daily-brief/subscription", map[string]any{"enabled": true, "timezone": "UTC", "deliveryTimeLocal": due.Format("15:04"), "topics": []string{briefdomain.TopicKeyTechDev}}, nil)
	// A historical issue is deliberately synthetic; the upcoming report must
	// come from the external model. This tests preservation without pretending
	// an old fixture is real-model evidence.
	date := due.AddDate(0, 0, -1).Format("2006-01-02")
	issue := briefdomain.NewIssue(user.ID, user.ID, date)
	if _, err := briefrepo.NewIssueRepository(db).Create(ctx, issue); err != nil {
		t.Fatal(err)
	}
	artifact := briefdomain.BriefArtifact{Headline: "历史验收样本", TopSummary: "合成历史数据", Sections: []briefdomain.BriefSection{{Key: briefdomain.TopicKeyTechDev, Title: "开发", Items: []briefdomain.BriefItemDraft{{Title: "历史条目", Summary: "历史摘要", WhyItMatters: "验证保留", URL: "https://example.org/history", Source: "历史样本", Topic: briefdomain.TopicKeyTechDev}}}}}
	if _, _, err := briefservice.NewPublisher(briefrepo.NewPublishTransaction(db)).Publish(ctx, briefservice.PublishInput{Issue: issue, Artifact: artifact, PublishedRunID: user.ID, PublishedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	stop()
	// The real executable must refuse scheduled-only mode while this enabled
	// subscription is still unbound. A nonzero exit alone is insufficient: the
	// failure must be the cutover guard rather than an unrelated boot error.
	t.Setenv("APP_DAILY_BRIEF_SCHEDULER", "scheduled")
	guardCtx, cancelGuard := context.WithTimeout(ctx, 45*time.Second)
	guard := exec.CommandContext(guardCtx, serverBinary)
	guard.Dir = directory
	guardOutput, guardErr := guard.CombinedOutput()
	cancelGuard()
	if guardErr == nil || !bytes.Contains(guardOutput, []byte("cannot stop legacy DailyBrief scheduler")) {
		t.Fatal("formal server did not reject an unmigrated subscription at the cutover guard")
	}
	t.Log("formal scheduled-only startup guard passed")
	for i := 0; i < 2; i++ {
		command := exec.Command(cutoverBinary, "-database", database, "-user", user.ID, "-apply")
		command.Dir = directory
		if err := command.Run(); err != nil {
			t.Fatalf("formal cutover CLI failed: %v", err)
		}
	}
	if err := db.Raw(`SELECT task_id FROM t_daily_brief_task_binding WHERE user_id=?`, user.ID).Scan(&taskID).Error; err != nil || taskID == "" {
		t.Fatal("CLI did not bind the subscription")
	}
	task, _, err := storepkg.NewStore(db).Get(ctx, user.ID, taskID)
	if err != nil || task.ConversationID != "" || !task.NextDueAt.Equal(due) {
		t.Fatal("cutover created an empty conversation or changed the due time")
	}
	stop = start("scheduled")
	t.Logf("scheduled server ready; waiting for actual daily tick at %s", due.Format(time.RFC3339))
	var page struct {
		PageState string                     `json:"pageState"`
		Issue     *briefdomain.BriefArtifact `json:"issue"`
	}
	deadline := due.Add(8 * time.Minute)
	for {
		request("GET", "/daily-brief/today", nil, &page)
		if page.PageState == "ready" && page.Issue != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("real daily tick did not publish a report; pageState=%s", page.PageState)
		}
		time.Sleep(2 * time.Second)
	}
	var report *storepkg.RunDetail
	request("GET", "/scheduled-tasks/"+taskID+"/latest-report", nil, &report)
	if report == nil || report.Run.Status != "reported" || len(report.Attempts) == 0 {
		t.Fatal("issue has no corresponding published task report")
	}
	evidence, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "published-report.json"), evidence, 0600); err != nil {
		t.Fatal(err)
	}
	toolSuccess := false
	for _, attempt := range report.Attempts {
		for _, tool := range attempt.Tools {
			if (tool.ToolName == "web_fetch" || tool.ToolName == "web_search") && tool.ToolState == "completed" {
				toolSuccess = true
			}
		}
	}
	if !toolSuccess {
		t.Fatal("report has no successful real web tool event; it is not research acceptance evidence")
	}
	var published struct{ Run, Message, Content string }
	if err := db.Raw(`SELECT i.published_run_id AS run, o.published_message_id AS message, m.content
		FROM t_daily_brief_issue i JOIN t_scheduled_task_occurrence o ON o.id=i.published_run_id
		JOIN t_message m ON m.id=o.published_message_id WHERE i.user_id=? AND i.brief_date=?`, user.ID, due.Format("2006-01-02")).Scan(&published).Error; err != nil {
		t.Fatal(err)
	}
	if published.Run != report.Run.ID || published.Message == "" || !strings.Contains(published.Content, page.Issue.Headline) {
		t.Fatal("issue and conversation were not projected from the same occurrence")
	}
	for _, section := range page.Issue.Sections {
		for _, item := range section.Items {
			if !strings.Contains(published.Content, item.URL) || !strings.Contains(published.Content, item.Summary) {
				t.Fatal("issue items differ from the conversation report")
			}
		}
	}
	var unread []storepkg.UnreadView
	request("GET", "/scheduled-tasks/unread", nil, &unread)
	if len(unread) != 1 || unread[0].UnreadCount != 1 {
		t.Fatal("background publication did not create exactly one unread report")
	}
	request("GET", "/daily-brief/issues?date="+date, nil, &page)
	if page.Issue == nil || page.Issue.Headline != artifact.Headline {
		t.Fatal("cutover hid or changed the legacy issue")
	}
	stop()
	stop = start("scheduled")
	time.Sleep(2 * time.Duration(config.Get().ScheduledTask.ScanIntervalSeconds) * time.Second)
	var count int64
	if err := db.Raw(`SELECT count(*) FROM t_message WHERE user_id=?`, user.ID).Scan(&count).Error; err != nil || count != 1 {
		t.Fatal("server restart duplicated the report")
	}
	request("POST", "/scheduled-tasks/conversations/"+unread[0].ConversationID+"/read", nil, nil)
	request("GET", "/scheduled-tasks/unread", nil, &unread)
	if len(unread) != 0 {
		t.Fatal("reading the report did not clear unread state")
	}
	request("PUT", "/daily-brief/subscription", map[string]any{"enabled": false, "timezone": "UTC", "deliveryTimeLocal": due.Format("15:04"), "topics": []string{briefdomain.TopicKeyTechDev}}, nil)
	task, _, err = storepkg.NewStore(db).Get(ctx, user.ID, taskID)
	if err != nil || string(task.Status) != "paused" {
		t.Fatal("HTTP unsubscribe did not pause the migrated task")
	}
	var legacyRuns int64
	if err := db.Raw(`SELECT count(*) FROM t_daily_brief_generation_run WHERE user_id=?`, user.ID).Scan(&legacyRuns).Error; err != nil || legacyRuns != 0 {
		t.Fatal("legacy generation still ran in scheduled-only acceptance")
	}
	t.Logf("database=%s task=%s occurrence=%s: real-server daily tick, web tools, dual publication, history, restart dedup, unread and unsubscribe passed", database, taskID, report.Run.ID)
}
