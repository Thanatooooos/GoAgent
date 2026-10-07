import * as React from "react";
import { Menu } from "lucide-react";
import { useLocation } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { useChatStore } from "@/stores/chatStore";

interface HeaderProps {
  onToggleSidebar: () => void;
}

export function Header({ onToggleSidebar }: HeaderProps) {
  const { currentSessionId, sessions } = useChatStore();
  const location = useLocation();
  const currentSession = React.useMemo(
    () => sessions.find((session) => session.id === currentSessionId),
    [sessions, currentSessionId]
  );
  const isBriefPage = location.pathname.startsWith("/brief");
  const isScheduledTasksPage = location.pathname.startsWith("/scheduled-tasks");

  return (
    <header className="chat-header sticky top-0 z-20 bg-white">
      <div className="flex h-14 items-center justify-between px-5 md:px-7">
        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            size="icon"
            onClick={onToggleSidebar}
            aria-label="切换侧边栏"
            className="text-[#6b6b6b] hover:bg-[#f1f1f1] lg:hidden"
          >
            <Menu className="h-5 w-5" />
          </Button>
          <div className="min-w-0">
            <p className="truncate text-sm font-medium text-[#2f2f2f]">
              {isScheduledTasksPage ? "定时任务" : isBriefPage ? "每日简报" : currentSession?.title || "New conversation"}
            </p>
            <p className="text-[11px] text-[#8a8a8a]">
              {isScheduledTasksPage ? "GoAgent" : isBriefPage ? "GoAgent · Daily Brief" : "GoAgent · General model"}
            </p>
          </div>
        </div>
      </div>
    </header>
  );
}
