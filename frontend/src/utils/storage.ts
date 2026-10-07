import type { User } from "@/types";

const TOKEN_KEY = "ragent_token";
const USER_KEY = "ragent_user";
const THEME_KEY = "ragent_theme";
const CHAT_KB_IDS_KEY = "ragent_chat_kb_ids";

function safeGet(key: string) {
  try {
    return window.localStorage.getItem(key);
  } catch {
    return null;
  }
}

function safeSet(key: string, value: string) {
  try {
    window.localStorage.setItem(key, value);
  } catch {
    return;
  }
}

function safeRemove(key: string) {
  try {
    window.localStorage.removeItem(key);
  } catch {
    return;
  }
}

export const storage = {
  getToken(): string | null {
    return safeGet(TOKEN_KEY);
  },
  setToken(token: string) {
    safeSet(TOKEN_KEY, token);
  },
  clearToken() {
    safeRemove(TOKEN_KEY);
  },
  getUser(): User | null {
    const raw = safeGet(USER_KEY);
    if (!raw) return null;
    try {
      return JSON.parse(raw) as User;
    } catch {
      return null;
    }
  },
  setUser(user: User) {
    safeSet(USER_KEY, JSON.stringify(user));
  },
  clearUser() {
    safeRemove(USER_KEY);
  },
  clearAuth() {
    safeRemove(TOKEN_KEY);
    safeRemove(USER_KEY);
  },
  getTheme(): string | null {
    return safeGet(THEME_KEY);
  },
  setTheme(theme: string) {
    safeSet(THEME_KEY, theme);
  },
  getChatKnowledgeBaseIds(): string[] {
    const raw = safeGet(CHAT_KB_IDS_KEY);
    if (!raw) return [];
    try {
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed)) {
        return parsed.filter((id): id is string => typeof id === "string" && id.length > 0);
      }
    } catch {
      return [];
    }
    return [];
  },
  setChatKnowledgeBaseIds(ids: string[]) {
    safeSet(CHAT_KB_IDS_KEY, JSON.stringify(ids.filter(Boolean)));
  }
};
