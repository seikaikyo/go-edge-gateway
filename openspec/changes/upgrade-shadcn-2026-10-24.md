# 升級 shadcn 移除 zod 3 漏洞

- **type**: fix
- **建立日期**: 2026-10-24

## 背景

2026-10-24 修補後重跑 osv-scanner，web/scanner-ui/package-lock.json 只剩 1 筆有修補版的漏洞：zod 3.25.76（GHSA-mrhx-c7xc-m3w5，驗證超大集合時耗盡記憶體）。這份 zod 由 shadcn 4.1.2 直接相依引入，zod 3.x 沒有修補版。shadcn 4.22.0 起改相依 zod ^4.2.0，會解析到已修補的 zod 4.6.7。

## 變更內容

`npm update shadcn`，在 package.json 既有的 ^4 範圍內把 shadcn 4.1.2 升到 4.22.1，package.json 不變。

鎖定檔連帶變化：移除 shadcn 底下的 zod 3.25.76 與舊版 CLI 的相依；新增 @shadcn/registry、cn（兩者都由 shadcn 帳號經 GitHub Actions 受信任發布，沒有安裝腳本）、socks、smart-buffer、undici。

shadcn 提供的 `shadcn/tailwind.css`（src/index.css 第 3 行引入）在 4.22.0 只有新增：scroll-fade 與 shimmer 兩組工具類別，原有內容不變。專案內沒有同名類別。

## 影響範圍

| 對象 | 影響 |
| --- | --- |
| 依賴檔 | web/scanner-ui/package-lock.json |
| 樣式 | 只多出未使用的工具類別，現有畫面不變。 |
| 執行時 | shadcn 是元件產生器，執行時不載入它的 JS。 |

## UI 規格

無 UI 變更。

## 測試計畫

1. `npm audit --package-lock-only`：修補前 high 2 筆（zod 與依賴它的 shadcn），修補後 0 筆。
2. `npm ci` 後 `npm run build` 成功。
3. 推送後確認部署成功、正式網址回 200、畫面無錯誤、Sentry 無新錯誤。
