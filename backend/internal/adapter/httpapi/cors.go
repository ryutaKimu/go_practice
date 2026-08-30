package httpapi

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// 管理画面(React)はAPIとオリジンが異なる。
// ローカルでは localhost:5173 と localhost:8080、本番では CloudFront と ALB。
// ブラウザからのアクセスを許可するため、CORSヘッダを返す必要がある。
//
// ワイルドカード（*）は使わない。管理画面は認証トークンを載せて個人情報を扱うため、
// 任意のサイトからAPIを叩けてはならない。許可するオリジンは設定で明示する。
type CORS struct {
	allowedOrigins []string
}

func NewCORS(allowedOrigins []string) *CORS {
	return &CORS{allowedOrigins: allowedOrigins}
}

// リクエストで受け付けるヘッダ。フロントが送るものをすべて列挙する必要がある。
// ここに漏れがあると、該当ヘッダを付けたリクエストがプリフライトで弾かれる。
var allowedHeaders = strings.Join([]string{
	"Authorization",
	"Content-Type",
	"Idempotency-Key", // 注文登録で必須(FR-302)
	"X-Request-ID",    // クライアント側で採番したIDを引き継ぐ場合
}, ", ")

// ブラウザのJSから読み取れるようにするレスポンスヘッダ。
// 既定ではCORS越しに読めるヘッダが限られており、明示しないと隠される。
// X-Request-ID はフロントがエラー表示や障害調査に使うので必須
// （frontend/src/api/client.ts の ApiError.requestId）。
var exposedHeaders = strings.Join([]string{
	"X-Request-ID",
	"Idempotent-Replay", // 注文登録が再送だったかをフロントが判定する(ADR-0003)
}, ", ")

// preflightMaxAge はプリフライト結果をブラウザにキャッシュさせる時間。
// 管理画面は操作のたびにPOSTを投げるので、都度OPTIONSが飛ぶと往復が倍になる。
const preflightMaxAge = 10 * time.Minute

func (c *CORS) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Originが無いのはブラウザ以外からのアクセス（curl、ECサイトのサーバー間通信など）。
		// CORSはブラウザの仕組みなので、何も付けずにそのまま通す。
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}

		if !c.isAllowed(origin) {
			// 許可外のオリジンにはCORSヘッダを付けない。
			// ここで403を返さずリクエスト自体は処理させるのは、
			// 拒否の判断をブラウザに委ねるCORSの設計に沿うため。
			next.ServeHTTP(w, r)
			return
		}

		// リクエストのOriginをそのまま返す。許可リストと照合済みなので安全。
		w.Header().Set("Access-Control-Allow-Origin", origin)
		// オリジンごとに応答が変わるので、キャッシュを混ぜないよう Vary を付ける。
		w.Header().Add("Vary", "Origin")
		w.Header().Set("Access-Control-Expose-Headers", exposedHeaders)

		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
			w.Header().Set("Access-Control-Max-Age", strconv.Itoa(int(preflightMaxAge.Seconds())))
			// プリフライトは本体を持たない。ここで打ち切り、ハンドラまで通さない。
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (c *CORS) isAllowed(origin string) bool {
	return slices.Contains(c.allowedOrigins, origin)
}
