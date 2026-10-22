# 升級 golang.org/x/net 與工具鏈修補已知漏洞

- **type**: fix
- **建立日期**: 2026-10-22

## 背景

2026-10-22 每日資安巡檢的 osv-scanner 對本 repo 回報 6 筆已知漏洞，全部來自 `go.mod` 的 `golang.org/x/net` v0.55.0，最高修補版本 v0.60.0。

同日 Go 發布 go1.26.9（10/8，crypto/tls、net/http 等安全修補）與 go1.26.10（10/19，crypto/x509 封鎖遭入侵的 eMudhra 中繼憑證機構，CVE-2026-111943）。`go.mod` 的 `toolchain` 還寫 go1.26.7。repo 也一直缺 CLAUDE.md，推送檢查會擋。

## 變更內容

1. `go get golang.org/x/net@v0.60.0` 加 `go mod tidy`，連帶 x/sync 升到 v0.22.0，都在同一主版本內。
2. `go mod edit -toolchain=go1.26.10`。
3. 新增 `CLAUDE.md`，說明目錄、相依與驗證指令。

## 影響範圍

| 對象 | 影響 |
| --- | --- |
| 執行期行為 | 無功能變更。x/net 只升修補版。 |
| go.mod / go.sum | 兩檔更新。 |
| 部署 | Render 上目前沒有這個服務，推送不會觸發部署。 |

## UI 規格

無 UI 變更。

## 測試計畫

1. 在乾淨的工作樹（與 go-common、go-factory-io 同層）跑 `go build ./...`、`go test -count=1 ./...` 通過（2026-10-22 已跑過）。
2. 推送後重跑每日巡檢，確認 osv-details/go-edge-gateway.json 的漏洞數歸零。
