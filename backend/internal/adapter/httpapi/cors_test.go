package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const devOrigin = "http://localhost:5173"

func corsServer() http.Handler {
	// MIN-011 usecase実装後に差し替え
	return NewServer(stubDB{}, "test", []string{devOrigin}, nil).Routes()
}

// このテストが無かったために CORS の実装漏れに気づけなかった。
// 管理画面から /healthz を叩けることを、ブラウザと同じ条件で確認する。
func TestCORS_許可オリジンからのリクエストにヘッダが付く(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", devOrigin)

	corsServer().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// これが無いとブラウザがレスポンスの読み取りを拒否し、fetch は "Failed to fetch" になる。
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != devOrigin {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, devOrigin)
	}
	// オリジンごとに応答が変わるため、キャッシュを分ける必要がある。
	if got := rec.Header().Get("Vary"); !strings.Contains(got, "Origin") {
		t.Errorf("Vary = %q, Origin を含むことを期待", got)
	}
}

// frontend/src/api/client.ts が ApiError.requestId を読むために必要。
// Expose-Headers に無いと、CORS越しでは常に null になる。
func TestCORS_フロントが読むヘッダを公開する(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", devOrigin)

	corsServer().ServeHTTP(rec, req)

	exposed := rec.Header().Get("Access-Control-Expose-Headers")
	for _, want := range []string{"X-Request-ID", "Idempotent-Replay"} {
		if !strings.Contains(exposed, want) {
			t.Errorf("Access-Control-Expose-Headers = %q, %s を含むことを期待", exposed, want)
		}
	}
}

func TestCORS_プリフライト(t *testing.T) {
	rec := httptest.NewRecorder()
	// 注文登録は Idempotency-Key と Authorization を付けるためプリフライトが飛ぶ。
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/orders", nil)
	req.Header.Set("Origin", devOrigin)
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Idempotency-Key, Authorization")

	corsServer().ServeHTTP(rec, req)

	// ルーティング未定義のパスでも、プリフライトはCORS層で完結して204を返す。
	// ここで404を返すとブラウザは本リクエストを送らない。
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}

	allowHeaders := rec.Header().Get("Access-Control-Allow-Headers")
	for _, want := range []string{"Authorization", "Content-Type", "Idempotency-Key"} {
		if !strings.Contains(allowHeaders, want) {
			t.Errorf("Access-Control-Allow-Headers = %q, %s を含むことを期待", allowHeaders, want)
		}
	}
	if !strings.Contains(rec.Header().Get("Access-Control-Allow-Methods"), "POST") {
		t.Errorf("Access-Control-Allow-Methods = %q, POST を含むことを期待",
			rec.Header().Get("Access-Control-Allow-Methods"))
	}
	if rec.Header().Get("Access-Control-Max-Age") == "" {
		t.Error("Access-Control-Max-Age が空です（毎回プリフライトが飛ぶ）")
	}
}

func TestCORS_許可外オリジンにはヘッダを付けない(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "https://evil.example")

	corsServer().ServeHTTP(rec, req)

	// ヘッダを付けなければ、ブラウザ側がレスポンスの読み取りを拒否する。
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, 許可外オリジンには付けてはならない", got)
	}
}

// 部分一致で許可してしまう実装ミスを防ぐ。
// "http://localhost:5173.evil.example" のようなオリジンを通してはならない。
func TestCORS_オリジンは完全一致で判定する(t *testing.T) {
	tests := []string{
		"http://localhost:5173.evil.example",
		"http://evil.example/http://localhost:5173",
		"https://localhost:5173", // スキームが違う
		"http://localhost:5174",  // ポートが違う
		"http://localhost",       // ポートなし
	}

	for _, origin := range tests {
		t.Run(origin, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			req.Header.Set("Origin", origin)

			corsServer().ServeHTTP(rec, req)

			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
				t.Errorf("Access-Control-Allow-Origin = %q, 許可してはならない", got)
			}
		})
	}
}

// curl やサーバー間通信（ECサイトからの注文登録）は Origin を送らない。
// CORSはブラウザの仕組みなので、これらは素通しする。
func TestCORS_Originなしのリクエストは素通しする(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	corsServer().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, Originなしでは付けない", got)
	}
}
