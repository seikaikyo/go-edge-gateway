---
title: Security Audit Remediation for Scan API
type: fix
status: in-progress
created: 2026-08-31
---

# Security Audit Remediation for Scan API

## Background

稽核在 scan API 找到一組可串起來的問題。`core/api.go` 的 `NewAPIServer` 只掛
`chimw.RealIP` 與 `chimw.Recoverer`，`/api/scan`、`/api/scan/quick`、`/api/read`
三個端點沒有任何認證，掃描目標的 host / port / serial_port 完全由請求 body 決定，
伺服器又綁 `:%d`（0.0.0.0）。`render.yaml` 把它宣告成公開 web 服務（實測目前未部署、
回 404，但設定在）。合起來就是一台開放給任何人使用的內網 Modbus 掃描器。

## 變更內容

### 1. 認證與繫結位址（Critical）

- 預設繫結 `127.0.0.1`。要對外聽必須明示：`--listen` 旗標或 `EDGE_GATEWAY_LISTEN`
  環境變數或設定檔 `api.listen`。優先序 flag > env > config > 預設。
- `/api/*` 全數要求 bearer token。token 從環境變數讀（預設 `EDGE_GATEWAY_API_TOKEN`，
  可用 `api.token_env` 改名），比對走 `crypto/subtle.ConstantTimeCompare`。
- Fail closed：沒設 token 時 `/api/*` 一律回 503，不是放行。啟動時記一筆 warn log。
- `/health` 與嵌入式 UI 靜態檔維持不需認證（Render healthCheckPath 需要）。

### 2. 掃描目標白名單（Critical / High）

- 新增 `scan.allowed_targets` CIDR 允許清單。預設 loopback 加 RFC1918
  （`127.0.0.0/8`、`::1/128`、`10.0.0.0/8`、`172.16.0.0/12`、`192.168.0.0/16`）。
- 不論設定為何，`169.254.0.0/16` 與 `fe80::/10` 一律拒絕（雲端 metadata 端點）。
- host 是網域名稱時先解析，所有解析出的 IP 都要在白名單內才放行。
- `serial_port` 必須是 `modbus.ListSerialPorts()` 回傳集合的成員，任意路徑不再進
  `NewRTUClientHandler` → `syscall.Open(O_RDWR)`。

### 3. 錯誤訊息不外洩（High）

- `connect failed: <err>` / `read failed: <err>` 這類原文改回通用字串，底層 error
  只寫 slog。非同步 job 的 `error` 欄位同樣只存通用字串。
- 目的是拿掉「連線被拒 vs 逾時」可區分的內網埠掃描 oracle。

### 4. CSRF 與 Content-Type（Medium）

- 非 GET 的 `/api/*` 要求 `Content-Type: application/json`（允許帶 charset 參數）。
- 有帶 `Origin` 標頭時，必須同源（host 相符）或列在 `api.allowed_origins`，否則 403。

## 影響範圍

| 檔案 | 變更 |
|---|---|
| `core/config.go` | 新增 `api` 與 `scan` 設定區段 |
| `core/api.go` | 繫結位址、認證中介層、Content-Type / Origin 中介層 |
| `core/security.go` | 新增：token 中介層、請求來源檢查 |
| `internal/scan/policy.go` | 新增：CIDR 白名單、serial port 白名單 |
| `internal/scan/scan.go` | handler 改為 struct method、套白名單、錯誤訊息去識別 |
| `cmd/edge-gateway/main.go` | `--listen` 旗標 |
| `edge-gateway.yaml` / `.demo.yaml` | 新設定範例 |

跨模組注意：認證中介層與 host 白名單都做在本 repo，不動 `go-common`（避免為此發版）。

## UI 規格

嵌入式 React UI 不在本次變更範圍。加上認證後 UI 需要帶 token 才能呼叫 `/api/*`，
現行 bundle 沒有 token 輸入欄位，屬已知後續工作，記在殘餘風險。

## 測試計畫

Go 表格測試涵蓋四項控制：

- `core/security_test.go`：未帶 token 被擋（401）、token 錯誤被擋、正確 token 放行、
  未設定 token 時回 503、非 JSON Content-Type 被擋（415）、跨站 Origin 被擋（403）、
  同源 Origin 放行。
- `internal/scan/policy_test.go`：CIDR 白名單准駁、link-local 永遠拒絕、
  網域名稱解析後全部 IP 都要合格、serial port 白名單准駁。
- `internal/scan/scan_test.go`：TCP 目標不在白名單回 403 且訊息不含底層 error、
  RTU serial_port 不在清單回 400、連線失敗訊息是通用字串。

驗收：`go build ./...` 與 `go test ./...` 實際通過。

## 殘餘風險

- DNS rebinding：白名單在 handler 層解析一次，`goburrow/modbus` 連線時會再解析一次，
  中間有 TOCTOU 窗口。要完全關掉需要在 dialer 層綁定已驗證的 IP，牽動 `go-common`
  的 `NewClient`，本次不做。
- 時間側通道：錯誤訊息統一後，「連線被拒」仍比「逾時」早回，回應時間仍可區分。
  要抹平需要固定延遲，會拖慢正常掃描，本次不做。
- 嵌入式 UI 尚未支援 token，加上認證後 UI 的掃描功能會失效。
