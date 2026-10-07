import * as React from "react";
import { AlertCircle, CalendarDays, ChevronLeft, ChevronRight, ExternalLink, Loader2, Newspaper } from "lucide-react";
import { toast } from "sonner";

import { MainLayout } from "@/components/layout/MainLayout";
import { TopicCatalogPicker } from "@/components/dailybrief/TopicCatalogPicker";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  getDailyBriefByDate,
  getDailyBriefSubscription,
  getDailyBriefTopicCatalog,
  getTodayDailyBrief,
  updateDailyBriefSubscription
} from "@/services/dailyBriefService";
import { topicBreadcrumbMap } from "@/lib/dailyBriefTopicCatalog";
import type {
  DailyBriefIssueItem,
  DailyBriefIssueResponse,
  DailyBriefPageState,
  DailyBriefSubscription,
  DailyBriefTopicCatalogNode
} from "@/types/dailyBrief";
import { dailyBriefSourceLabel, dailyBriefTopicLabel } from "@/types/dailyBrief";

function toLocalDateInputValue(date: Date): string {
  const year = date.getFullYear();
  const month = `${date.getMonth() + 1}`.padStart(2, "0");
  const day = `${date.getDate()}`.padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function shiftDate(value: string, days: number): string {
  const [year, month, day] = value.split("-").map(Number);
  const base = new Date(year, month - 1, day);
  base.setDate(base.getDate() + days);
  return toLocalDateInputValue(base);
}

function formatDateLabel(value: string): string {
  const [year, month, day] = value.split("-").map(Number);
  if (!year || !month || !day) {
    return value;
  }
  return new Date(year, month - 1, day).toLocaleDateString("zh-CN", {
    year: "numeric",
    month: "long",
    day: "numeric"
  });
}

function formatTimeLabel(value?: string | null): string {
  if (!value) return "尚未生成";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN");
}

function normalizeIssueItems(issue: DailyBriefIssueResponse["issue"]): DailyBriefIssueItem[] {
  if (!issue) return [];
  if (issue.items.length > 0) return issue.items;
  return issue.sections.flatMap((section) => section.items);
}

function statusTone(pageState: DailyBriefPageState): "default" | "secondary" | "destructive" | "outline" {
  if (pageState === "ready") return "default";
  if (pageState === "generating") return "secondary";
  if (pageState === "failed") return "destructive";
  return "outline";
}

function statusLabel(pageState: DailyBriefPageState): string {
  if (pageState === "ready") return "已就绪";
  if (pageState === "generating") return "生成中";
  if (pageState === "failed") return "生成失败";
  return "暂无";
}

export function DailyBriefPage() {
  const today = React.useMemo(() => toLocalDateInputValue(new Date()), []);
  const [selectedDate, setSelectedDate] = React.useState(today);

  const [issueData, setIssueData] = React.useState<DailyBriefIssueResponse | null>(null);
  const [issueLoading, setIssueLoading] = React.useState(false);
  const [issueError, setIssueError] = React.useState<string | null>(null);

  const [subscription, setSubscription] = React.useState<DailyBriefSubscription | null>(null);
  const [subscriptionDraft, setSubscriptionDraft] = React.useState<DailyBriefSubscription | null>(null);
  const [subscriptionLoading, setSubscriptionLoading] = React.useState(false);
  const [subscriptionError, setSubscriptionError] = React.useState<string | null>(null);
  const [savingSubscription, setSavingSubscription] = React.useState(false);
  const [topicCatalog, setTopicCatalog] = React.useState<DailyBriefTopicCatalogNode[]>([]);

  const topicLabels = React.useMemo(() => topicBreadcrumbMap(topicCatalog), [topicCatalog]);

  const loadIssue = React.useCallback(async (date: string) => {
    setIssueLoading(true);
    setIssueError(null);
    try {
      const result = date === today ? await getTodayDailyBrief() : await getDailyBriefByDate(date);
      setIssueData(result);
    } catch (error) {
      console.error(error);
      setIssueError("加载每日简报失败。");
    } finally {
      setIssueLoading(false);
    }
  }, [today]);

  const loadSubscription = React.useCallback(async () => {
    setSubscriptionLoading(true);
    setSubscriptionError(null);
    try {
      const result = await getDailyBriefSubscription();
      setSubscription(result);
      setSubscriptionDraft(result);
    } catch (error) {
      console.error(error);
      setSubscriptionError("加载订阅设置失败。");
    } finally {
      setSubscriptionLoading(false);
    }
  }, []);

  const loadTopicCatalog = React.useCallback(async () => {
    try {
      const result = await getDailyBriefTopicCatalog();
      setTopicCatalog(result.nodes);
    } catch (error) {
      console.error(error);
    }
  }, []);

  React.useEffect(() => {
    void loadTopicCatalog();
  }, [loadTopicCatalog]);

  React.useEffect(() => {
    void loadSubscription();
  }, [loadSubscription]);

  React.useEffect(() => {
    void loadIssue(selectedDate);
  }, [loadIssue, selectedDate]);

  const isTodaySelected = selectedDate === today;

  const onToggleTopic = (key: string, checked: boolean) => {
    setSubscriptionDraft((prev) => {
      if (!prev) return prev;
      const topics = checked ? [...prev.topics, key] : prev.topics.filter((topic) => topic !== key);
      return { ...prev, topics };
    });
  };

  const onSaveSubscription = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!subscriptionDraft) return;
    setSavingSubscription(true);
    try {
      const updated = await updateDailyBriefSubscription({
        enabled: subscriptionDraft.enabled,
        timezone: subscriptionDraft.timezone,
        deliveryTimeLocal: subscriptionDraft.deliveryTimeLocal,
        topics: subscriptionDraft.topics
      });
      setSubscription(updated);
      setSubscriptionDraft(updated);
      toast.success("订阅设置已保存。");
      if (isTodaySelected) {
        await loadIssue(today);
      }
    } catch (error) {
      console.error(error);
      toast.error("保存订阅设置失败。");
    } finally {
      setSavingSubscription(false);
    }
  };

  const pageState = issueData?.pageState ?? "empty";
  const issue = issueData?.issue ?? null;
  const issueSections = issue?.sections ?? [];
  const flattenedItems = normalizeIssueItems(issue);

  return (
    <MainLayout variant="brief">
      <div className="brief-page flex-1 min-h-0 overflow-y-auto">
        <div className="brief-page-inner mx-auto flex w-full max-w-7xl flex-col">
          <Card className="brief-overview">
            <CardHeader className="brief-overview-header">
              <div className="brief-overview-grid">
                <div>
                  <p className="brief-kicker">每日简报</p>
                  <CardTitle className="brief-date-title">
                    <Newspaper className="h-5 w-5" />
                    {formatDateLabel(selectedDate)}
                  </CardTitle>
                  <p className="brief-generated-at">
                    最近生成：{formatTimeLabel(issueData?.lastGeneratedAt)}
                  </p>
                </div>
                <div className="brief-toolbar">
                  <Badge variant={statusTone(pageState)} className="brief-status">{statusLabel(pageState)}</Badge>
                  <Button type="button" variant="outline" size="sm" onClick={() => setSelectedDate(shiftDate(selectedDate, -1))}>
                    <ChevronLeft className="mr-1 h-4 w-4" />
                    上一天
                  </Button>
                  <Button type="button" variant="outline" size="sm" onClick={() => setSelectedDate(today)}>
                    今天
                  </Button>
                  <Button type="button" variant="outline" size="sm" onClick={() => setSelectedDate(shiftDate(selectedDate, 1))}>
                    下一天
                    <ChevronRight className="ml-1 h-4 w-4" />
                  </Button>
                  <div className="relative">
                    <CalendarDays className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-400" />
                    <Input
                      type="date"
                      className="brief-date-input pl-9"
                      value={selectedDate}
                      onChange={(event) => setSelectedDate(event.target.value)}
                    />
                  </div>
                </div>
              </div>
            </CardHeader>
          </Card>

          <div className="brief-layout">
            <section className="brief-reading-column">
              {issueLoading ? (
                <Card className="brief-state-card">
                  <CardContent className="brief-state-content">
                    <Loader2 className="mx-auto mb-3 h-5 w-5 animate-spin" />
                    正在加载简报...
                  </CardContent>
                </Card>
              ) : issueError ? (
                <Card className="brief-state-card brief-state-card-error">
                  <CardContent className="brief-state-content">
                    <AlertCircle className="mx-auto mb-3 h-5 w-5" />
                    {issueError}
                    <div className="mt-4">
                      <Button type="button" variant="outline" onClick={() => void loadIssue(selectedDate)}>
                        重试
                      </Button>
                    </div>
                  </CardContent>
                </Card>
              ) : pageState === "ready" && issue ? (
                <>
                  <Card className="brief-summary-card">
                    <CardHeader className="brief-content-header">
                      <CardTitle className="brief-headline">{issue.headline}</CardTitle>
                    </CardHeader>
                    <CardContent className="brief-content-body">
                      <p className="brief-lead">{issue.topSummary}</p>
                    </CardContent>
                  </Card>

                  {(issueSections.length > 0 ? issueSections : [{ key: "all", title: "要闻", items: flattenedItems }]).map(
                    (section) => (
                      <Card key={section.key} className="brief-section-card">
                        <CardHeader className="brief-section-header">
                          <CardTitle className="brief-section-title">{section.title}</CardTitle>
                        </CardHeader>
                        <CardContent className="brief-section-content">
                          {section.items.length === 0 ? (
                            <p className="text-sm text-slate-500">该栏目暂无内容。</p>
                          ) : (
                            section.items.map((item, index) => (
                              <article key={`${section.key}-${index}-${item.url}`} className="brief-article">
                                <div className="brief-article-heading">
                                  <h3 className="brief-article-title">{item.title}</h3>
                                  <div className="flex flex-wrap items-center gap-2">
                                    <Badge variant="outline" className="brief-meta-badge">
                                      {dailyBriefSourceLabel(item.source)}
                                    </Badge>
                                    <Badge variant="secondary" className="brief-meta-badge">
                                      {dailyBriefTopicLabel(item.topic, topicLabels)}
                                    </Badge>
                                  </div>
                                </div>
                                <p className="brief-article-summary">{item.summary}</p>
                                <p className="brief-article-why">
                                  <span className="font-semibold text-slate-700">为何重要：</span> {item.whyItMatters}
                                </p>
                                <a
                                  href={item.url}
                                  target="_blank"
                                  rel="noreferrer"
                                  className="brief-source-link"
                                >
                                  查看原文
                                  <ExternalLink className="h-3.5 w-3.5" />
                                </a>
                              </article>
                            ))
                          )}
                        </CardContent>
                      </Card>
                    )
                  )}
                </>
              ) : (
                <Card className="brief-state-card">
                  <CardContent className="brief-state-content">
                    {pageState === "generating" ? (
                      <>
                        <Loader2 className="mx-auto mb-3 h-5 w-5 animate-spin" />
                        <p className="font-medium text-slate-900">今日简报正在生成</p>
                        <p className="mt-1 text-sm text-slate-500">请稍后再来查看。</p>
                      </>
                    ) : pageState === "failed" ? (
                      <>
                        <AlertCircle className="mx-auto mb-3 h-5 w-5" />
                        <p className="font-medium text-slate-900">该日期简报生成失败</p>
                        <p className="mt-1 text-sm text-slate-500">系统可能会自动重试。</p>
                      </>
                    ) : (
                      <>
                        <Newspaper className="mx-auto mb-3 h-5 w-5 text-slate-400" />
                        <p className="font-medium text-slate-900">该日期暂无简报</p>
                        <p className="mt-1 text-sm text-slate-500">请开启订阅并选择主题。</p>
                      </>
                    )}
                  </CardContent>
                </Card>
              )}
            </section>

            <aside>
              <Card className="brief-settings-card">
                <CardHeader className="brief-settings-header">
                  <CardTitle className="brief-settings-title">订阅设置</CardTitle>
                </CardHeader>
                <CardContent className="brief-settings-content">
                  {subscriptionLoading ? (
                    <div className="flex items-center gap-2 text-sm text-slate-500">
                      <Loader2 className="h-4 w-4 animate-spin" />
                      正在加载订阅设置...
                    </div>
                  ) : subscriptionError ? (
                    <div className="text-sm text-red-600">{subscriptionError}</div>
                  ) : subscriptionDraft ? (
                    <form className="brief-settings-form" onSubmit={onSaveSubscription}>
                      <div className="brief-toggle-row">
                        <Checkbox
                          id="brief-enabled"
                          checked={subscriptionDraft.enabled}
                          onCheckedChange={(checked) => {
                            setSubscriptionDraft((prev) => (prev ? { ...prev, enabled: checked === true } : prev));
                          }}
                        />
                        <Label htmlFor="brief-enabled">开启每日简报</Label>
                      </div>

                      <div className="brief-field">
                        <Label htmlFor="brief-timezone">时区</Label>
                        <Input
                          id="brief-timezone"
                          value={subscriptionDraft.timezone}
                          onChange={(event) => {
                            const value = event.target.value;
                            setSubscriptionDraft((prev) => (prev ? { ...prev, timezone: value } : prev));
                          }}
                          placeholder="例如 Asia/Shanghai"
                        />
                      </div>

                      <div className="brief-field">
                        <Label htmlFor="brief-delivery-time">推送时间</Label>
                        <Input
                          id="brief-delivery-time"
                          type="time"
                          value={subscriptionDraft.deliveryTimeLocal}
                          onChange={(event) => {
                            const value = event.target.value;
                            setSubscriptionDraft((prev) => (prev ? { ...prev, deliveryTimeLocal: value } : prev));
                          }}
                        />
                      </div>

                      <TopicCatalogPicker
                        catalog={topicCatalog}
                        selectedTopics={subscriptionDraft.topics}
                        onToggleTopic={onToggleTopic}
                      />

                      <Button
                        type="submit"
                        className="brief-save-button w-full"
                        disabled={savingSubscription || !subscriptionDraft.timezone || !subscriptionDraft.deliveryTimeLocal}
                      >
                        {savingSubscription ? (
                          <>
                            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                            保存中...
                          </>
                        ) : (
                          "保存设置"
                        )}
                      </Button>

                      {subscription ? (
                        <p className="text-xs text-slate-500">
                          当前状态：{subscription.enabled ? "已开启" : "已关闭"}
                        </p>
                      ) : null}
                    </form>
                  ) : null}
                </CardContent>
              </Card>
            </aside>
          </div>
        </div>
      </div>
    </MainLayout>
  );
}
