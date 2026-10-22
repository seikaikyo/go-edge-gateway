# go-edge-gateway

製造現場用的邊緣閘道器，單一 Go 執行檔，以外掛方式把 SECS/GEM、Modbus、MQTT 設備資料轉送到 MES 或雲端。架構圖與設定說明見 `README.md`。

## 目錄

| 路徑 | 內容 |
|------|------|
| `cmd/edge-gateway/` | 執行檔進入點 |
| `core/` | 閘道器主體：設定載入、外掛協調、上傳（uplink）、HTTP API、安全設定 |
| `plugin/` | 外掛介面與註冊表，`modbus`、`mqtt`、`secsgem` 三個外掛 |
| `internal/scan/` | Modbus 暫存器掃描，用 go-common 的 `modbus` 套件 |
| `web/scanner-ui/` | 掃描介面前端 |
| `edge-gateway.yaml`、`edge-gateway.demo.yaml` | 設定檔範本與展示用設定 |
| `openspec/` | 變更提案，改動 20 行以上先在 `openspec/changes/` 建提案 |

## 相依

- `go.mod` 用 `replace` 指到同層目錄的 `../go-common` 與 `../go-factory-io`，建構前這兩個 repo 要 clone 在旁邊
- `render.yaml` 與 `Dockerfile` 留著但 Render 上目前沒有這個服務

## 開發

- Go 版本照 `go.mod`（toolchain go1.26.10）
- 驗證：`go build ./...`、`go vet ./...`、`go test ./...`
- 這是公開 repo，不放本機路徑、帳號、金鑰
