package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/xuri/excelize/v2"
)

type StockItem struct {
	Name string `json:"name"`
	Code string `json:"code"`
}

type Paging struct {
	TotalPages int `json:"totalpages"`
}

type StockRespone struct {
	Data struct {
		List   []StockItem `json:"list"`
		Paging Paging      `json:"paging"`
	} `json:"data"`
}

func main() {
	client := &http.Client{}
	f := excelize.NewFile()
	sheet := "Sheet1"

	headers := []string{"STT", "Tên đầy đủ", "Mã số doanh nghiệp", "Sàn niêm yết", "Mã chứng khoán", "Nơi cấp"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}
	row := 2
	totalPages := 1
	for page := 1; page <= totalPages; page++ {
		url := fmt.Sprintf("https://api.hsx.vn/l/api/v1/1/securities/stock?pageIndex=%d&pageSize=30&alphabet=&sectorId=", page)

		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "application/json")

		res, err := client.Do(req)
		if err != nil {
			panic(err)
		}
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		if err != nil {
			panic(err)
		}

		var apiRes StockRespone
		if err := json.Unmarshal(body, &apiRes); err != nil {
			fmt.Println("lỗi giải mã")
			panic(err)
		}

		if page == 1 {
			totalPages = apiRes.Data.Paging.TotalPages
		}

		for i, item := range apiRes.Data.List {

			f.SetCellValue(sheet, fmt.Sprintf("A%d", row), row-1)
			f.SetCellValue(sheet, fmt.Sprintf("B%d", row), item.Name)
			// f.SetCellValue(sheet, fmt.Sprintf("C%d", row), item.BusinessRegNo)
			// f.SetCellValue(sheet, fmt.Sprintf("D%d", row), item.ListingBoard)
			f.SetCellValue(sheet, fmt.Sprintf("E%d", row), item.Code)
			// f.SetCellValue(sheet, fmt.Sprintf("F%d", row), item.Isi)
			row++
			_ = i
		}
		time.Sleep(300 * time.Millisecond)
		fmt.Printf("page: %d, totalPages: %d, items: %d\n", page, totalPages, len(apiRes.Data.List))
	}
	if err := f.SaveAs("stock.xlsx"); err != nil {
		panic(err)
	}

	fmt.Println("Done export Excel")

}
