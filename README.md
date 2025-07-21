# GitHub PRD Bot

這是一個基於 Go 語言開發的 GitHub 機器人。

## 功能

當一個 GitHub Issue 被標記為 `NEED_PRD` 時，此機器人會自動執行以下操作：

1.  讀取該 Issue 的標題、內文以及專案的 `README.md` 檔案。
2.  使用 Google Gemini AI 模型生成一份英文的產品需求文件 (PRD)。
3.  偵測 Issue 內文的主要語言。
4.  將生成好的英文 PRD 翻譯成 Issue 的主要語言。
5.  在該 Issue 下方留言，同時提供英文和翻譯後的 PRD。

## 設定

您需要設定以下環境變數來運行此機器人：

-   `GITHUB_TOKEN`: 您的 GitHub Personal Access Token。
-   `GOOGLE_API_KEY`: 您的 Google AI API 金鑰。
-   `GITHUB_WEBHOOK_SECRET`: 用於驗證 GitHub Webhook 的密鑰。

## 部署

此專案包含一個 `Dockerfile`，可以輕易地將其部署為一個容器化服務。
