# PayPay NISA 投信分析器

以 React 前端、Go API 與 PostgreSQL 建立的繁體中文投信研究工具。它只讀取 PayPay 證券公開基金清單，不登入帳戶、不下單，也不把主觀新聞或情緒混入歷史情境計算。

## 本機啟動

先在專案根目錄的 `.env` 填入 Supabase Session Pooler 的 `DATABASE_URL`，再執行：

```powershell
docker compose up --build
```

開啟 `http://localhost:8080`。Compose 會啟動兩個服務：React 網站與 Go API；資料永久儲存在 Supabase PostgreSQL。

停止服務：

```powershell
docker compose down
```

## 開發模式

在 PowerShell 將 Supabase 連線字串放入工作階段，再各自啟動 API 與 React 開發伺服器：

```powershell
$env:DATABASE_URL = "你的 Supabase Session Pooler URI"
go run ./cmd/server
```

在另一個終端機：

```powershell
cd frontend
npm ci
npm run dev
```

React 開發伺服器是 `http://localhost:5173`，並會將 `/api` 請求代理到 Go API。

## 匯入可驗證的歷史資料

推估只接受發行商提供、已將稅前分配金再投資的淨值資料。CSV 必須有 `date,adjusted_nav` 欄位。設定 `DATABASE_URL` 後執行：

```powershell
go run ./cmd/importcsv -fund "基金 ID" -csv "C:\path\official-adjusted-nav.csv" -source "https://發行商的原始資料網址"
```

匯入至少 61 個不同月份的資料後，頁面才會產生 5 年歷史情境。

## Supabase 正式環境

1. 在 Supabase 建立空白專案；不要手動用後台建表。
2. 安裝並登入 Supabase CLI，將本機專案連結到該專案，然後執行 `supabase db push`。它會套用版本控管的遷移檔。
3. 在部署平台的祕密設定中填入 Supabase 提供的 PostgreSQL `DATABASE_URL`，供 Go API 使用。
4. React 不直接連 Supabase 資料 API；瀏覽器只呼叫 Go API。絕不可把資料庫密碼或 `service_role` 密鑰放進前端或 Git。

日後新增表或欄位時，一律先用 `supabase migration new <名稱>` 建立遷移檔，再套用到正式環境。

本機直接驗證 Supabase 連線時，在專案根目錄建立或開啟未納入 Git 的 `.env`，將 Session pooler 的連線字串填入其中：

```powershell
# 填入 DATABASE_URL=你從 Supabase Connect 視窗複製的字串。
docker compose up --build -d
```

## 驗證

```powershell
go test ./...
cd frontend; npm ci; npm run build
```
