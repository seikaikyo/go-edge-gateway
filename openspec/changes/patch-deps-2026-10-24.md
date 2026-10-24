# 修補 chi 的 X-Forwarded-For 偽造與 scanner-ui 的 zod

- **type**: fix
- **建立日期**: 2026-10-24

## 背景

2026-10-24 每日巡檢的 osv-scanner 列出 3 筆有修補版的漏洞：

1. go.mod：github.com/go-chi/chi/v5 v5.3.0，GHSA-j6qf-3m9c-vwj3（CVE-2026-114437，middleware 的 ClientIPFromXFF 可被 X-Forwarded-For 偽造用戶端 IP），v5.4.0 修補。
2. web/scanner-ui/package-lock.json：zod 4.3.6，GHSA-mrhx-c7xc-m3w5（驗證超大集合時耗盡記憶體），4.6.6 修補。

## 變更內容

1. `go get github.com/go-chi/chi/v5@v5.4.0` 後 `go mod tidy`。連帶升級 golang.org/x/sync v0.22.0 → v0.23.0，go.sum 補上 paho.mqtt.golang 需要的 x/sync 雜湊。
2. web/scanner-ui：`npm update zod`，zod 4.3.6 → 4.6.7。

不改程式碼，internal/scan/web 的建構產物不重新產生。

## 影響範圍

| 對象 | 影響 |
| --- | --- |
| Go 服務 | chi 次版號升級，路由 API 不變。 |
| scanner-ui | zod 修補版升級，打包內容不變。 |
| 依賴檔 | go.mod、go.sum、web/scanner-ui/package-lock.json。 |

## 剩餘風險

web/scanner-ui 另有一份 zod 3.25.76，由 shadcn 4.1.2 直接相依引入。zod 3.x 沒有修補版，等 shadcn 改用 zod 4。

## UI 規格

無 UI 變更。

## 測試計畫

1. 以 `git archive HEAD` 取出乾淨複本（旁邊放 go-common、go-factory-io 讓 replace 指向有效），套用新的 go.mod、go.sum 後 `go mod tidy` 無差異，`go build ./...`、`go vet ./...` 通過，`go test ./...` 全部通過。
2. web/scanner-ui `npm ci` 後 `npm run build` 成功。
