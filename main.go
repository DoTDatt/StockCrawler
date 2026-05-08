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

	_ "github.com/lib/pq"
	"github.com/robfig/cron"
)

const (
	dsn        = "postgresql://postgres.kuopewvcftlvjltejfkt:Datdooiladatdo@aws-1-ap-northeast-1.pooler.supabase.com:6543/postgres?sslmode=require"
	apiURL     = "https://api.hsx.vn/l/api/v1/1/securities/stock"
	pageSize   = 50
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

		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return nil, fmt.Errorf("client error: %d", resp.StatusCode)
		}

		if resp.StatusCode >= 500 {
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		resp.Body.Close()
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

	wg.Done()
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

		var companyID int

		err = tx.QueryRow(`
			INSERT INTO companies 
			(id_no, short_name, address, telephone, fax, website, capital, exchange, status, type)
			VALUES ($1,$2,$3,$4,$5,$6,$7,'HOSE','active','congtydaichung')
			ON CONFLICT (short_name, id_no)
			DO UPDATE SET
				address = EXCLUDED.address,
				telephone = EXCLUDED.telephone,
				capital = EXCLUDED.capital,
				updated_at = CURRENT_TIMESTAMP
			RETURNING company_id
		`,
			idno,
			item.Brief,
			item.Address,
			item.Phone,
			item.Fax,
			item.WebUrl,
			item.Capital,
		).Scan(&companyID)

		if err != nil {
			return err
		}

		_, err = tx.Exec(`
			INSERT INTO company_translations
			(company_id, lang_code, name, slug, description)
			VALUES ($1,'vi',$2,$3,NULL)
			ON CONFLICT (company_id, lang_code)
			DO UPDATE SET
				name = EXCLUDED.name,
				slug = EXCLUDED.slug
		`,
			companyID,
			item.Name,
			slug,
		)

		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

var db *sql.DB

func main() {

	var err error

	db, err = sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal("Lỗi kết nối DB:", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		log.Fatal("DB không phản hồi:", err)
	}
	// http.HandleFunc("/crawl", func(w http.ResponseWriter, r *http.Request) {
	// 	go func() {
	// 		defer func() {
	// 			if r := recover(); r != nil {
	// 				log.Println("crawler panic recovered:", r)
	// 			}
	// 		}()
	// 		Crawler()
	// 	}()
	// 	w.Write([]byte("Crawler started"))
	// })

	c := cron.New()
	c.AddFunc("@every 30s", func() {
		Crawler()
	})
	c.Start()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
