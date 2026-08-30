import { BrowserRouter, Link, Navigate, Route, Routes } from "react-router-dom";
import { StatusPage } from "./pages/StatusPage";

/**
 * ルーティングの骨格。業務画面は MIN-015 以降で順次追加する
 * （docs/backlog.md Sprint 1）。
 */
export function App() {
  return (
    <BrowserRouter>
      <div className="layout">
        <header className="header">
          <h1>ミナトマート 在庫・注文管理</h1>
          <nav>
            <Link to="/status">稼働状況</Link>
          </nav>
        </header>

        <main className="main">
          <Routes>
            <Route path="/" element={<Navigate to="/status" replace />} />
            <Route path="/status" element={<StatusPage />} />
            <Route path="*" element={<p>ページが見つかりません</p>} />
          </Routes>
        </main>
      </div>
    </BrowserRouter>
  );
}
