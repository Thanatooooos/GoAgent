import { ExternalLink, ShieldAlert } from "lucide-react";

import type { ApprovalPendingPayload } from "@/types";

interface ApprovalPendingCardProps {
  approval: ApprovalPendingPayload;
}

function riskLabel(riskLevel?: string) {
  const normalized = riskLevel?.trim().toLowerCase();
  if (!normalized) return null;
  switch (normalized) {
    case "high":
      return "高风险";
    case "medium":
      return "中风险";
    case "low":
      return "低风险";
    default:
      return riskLevel;
  }
}

export function ApprovalPendingCard({ approval }: ApprovalPendingCardProps) {
  const summary =
    approval.reasonMessage?.trim() ||
    approval.reason?.trim() ||
    "继续执行前需要你的确认。";
  const urls = (approval.candidateUrls ?? []).filter((url) => url.trim().length > 0);
  const risk = riskLabel(approval.riskLevel);

  return (
    <div className="overflow-hidden rounded-lg border border-orange-200 bg-orange-50">
      <div className="flex items-center gap-2 px-4 py-3">
        <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-orange-100">
          <ShieldAlert className="h-4 w-4 text-orange-700" />
        </div>
        <span className="text-sm font-medium text-orange-900">需要确认后继续</span>
        {risk ? (
          <span className="rounded-full bg-orange-100 px-2 py-0.5 text-xs text-orange-700">
            {risk}
          </span>
        ) : null}
      </div>

      <div className="border-t border-orange-200 px-4 pb-4 pt-3 text-sm leading-6 text-orange-950">
        <p>{summary}</p>

        {approval.question?.trim() ? (
          <p className="mt-3 text-xs leading-5 text-orange-800">
            原始问题：{approval.question.trim()}
          </p>
        ) : null}

        {approval.currentStepTitle?.trim() ? (
          <p className="mt-2 text-xs leading-5 text-orange-800">
            当前步骤：{approval.currentStepTitle.trim()}
          </p>
        ) : null}

        {approval.searchQuery?.trim() ? (
          <p className="mt-2 text-xs leading-5 text-orange-800">
            计划查询：{approval.searchQuery.trim()}
          </p>
        ) : null}

        {urls.length ? (
          <div className="mt-3 space-y-2">
            {urls.map((url) => (
              <a
                key={url}
                href={url}
                target="_blank"
                rel="noreferrer"
                className="flex items-center gap-1 break-all text-xs text-orange-700 underline underline-offset-2 hover:text-orange-900"
              >
                <ExternalLink className="h-3.5 w-3.5 flex-shrink-0" />
                <span>{url}</span>
              </a>
            ))}
          </div>
        ) : null}
      </div>
    </div>
  );
}
