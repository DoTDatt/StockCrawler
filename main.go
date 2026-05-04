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
	"strings"
	"time"

	_ "github.com/lib/pq"
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
		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0")
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == 200 {
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			var data StockResponse
			if err := json.Unmarshal(body, &data); err != nil {
				return nil, err
			}
			return &data, nil
		}
		time.Sleep(5 * time.Second) //
	}
	return nil, fmt.Errorf("failed to fetch")
}

func runCrawler() {
	log.Println(">>> Start crawling")
	ctx := context.Background()

	totalPages := 1
	for page := 1; page <= totalPages; page++ {
		u, _ := url.Parse(apiURL)
		q := u.Query()
		q.Set("pageIndex", fmt.Sprintf("%d", page))
		q.Set("pageSize", fmt.Sprintf("%d", pageSize))
		u.RawQuery = q.Encode()
		finalUrl := u.String()
		data, err := fetchWithRetry(ctx, finalUrl)
		if err != nil {
			log.Printf("Lỗi fetch trang %d: %v", page, err)
			continue
		}

		if page == 1 {
			totalPages = data.Data.Paging.TotalPages
		}

		err = upsertToDB(data.Data.List)
		if err != nil {
			log.Printf("Lỗi Upsert trang %d: %v", page, err)
		} else {
			log.Printf("Page %d/%d", page, totalPages)
		}

		time.Sleep(delay)
	}
	log.Println(">>> End : total")
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

	if err := db.Ping(); err != nil {
		log.Fatal("DB không phản hồi:", err)
	}
	http.HandleFunc("/crawl", func(w http.ResponseWriter, r *http.Request) {
		go runCrawler() // chạy background
		w.Write([]byte("Crawler started"))
	})

	log.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
