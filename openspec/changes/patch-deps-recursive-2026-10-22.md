# 升級 scanner-ui 的 npm 相依套件修補已知漏洞

- **type**: fix
- **建立日期**: 2026-10-22

## 背景

2026-10-22 每日資安巡檢的 osv-scanner 原本沒有遞迴掃描，子目錄的鎖定檔沒被掃到。補上遞迴掃描後，`web/scanner-ui/package-lock.json` 有 19 個套件、100 筆漏洞可在同一主版號內修補。修補前 `npm audit`：critical 1、high 17、moderate 6、low 2，共 26 筆（`--omit=dev` 為 24 筆）。

Go 模組已在同日的 `patch-go-modules-2026-10-22` 提案處理，本提案不動 `go.mod` 與 `go.sum`。

## 變更內容

只用 `npm update` 在 package.json 既有範圍內升級，不改 package.json、不加 overrides、不跨主版號：

1. 直接相依：vite 8.0.3 → 8.3.5
2. 間接相依（經 shadcn CLI 引入）：@modelcontextprotocol/sdk 1.29.0 → 1.32.1、hono 4.12.9 → 4.13.13、@hono/node-server 1.19.12 → 2.1.5（sdk 1.32.1 宣告 `^1.19.9 || ^2.0.5`）、express-rate-limit 8.3.2 → 8.7.1、ip-address 10.1.0 → 10.7.4、body-parser 2.2.2 → 2.4.0、qs 6.15.0 → 6.16.1、proxy-addr 2.0.7 → 2.0.8、fast-uri 3.1.0 → 3.1.9
3. 間接相依（建構工具鏈）：@babel/core 7.29.0 → 7.29.7、postcss 8.5.8 → 8.5.31、postcss-selector-parser 7.1.1 → 7.1.7、nanoid 3.3.11 → 3.3.20、source-map-js 1.2.1 → 1.2.2、browserslist 4.28.2 → 4.29.4、baseline-browser-mapping 2.10.13 → 2.11.30、js-yaml 4.1.1 → 4.3.2、brace-expansion 1.1.13 → 1.1.21 與 5.0.5 → 5.0.12、@humanfs/node 0.16.7 → 0.16.8

## 影響範圍

| 對象 | 影響 |
| --- | --- |
| 執行期行為 | 無功能變更。scanner-ui 的 JS 建構產物 255.74 kB → 255.44 kB，CSS 38.78 kB 不變。 |
| 依賴檔 | 只動 `web/scanner-ui/package-lock.json`。 |
| 內嵌前端 | `internal/scan/web/` 的建構產物不在本次提交內，下次重新建構前端時才會更新。 |
| 部署 | Render 上目前沒有這個服務，推送不會觸發部署。 |

## 剩餘風險

修補後 `npm audit`：high 6 筆，全部來自 braces 3.0.3（GHSA-vfj7-8cjw-p6xm），經 shadcn → ts-morph、fast-glob → micromatch 引入。braces 官方沒有修補版，等上游發版。

## UI 規格

無 UI 變更。

## 測試計畫

1. `npm ci` 後 `npm run build`（`tsc -b` 加 `vite build`）修補前後都成功。
2. `npm run lint` 修補前後都失敗，原因相同：repo 沒有 `eslint.config.js`，與本次升級無關。
3. `npm audit` 修補前 26 筆，修補後 6 筆。
4. 推送後重跑每日巡檢，確認 osv-details/go-edge-gateway.json 的漏洞數下降。
