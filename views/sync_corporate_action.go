package views

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/go-resty/resty/v2"
	"github.com/pocketbase/pocketbase/core"
)

// CorporateActionResponse matches the top-level API response
type CorporateActionResponse struct {
	StatusCode int                   `json:"statusCode"`
	Message    string                `json:"message"`
	Result     CorporateActionResult `json:"result"`
}

// CorporateActionResult contains the data slice
type CorporateActionResult struct {
	Data []CorporateAction `json:"data"`
}

// CorporateAction represents the specific dividend/right share record
type CorporateAction struct {
	CompanyName          string  `json:"companyName"`
	StockSymbol          string  `json:"stockSymbol"`
	Bonus                string  `json:"bonus"`
	Cash                 string  `json:"cash"`
	TotalDividend        string  `json:"totalDividend"`
	BookClosureDateAD    string  `json:"bookClosureDateAD"`
	BookClosureDateBS    string  `json:"bookClosureDateBS"`
	FiscalYearAD         string  `json:"fiscalYearAD"`
	FiscalYearBS         string  `json:"fiscalYearBS"`
	RightShare           *string `json:"rightShare"`
	RightBookCloseDateAD *string `json:"rightBookCloseDateAD"`
	RightBookCloseDateBS *string `json:"rightBookCloseDateBS"`
}

// syncCorporateActions fetches data using a worker pool with 4 concurrent processes
func syncCorporateActions(symbols []string) []CorporateAction {
	const numWorkers = 4
	jobs := make(chan string, len(symbols))
	results := make(chan []CorporateAction, len(symbols))
	var wg sync.WaitGroup

	client := resty.New()

	for w := 1; w <= numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for symbol := range jobs {
				url := fmt.Sprintf("https://nepalipaisa.com/api/GetDividendRights?stockSymbol=%s&pageNo=1&itemsPerPage=100&pagePerDisplay=100", symbol)

				var apiResponse CorporateActionResponse
				resp, err := client.R().
					SetResult(&apiResponse).
					Get(url)

				if err != nil {
					log.Printf("[%s] Request error: %v", symbol, err)
					continue
				}

				if resp.IsError() {
					log.Printf("[%s] API error: %d", symbol, resp.StatusCode())
					continue
				}

				results <- apiResponse.Result.Data
			}
		}()
	}

	for _, s := range symbols {
		jobs <- s
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
	}()

	var allActions []CorporateAction
	for res := range results {
		allActions = append(allActions, res...)
	}

	return allActions
}

// SyncCorporateAction is the HTTP handler that fetches distinct stock symbols
// from daily_transactions and syncs their corporate actions from the external API.
func SyncCorporateAction(e *core.RequestEvent) error {
	type symbolRow struct {
		Symbol string `db:"symbol"`
	}

	var rows []symbolRow
	err := e.App.DB().NewQuery("SELECT DISTINCT symbol FROM daily_transactions").All(&rows)
	if err != nil {
		return e.JSON(http.StatusInternalServerError, map[string]any{
			"status": "failed",
			"error":  err.Error(),
		})
	}

	symbols := make([]string, 0, len(rows))
	for _, r := range rows {
		symbols = append(symbols, r.Symbol)
	}

	actions := syncCorporateActions(symbols)

	// Summarise results

	bonusCollection, _ := e.App.FindCollectionByNameOrId("corporate_actions_bonus")
	dividendCollection, _ := e.App.FindCollectionByNameOrId("corporate_actions_cash_dividend")
	rightShareCollection, _ := e.App.FindCollectionByNameOrId("corporate_actions_right_share")

	err = e.App.RunInTransaction(func(txApp core.App) error {
		for _, a := range actions {
			bonus, _ := strconv.ParseFloat(a.Bonus, 64)
			cash, _ := strconv.ParseFloat(a.Cash, 64)

			if a.BookClosureDateAD == "" {
				continue
			}
			if bonus > 0 {
				record := core.NewRecord(bonusCollection)
				record.Set("symbol", strings.ToUpper(a.StockSymbol))
				record.Set("book_close_date", a.BookClosureDateAD)
				record.Set("bonus_pct", bonus)
				if err := txApp.Save(record); err != nil {
					// if strings.Contains(err.Error(), "UNIQUE constraint failed") {
					// 	continue // ignore if unique constraint
					// }
					// return err
					continue
				}
			}
			if cash > 0 {
				record := core.NewRecord(dividendCollection)
				record.Set("symbol", strings.ToUpper(a.StockSymbol))
				record.Set("book_close_date", a.BookClosureDateAD)
				record.Set("cash_dividend_pct", cash)
				if err := txApp.Save(record); err != nil {
					continue
				}
			}
			if a.RightShare != nil {
				record := core.NewRecord(rightShareCollection)
				record.Set("symbol", strings.ToUpper(a.StockSymbol))
				record.Set("book_close_date", a.BookClosureDateAD)
				record.Set("right_share_ratio", a.RightShare)
				if err := txApp.Save(record); err != nil {
					continue
				}
			}
		}
		return nil
	})

	if err != nil {
		return e.JSON(http.StatusOK, map[string]any{
			"status": "failed",
			"err":    err,
		})
	}

	return e.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"msg":    "successfully sync corporate actions",
	})
}
