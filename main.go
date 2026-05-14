package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/robfig/cron"
)

const (
	apiURL     = "https://api.hsx.vn/l/api/v1/1/securities/stock"
	pageSize   = 30
	maxRetries = 3
	delay      = 500 * time.Millisecond
)

type StockItem struct {
	Name    string `json:"name"`
	Brief   string `json:"brief"`
	Address string `json:"address"`
	Phone   string `json:"phone"`
	Fax     string `json:"fax"`
	WebUrl  string `json:"webUrl"`
	Capital uint64 `json:"capital"`
	Isin    string `json:"isin"`
}

type Paging struct {
	TotalPages int `json:"totalpages"`
}

type Data struct {
	List   []StockItem `json:"list"`
	Paging Paging      `json:"paging"`
}

type StockResponse struct {
	Data Data `json:"data"`
}

func fetchWithRetry(ctx context.Context, url string) (*StockResponse, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	for i := 0; i < maxRetries; i++ {

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}

		req.Header.Set("User-Agent", "Mozilla/5.0")

		resp, err := client.Do(req)
		if err != nil {

			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}

		switch {

		case resp.StatusCode == http.StatusOK: //200
			//go tự động break sau khi 1 case chạy xong
		case resp.StatusCode == http.StatusBadRequest:
			return nil, fmt.Errorf("400 bad request")

		case resp.StatusCode == http.StatusUnauthorized:
			return nil, fmt.Errorf("401 unauthorzied")

		case resp.StatusCode == http.StatusNotFound:
			return nil, fmt.Errorf("404 not found")

		case resp.StatusCode == http.StatusRequestTimeout:
			log.Println("408 request timeout")
			resp.Body.Close()
			time.Sleep(time.Second)
			continue

		case resp.StatusCode == http.StatusTooManyRequests:
			log.Println("429 too many request")
			resp.Body.Close()
			time.Sleep(5 * time.Second)
			continue

		case resp.StatusCode >= 500:
			log.Printf("Lỗi ở server (%d):Server đang quá tải", resp.StatusCode)
			resp.Body.Close()
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		default:
			resp.Body.Close()
			return nil, fmt.Errorf("mã lỗi không xác định %d", resp.StatusCode)

		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("lỗi khi đọc body %w", err)
		}

		var data StockResponse
		if err := json.Unmarshal(body, &data); err != nil {
			return nil, err
		}

		return &data, nil
	}

	return nil, fmt.Errorf("failed after retries")
}

func Worker(ctx context.Context, jobs <-chan int, wg *sync.WaitGroup) {

	defer wg.Done()
	for page := range jobs {
		u, _ := url.Parse(apiURL)
		q := u.Query()
		q.Set("pageindex", fmt.Sprintf("%d", page))
		q.Set("pagesize", fmt.Sprintf("%d", pageSize))
		u.RawQuery = q.Encode()

		data, err := fetchWithRetry(ctx, u.String())
		if err != nil {
			log.Printf("Page[%d] fetch error: %v", page, err)
			continue
		}

		err = upsertToDB(data.Data.List)
		if err != nil {
			log.Printf("Page %d db error: %v", page, err)
		} else {
			log.Println("Success")
		}
		time.Sleep(delay)

	}
}
func Crawler() {
	log.Println("Start crawling")
	ctx := context.Background()

	firstPage, err := fetchWithRetry(ctx, apiURL+"?PageIndex=1&pageSize=50")
	if err != nil {
		log.Println("Không thể lấy trang đầu", err)
		return
	}
	totalPage := firstPage.Data.Paging.TotalPages

	jobs := make(chan int, totalPage)
	var wg sync.WaitGroup
	numWorkers := 3

	for i := 1; i <= numWorkers; i++ {
		wg.Add(1)
		go Worker(ctx, jobs, &wg)
	}

	for page := 1; page <= totalPage; page++ {
		jobs <- page
	}
	close(jobs)

	wg.Wait()
	log.Println("Finished all page")
}

func upsertToDB(items []StockItem) error {
	if len(items) == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, item := range items {
		if item.Brief == "" || item.Name == "" {
			continue
		}

		idno := item.Isin
		if idno == "" {
			idno = item.Brief
		}

		slug := strings.ToLower(strings.ReplaceAll(item.Name, " ", "-"))

		companyQuery := `
            INSERT INTO companies 
            (id_no, short_name, address, telephone, fax, website, capital, exchange, status, type)
            VALUES (?, ?, ?, ?, ?, ?, ?, 'HOSE', 'active', 'congtydaichung')
            ON DUPLICATE KEY UPDATE
                address = VALUES(address),
                telephone = VALUES(telephone),
                capital = VALUES(capital),
                updated_at = NOW()
        `
		_, err = tx.Exec(companyQuery,
			idno, item.Brief, item.Address, item.Phone,
			item.Fax, item.WebUrl, item.Capital,
		)
		if err != nil {
			return err
		}

		translationQuery := `
            INSERT INTO company_translations (company_id, lang_code, name, slug, description)
            SELECT company_id, 'vi', ?, ?, NULL 
            FROM companies 
            WHERE id_no = ? AND short_name = ?
            ON DUPLICATE KEY UPDATE
                name = VALUES(name),
                slug = VALUES(slug)
        `

		_, err = tx.Exec(translationQuery, item.Name, slug, idno, item.Brief)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

var db *sql.DB

func main() {

	var err error

	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_NAME"),
	)

	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal("Lỗi kết nối DB:", err)
	}

	defer db.Close()

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	c := cron.New()
	Crawler()
	c.AddFunc("60000s", func() {
		Crawler()
	})
	c.Start()

	log.Println("Crawler")
	select {}
}
