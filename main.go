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

	_ "github.com/go-sql-driver/mysql"
	"github.com/robfig/cron/v3"
)

const (
	dsn        = "root:root@tcp(127.0.0.1:3306)/vtv_index_news_db2?charset=utf8mb4&parseTime=True&loc=Local"
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

	var companyValues []string
	var companyArray []any
	var translationValues []string
	var translationArgs []any

	for _, item := range items {
		if item.Brief == "" || item.Name == "" {
			continue
		}

		idno := item.Isin
		if idno == "" {
			idno = item.Brief
		}
		slug := strings.ToLower(strings.ReplaceAll(item.Name, " ", "-"))

		companyValues = append(companyValues, "(?, ?, ?, ?, ?, ?, NULL, ?, 'HOSE', 'active', 'congtydaichung')")

		companyArray = append(companyArray,
			idno,
			item.Brief,
			item.Address,
			item.Phone,
			item.Fax,
			item.WebUrl,
			item.Capital,
		)

		translationValues = append(translationValues, "(?,'vi'?,?,null)")
		translationArgs = append(translationArgs, idno, item.Name, slug)

	}

	companyUpsertQuery := `
		INSERT INTO companies 
		(id_no, short_name, address, telephone, fax, website, email, capital, exchange, status, type)
		VALUES` + strings.Join(companyValues, ",") + `
		ON DUPLICATE KEY UPDATE
		address = VALUES(address),
		telephone = VALUES(telephone),
		capital = VALUES(capital),
		updated_at = NOW()
		`
	tx.Exec(companyUpsertQuery, companyArray...)

	companyTranslationsUpsertQuery := `
		INSERT INTO company_translations
		(company_id, lang_code, name, slug, description)
		VALUES ` + strings.Join(translationValues, ",") + `
		ON DUPLICATE KEY UPDATE
		name = VALUES(name),
		slug = VALUES(slug)	
		`

	tx.Exec(companyTranslationsUpsertQuery, translationArgs...)

	return tx.Commit()
}

var db *sql.DB

func main() {

	var err error

	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal("Lỗi kết nối DB:", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal("DB không phản hồi:", err)
	}
	runCrawler()
	c := cron.New()
	c.AddFunc("@every 30s", func() {
		runCrawler()
	})
	c.Start()

	log.Println("--- Start ---")
	select {}
}
