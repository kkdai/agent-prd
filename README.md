# GitHub PRD Bot

這是一個基於 Go 語言和 Google Gemini AI 開發的 GitHub 機器人。它可以自動為新的 Issue 產生產品需求文件 (PRD)，並根據 PRD 拆解開發子任務。

<img width="1024" height="1024" alt="image" src="https://github.com/user-attachments/assets/09562463-f84e-4617-9b27-672863afcc29" />

## 功能

這個機器人可以透過兩種方式觸發：

1.  **自動觸發**：當一個新的 Issue 被建立時，自動產生 PRD。
2.  **手動觸發**：在 Issue 留言中提及 (mention) 機器人並附上指令。

### 1. 產生產品需求文件 (PRD)

-   **自動觸發**: 建立一個新的 Issue。
-   **手動指令**: `@<bot-name> need_prd`
-   **流程**:
    1.  讀取該 Issue 的標題、內文以及專案的 `README.md` 檔案。
    2.  使用 Google Gemini AI 模型生成一份英文的產品需求文件 (PRD)。
    3.  偵測 Issue 內文的主要語言。
    4.  將生成好的英文 PRD 翻譯成 Issue 的主要語言。
    5.  在該 Issue 下方留言，同時提供英文和翻譯後的 PRD。

### 2. 產生子任務 (Sub-tasks)

-   **手動指令**: `@<bot-name> need_sub_task`
-   **流程**:
    1.  在該 Issue 的所有留言中，尋找最新的一份 PRD 文件。
    2.  根據 PRD 的內容，使用 Google Gemini AI 模型將其分解為一系列可執行的開發子任務。
    3.  將產生的子任務清單（以 Markdown checklist 格式）作為一個新的留言發佈到該 Issue 中。

---

## 安裝與設定

您需要將此機器人設定為一個 **GitHub App** 並部署它，然後在您的 Repository 中安裝該 App。

### 步驟 1: 建立 GitHub App

1.  前往 GitHub 的開發者設定頁面: **Settings** > **Developer settings** > **GitHub Apps** > **New GitHub App**。
2.  **App name**: 為您的機器人取一個名字，例如 `prd-bot-for-my-org`。
3.  **Homepage URL**: 填寫您的專案 GitHub 網址。
4.  **Webhook**:
    -   勾選 **Active**。
    -   **Webhook URL**: 填寫您部署後服務的公開網址，並在結尾加上 `/webhook` (例如: `https://your-service-url.com/webhook`)。
    -   **Webhook secret**: 產生一個安全的隨機字串，並記錄下來。稍後會用到。
5.  **Permissions**:
    -   **Repository permissions**:
        -   **Issues**: 設定為 `Read & write`。
        -   **Contents**: 設定為 `Read-only` (用於讀取 README.md)。
6.  **Subscribe to events**:
    -   勾選 **Issues**。
    -   勾選 **Issue comment**。
7.  點擊 **Create GitHub App**。

### 步驟 2: 取得 App 憑證並設定環境變數

建立 App 後，您需要取得以下資訊來設定環境變數：

-   `GITHUB_APP_ID`: 在 App 的 "General" 設定頁面可以找到 App ID。
-   `GITHUB_APP_NAME`: 您為 App 設定的名稱 (例如 `prd-bot-for-my-org`)。
-   `GITHUB_WEBHOOK_SECRET`: 您在步驟 1-4 中建立的 Webhook secret。
-   `GOOGLE_API_KEY`: 您的 Google AI API 金鑰。
-   `GITHUB_APP_PRIVATE_KEY`:
    1.  在 App 的 "General" 設定頁面下方，點擊 **Generate a new private key** 來下載一個 `.pem` 檔案。
    2.  **重要**: 您需要將此 `.pem` 檔案的內容進行 Base64 編碼。在終端機中執行以下指令 (macOS 或 Linux):
        ```bash
        base64 -i your-downloaded-key.pem
        ```
    3.  將指令輸出的**那一長串沒有換行的字串**作為此環境變數的值。

### 步驟 3: 安裝並部署

1.  **安裝 App**:
    -   在您的 GitHub App 設定頁面，點擊左側的 **Install App**。
    -   將此 App 安裝到您希望它運作的 Repository 中。
2.  **部署服務**:
    -   此專案包含一個 `Dockerfile`，可以輕易地將其部署為一個容器化服務 (例如 Google Cloud Run, Heroku, Fly.io 等)。
    -   在部署時，請務必將上述 5 個環境變數設定好。

---

## 部署

此專案包含一個 `Dockerfile`，可以輕易地將其部署為一個容器化服務。

```bash
# 1. 建立 Docker image
docker build -t your-image-name .

# 2. 執行容器 (本地測試)
# (請將 YOUR_... 替換為您的真實變數值)
docker run -p 8080:8080 \
  -e GITHUB_APP_ID="YOUR_APP_ID" \
  -e GITHUB_APP_PRIVATE_KEY="YOUR_BASE64_ENCODED_PRIVATE_KEY" \
  -e GITHUB_APP_NAME="YOUR_APP_NAME" \
  -e GOOGLE_API_KEY="YOUR_GOOGLE_KEY" \
  -e GITHUB_WEBHOOK_SECRET="YOUR_WEBHOOK_SECRET" \
  your-image-name
```