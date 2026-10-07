import * as React from "react";

import { Header } from "@/components/layout/Header";
import { Sidebar } from "@/components/layout/Sidebar";

interface MainLayoutProps {
  children: React.ReactNode;
  variant?: "default" | "chat" | "brief";
}

export function MainLayout({ children, variant = "default" }: MainLayoutProps) {
  const [sidebarOpen, setSidebarOpen] = React.useState(false);
  const isModernSurface = variant !== "default";
  const surfaceClass = variant === "brief" ? "brief-shell" : "";

  return (
    <div className={`${isModernSurface ? "chat-shell" : ""} ${surfaceClass} flex h-screen min-w-0 bg-white`}>
      <Sidebar isOpen={sidebarOpen} onClose={() => setSidebarOpen(false)} />
      <div className="flex h-full min-h-0 min-w-0 flex-1 flex-col bg-white">
        <Header onToggleSidebar={() => setSidebarOpen((prev) => !prev)} />
        <main className={`${isModernSurface ? "chat-main" : ""} ${variant === "brief" ? "brief-main" : ""} flex min-h-0 flex-1 flex-col overflow-hidden bg-white`}>
          {children}
        </main>
      </div>
    </div>
  );
}
