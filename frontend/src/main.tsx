import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { App } from "./App";
import "./index.css";

// 管理画面はサーバー状態が主なので、グローバル状態管理は入れず TanStack Query に寄せる
// （docs/05-architecture.md 3章）。
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      // 在庫数は他のオペレーターの操作で変わる。画面に戻ったら取り直す。
      refetchOnWindowFocus: true,
      retry: (failureCount, error) => {
        // 400番台はリトライしても結果が変わらない。無駄な負荷をかけない。
        const status = (error as { status?: number }).status;
        if (status !== undefined && status >= 400 && status < 500) return false;
        return failureCount < 2;
      },
    },
  },
});

const rootElement = document.getElementById("root");
if (!rootElement) {
  throw new Error("#root が見つかりません");
}

createRoot(rootElement).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>
  </StrictMode>,
);
