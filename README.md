# magnet-agg

插件化**影视磁力聚合搜索**（方案 2 MVP）

## 功能

- 多源并发搜索 + **infohash 去重**
- 源插件：
  - `6v520`：站内搜索 + 详情页提取 `magnet:`（中文影视）
  - `apibay`：ThePirateBay 公开 JSON API（第 2 源）
  - `torrents-csv`：torrents-csv.com 公开搜索 API（第 3 源）
  - `yts`：可选（`ENABLE_YTS=1`）；本网对 yts.mx TLS 常失败
- 极简 Web UI：搜索 / 复制磁力 / 看来源
- API：`GET /api/search?q=关键词`、`GET /api/plugins`、`GET /api/health`

## 本地 / NAS

```bash
docker compose up -d --build
# 默认 http://localhost:28910
curl -sS 'http://127.0.0.1:28910/api/search?q=inception' | head
```

环境变量：

| 变量 | 默认 | 说明 |
|------|------|------|
| `LISTEN` | `:8080` | 监听地址 |
| `SITE6V_BASE` | `https://www.6v520.com` | 6v 站点根 |
| `APIBAY_PROXY` / `TORRENTSCSV_PROXY` | 空 | 外网源专用代理（推荐，避免污染 6v） |
| `ENABLE_YTS` | 空 | 设为 `1` 启用 yts 插件 |
| `YTS_PROXY` / `YTS_BASE` | — | yts 可选代理与 API 根 |

## 扩展新源

实现 `plugin.Plugin`：

```go
type Plugin interface {
  Name() string
  Search(ctx context.Context, q string) ([]Result, error)
}
```

在 `main.go` 里 `reg.Register(...)` 即可。

## CI

GitHub Actions：`go test` + `go build` + `docker build`。

## 声明

仅供个人学习研究与自用架构验证。请遵守当地法律法规及目标站点服务条款；勿用于未授权传播。
