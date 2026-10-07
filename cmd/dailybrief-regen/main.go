package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
	dailybriefservice "local/rag-project/internal/app/dailybrief/service"
	dailybriefbootstrap "local/rag-project/internal/bootstrap/dailybrief"
	ragbootstrap "local/rag-project/internal/bootstrap/rag"
	"local/rag-project/internal/framework/config"
	fwlog "local/rag-project/internal/framework/log"
	infraai "local/rag-project/internal/infra-ai"
)

func main() {
	userID := flag.String("user-id", "1", "subscription user id")
	briefDate := flag.String("date", "", "brief date YYYY-MM-DD (default: today in user timezone)")
	flag.Parse()

	if err := fwlog.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "init log failed: %v\n", err)
		os.Exit(1)
	}
	if err := config.LoadConfig(""); err != nil {
		fmt.Fprintf(os.Stderr, "load config failed: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	aiRuntime := infraai.NewRuntime()
	ragRuntime, err := ragbootstrap.NewRuntime(ctx, ragbootstrap.RuntimeOptions{
		Config:    config.Get(),
		AIRuntime: aiRuntime,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap rag runtime: %v\n", err)
		os.Exit(1)
	}
	defer ragRuntime.Close()

	runtime, err := dailybriefbootstrap.NewRuntime(ctx, dailybriefbootstrap.RuntimeOptions{
		Config:      config.Get(),
		DB:          ragRuntime.DB,
		TaskRuntime: ragRuntime.TaskRuntime,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap daily brief: %v\n", err)
		os.Exit(1)
	}
	defer runtime.Close()

	if runtime.Orchestrator == nil {
		fmt.Fprintln(os.Stderr, "generation orchestrator is not configured (check LLM API keys)")
		os.Exit(1)
	}

	subscription, err := runtime.SubscriptionService.GetByUserID(ctx, *userID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get subscription: %v\n", err)
		os.Exit(1)
	}
	if !subscription.Enabled || len(subscription.Topics) == 0 {
		fmt.Fprintf(os.Stderr, "subscription for user %q is disabled or has no topics", *userID)
		os.Exit(1)
	}

	date := strings.TrimSpace(*briefDate)
	if date == "" {
		date, err = domain.ResolveBriefDate(time.Now(), subscription.Timezone)
		if err != nil {
			fmt.Fprintf(os.Stderr, "resolve brief date: %v\n", err)
			os.Exit(1)
		}
	}

	if runtime.DB != nil {
		if err := clearIssue(ctx, runtime, *userID, date); err != nil {
			fmt.Fprintf(os.Stderr, "clear existing issue: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Printf("regenerating daily brief user=%s date=%s topics=%d sources=%d\n",
		*userID, date, len(subscription.Topics), len(subscription.Sources))

	outcome, err := runtime.Orchestrator.Run(ctx, dailybriefservice.GenerationRequest{
		Subscription: subscription,
		BriefDate:    date,
		TriggerType:  domain.GenerationRunTriggerTypeScheduled,
		Now:          time.Now(),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "generation failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("done status=%s degraded=%v items=%d headline=%q\n",
		outcome.Issue.Status, outcome.Degraded, outcome.Issue.ItemCount, outcome.Issue.Headline)
}

func clearIssue(ctx context.Context, runtime *dailybriefbootstrap.Runtime, userID, briefDate string) error {
	issue, err := runtime.ReadService.GetByDate(ctx, userID, briefDate)
	if err != nil {
		return err
	}
	if issue.Issue.ID == "" {
		return nil
	}
	result := runtime.DB.WithContext(ctx).Exec(
		`DELETE FROM t_daily_brief_item WHERE issue_id = ?`,
		issue.Issue.ID,
	)
	if result.Error != nil {
		return result.Error
	}
	result = runtime.DB.WithContext(ctx).Exec(
		`DELETE FROM t_daily_brief_issue WHERE id = ?`,
		issue.Issue.ID,
	)
	return result.Error
}
