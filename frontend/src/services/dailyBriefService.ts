import axios from "axios";

import { api } from "@/services/api";
import type {
  DailyBriefIssueResponse,
  DailyBriefSubscription,
  DailyBriefTopicCatalogResponse,
  UpdateDailyBriefSubscriptionInput
} from "@/types/dailyBrief";

const FALLBACK_TIMEZONE = "UTC";
const FALLBACK_DELIVERY_TIME = "08:00";

function getLocalTimezone() {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || FALLBACK_TIMEZONE;
}

function buildEmptyIssue(date: string): DailyBriefIssueResponse {
  return {
    pageState: "empty",
    issue: null,
    briefDate: date,
    lastGeneratedAt: null
  };
}

function buildDefaultSubscription(): DailyBriefSubscription {
  return {
    enabled: false,
    timezone: getLocalTimezone(),
    deliveryTimeLocal: FALLBACK_DELIVERY_TIME,
    topics: []
  };
}

export async function getTodayDailyBrief(): Promise<DailyBriefIssueResponse> {
  try {
    return await api.get<DailyBriefIssueResponse>("/daily-brief/today");
  } catch (error) {
    if (axios.isAxiosError(error) && error.response?.status === 404) {
      return buildEmptyIssue(new Date().toISOString().slice(0, 10));
    }
    throw error;
  }
}

export async function getDailyBriefByDate(date: string): Promise<DailyBriefIssueResponse> {
  try {
    return await api.get<DailyBriefIssueResponse>(`/daily-brief/issues?date=${encodeURIComponent(date)}`);
  } catch (error) {
    if (axios.isAxiosError(error) && error.response?.status === 404) {
      return buildEmptyIssue(date);
    }
    throw error;
  }
}

export async function getDailyBriefTopicCatalog(): Promise<DailyBriefTopicCatalogResponse> {
  return api.get<DailyBriefTopicCatalogResponse>("/daily-brief/topic-catalog");
}

export async function getDailyBriefSubscription(): Promise<DailyBriefSubscription> {
  try {
    return await api.get<DailyBriefSubscription>("/daily-brief/subscription");
  } catch (error) {
    if (axios.isAxiosError(error) && error.response?.status === 404) {
      return buildDefaultSubscription();
    }
    throw error;
  }
}

export async function updateDailyBriefSubscription(
  input: UpdateDailyBriefSubscriptionInput
): Promise<DailyBriefSubscription> {
  return api.put<DailyBriefSubscription>("/daily-brief/subscription", input);
}
