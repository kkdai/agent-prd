# GitHub PRD Bot

這是一個基於 Go 語言開發的 GitHub 機器人。

<img width="1024" height="1024" alt="image" src="https://github.com/user-attachments/assets/09562463-f84e-4617-9b27-672863afcc29" />


## 功能

這個機器人會根據您在 GitHub Issue 上使用的標籤 (Label) 來觸發不同的自動化功能。

### 1. 產生產品需求文件 (PRD)

當一個 GitHub Issue 被標記為 `NEED_PRD` 時，此機器人會自動執行以下操作：

1.  讀取該 Issue 的標題、內文以及專案的 `README.md` 檔案。
2.  使用 Google Gemini AI 模型生成一份英文的產品需求文件 (PRD)。
3.  偵測 Issue 內文的主要語言。
4.  將生成好的英文 PRD 翻譯成 Issue 的主要語言。
5.  在該 Issue 下方留言，同時提供英文和翻譯後的 PRD。

### 2. 產生子任務 (Sub-tasks)

當一個 GitHub Issue 被標記為 `NEED_SUB_TASK` 時，機器人會：

1.  在該 Issue 的所有留言中，尋找最新的一份 PRD 文件。
2.  根據 PRD 的內容，使用 Google Gemini AI 模型將其分解為一系列可執行的開發子任務。
3.  將產生的子任務清單（以 Markdown checklist 格式）作為一個新的留言發佈到該 Issue 中。

## 設定

您需要設定以下環境變數來運行此機器人：

-   `GITHUB_TOKEN`: 您的 GitHub Personal Access Token。
-   `GOOGLE_API_KEY`: 您的 Google AI API 金鑰。
-   `GITHUB_WEBHOOK_SECRET`: 用於驗證 GitHub Webhook 的密鑰。

## 部署

此專案包含一個 `Dockerfile`，可以輕易地將其部署為一個容器化服務。
