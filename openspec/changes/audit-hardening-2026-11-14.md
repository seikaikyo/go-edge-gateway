# 單一設備的壞回覆只影響該設備的連線

- **type**: fix
- **status**: in progress
- **建立日期**: 2026-11-14

## 背景

2026-11-14 的安全檢查發現兩件事：

1. 閘道器連上 SECS/GEM 設備後送 S1F13，設備回覆的本體由 go-factory-io 解碼。解碼器原本沒有巢狀深度上限，過深的回覆會讓整個閘道器行程結束，所有外掛（Modbus、MQTT）與 HTTP API 一起停。根因在 go-factory-io，已在該 repo 修正；本 repo 另外補上自己這一層的防護。
2. `Dockerfile` 取得 go-factory-io 時設的 `GONOSUMCHECK` 不是 Go 認得的環境變數，模組實際會經公開 proxy 解析，`insteadOf` 的導向沒有作用。

## 變更內容

### 1. secsgem 外掛的 recover 與錯誤處理

`plugin/secsgem/secsgem.go`

- 原本：每台設備的連線流程（連線、S1F13、等待斷線）跑在同一個 goroutine，沒有 recover；處理回覆時的 panic 會結束整個行程。S1F13 失敗時只記錄錯誤，連線維持開著但沒有建立通訊。
- 現在：單次連線的流程移到 `runConnection`，裡面的 panic 被攔下，記錄日誌與健康狀態後關閉該設備的連線，照原本的 10 秒間隔重連；S1F13 失敗（包括回覆解碼失敗）時關閉該設備的連線並照同樣間隔重連。其他設備與外掛不受影響。

### 2. Dockerfile 的模組取得方式

`Dockerfile`

- 原本：`ENV GONOSUMCHECK=...`（無效設定），go-factory-io 經公開 proxy 與 checksum 資料庫解析。
- 現在：`ENV GOPRIVATE=github.com/dashfactory/go-factory-io`，同時涵蓋 GONOPROXY 與 GONOSUMDB，go 指令改用 git 直接取得，`insteadOf` 把位址導向實際的 repo。

## 影響範圍

| 對象 | 影響 |
| --- | --- |
| secsgem 外掛 | 設備回覆無法解碼或處理時，只有該設備斷線重連；S1F13 失敗改為斷線重連，不再停在已連線但未建立通訊的狀態。 |
| 其他外掛與 HTTP API | 不受單一設備的回覆影響。 |
| go.mod / go.sum | 不變。go-factory-io 新版發佈後另行升版。 |
| Docker 建置 | go-factory-io 改由 git 直接取得。 |

## 未在本提案處理

- go.sum 釘選 go-factory-io 的雜湊：需要從網路取得模組才能算出，且本機 `replace` 會讓 `go mod tidy` 移除該行，另案處理。
- go-factory-io 模組路徑與實際 repo 位置不一致：屬於 go-factory-io 的結構變更，另案決定。

## UI 規格

無 UI 變更。

## 測試計畫

1. `plugin/secsgem` 新增測試，先確認改前失敗、改後通過：
   - 一台設備回覆過深巢狀的 S1F14、另一台正常回覆：壞設備的連線被關閉，正常設備維持連線並送出資料。
   - 處理單一設備時發生 panic：行程存活，該設備記錄錯誤（子程序驗證）。
2. 以本機的 go-factory-io 修正版（`replace` 指向本機路徑）跑 `go build ./...`、`go vet ./...`、`go test ./...`，不比改動前多失敗；結束後 go.mod、go.sum 與改動前相同。
3. 本機 127.0.0.1 起閘道器與假設備，重打原始重現情境，確認閘道器行程存活。
