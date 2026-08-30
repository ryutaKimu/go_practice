# --- ビルド用 ---------------------------------------------------------------
FROM golang:1.26-alpine AS build

WORKDIR /src

# 依存だけ先にコピーしてレイヤキャッシュを効かせる。
# ソース変更のたびに依存を落とし直さないため。
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO を切って静的バイナリにする。scratch イメージで動かすため。
# -trimpath でビルドマシンの絶対パスをバイナリから除去する。
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# --- 開発用（ホットリロード） -----------------------------------------------
FROM golang:1.26-alpine AS dev

WORKDIR /app
RUN go install github.com/air-verse/air@latest
EXPOSE 8080
CMD ["air", "-c", ".air.toml"]

# --- 本番用 -----------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot AS prod

# distroless + nonroot。シェルもパッケージマネージャも入っていないため、
# 侵入されても攻撃者が使える道具がほとんどない。
COPY --from=build /out/api /api

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/api"]
