# Stock Crawler (Go)

## OVERVIEW

A lightweight and high-performance Go tool that automatically crawls Vietnamese stock data from the official Ho Chi Minh City Stock Exchange (STOCK) API and exports it into a formatted Excel report.

The tool is designed for simplicity, speed, and reliability — suitable for data collection, reporting, or financial analysis workflows.

---

## KEY FEATURES

### Auto Pagination
Automatically detects total pages from API response and crawls all available stock data without manual configuration.

### Excel Export
Generates clean `.xlsx` reports using `excelize/v2` with structured headers and formatted columns.

### Rate Limit Protection
Built-in delay (300ms) between requests to prevent API blocking and ensure stable crawling.

### Lightweight & Fast
Optimized Go implementation with minimal memory usage and fast execution.

---

## TECH STACK

- Go (Golang)
- net/http (API requests)
- encoding/json (JSON parsing)
- excelize/v2 (Excel generation)

---

## INPUT / OUTPUT 

## INPUT

| Parameter | Description |
|-----------|-------------|
| API Endpoint | HSX stock listing API |
| pageIndex | Page number for pagination |
| pageSize | Number of records per request |
| Headers | User-Agent, Accept |
## OUTPUT

| Column | Description |
|--------|-------------|
| STT | Sequential index |
| Tên đầy đủ | Company full name |
| Mã chứng khoán | Stock ticker symbol |

- Output file: `stock.xlsx`

## Data flow
HSX API
->
HTTP Client Request
->
JSON Response
->
Data Unmarshalling (Go Struct)
->
Data Transformation
->
Excel Generation
->
Output File (.xlsx)
## HOW TO RUN

## INSTALLATION

### 1.Install dependencies

```bash
go mod tidy
go get github.com/xuri/excelize/v2
```
## 2.USAGE EXAMPLE

```go
go run main.go
```
## 3.OUTPUT
stock.xlsx