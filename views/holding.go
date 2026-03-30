package views

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

type DailyTransactionRecord struct {
	Id               string         `db:"id" json:"id"`
	TrnNo            string         `db:"trn_no" json:"trn_no"`
	ClientName       string         `db:"client_name" json:"client_name"`
	Symbol           string         `db:"symbol" json:"symbol"`
	TrnType          string         `db:"trn_type" json:"trn_type"`
	Date             types.DateTime `db:"date" json:"date"`
	Qty              int            `db:"qty" json:"qty"`
	Rate             float64        `db:"rate" json:"rate"`
	Amount           float64        `db:"amount" json:"amount"`
	BrokerCommission float64        `db:"broker_commission" json:"broker_commission"`
	NepseCommission  float64        `db:"nepse_commission" json:"nepse_commission"`
	SeboCommission   float64        `db:"sebo_commission" json:"sebo_commission"`
	DpCharge         float64        `db:"dp_charge" json:"dp_charge"`
}

type CorporateActionBonus struct {
	Symbol        string         `db:"symbol" json:"symbol"`
	BookCloseDate types.DateTime `db:"book_close_date" json:"book_close_date"`
	BonusPct      float64        `db:"bonus_pct" json:"bonus_pct"`
	ListingDate   types.DateTime `db:"listing_date" json:"listing_date"`
}

type StockHolding struct {
	Symbol         string  `json:"symbol"`
	TotalQty       int     `json:"total_qty"`
	TotalCost      float64 `json:"total_cost"`
	AverageBuyRate float64 `json:"average_buy_rate"`
}

type DayWiseHoldings struct {
	Date     types.DateTime           `json:"date"`
	Holdings map[string]*StockHolding `json:"holdings"`
}

func CurrentHolding(e *core.RequestEvent) error {

	transactions := []DailyTransactionRecord{}

	err := e.App.DB().
		Select("*").
		From("daily_transactions").
		OrderBy("trn_no ASC").
		All(&transactions)

	if err != nil {
		return e.JSON(http.StatusOK, map[string]any{
			"status": "failed",
			"err":    err,
		})
	}

	holdings := make(map[string]*StockHolding)
	var dayWiseHoldings []DayWiseHoldings
	for _, t := range transactions {
		// if symbol not exist then create in map
		if _, exists := holdings[t.Symbol]; !exists {
			holdings[t.Symbol] = &StockHolding{Symbol: t.Symbol}
		}
		// get holding for that stocks
		h := holdings[t.Symbol]

		switch t.TrnType {
		case "buy":
			buyCost := (float64(t.Qty) * t.Rate) + t.BrokerCommission + t.NepseCommission + t.SeboCommission + t.DpCharge
			h.TotalQty += t.Qty
			h.TotalCost += buyCost
			if h.TotalQty > 0 {
				h.AverageBuyRate = h.TotalCost / float64(h.TotalQty)
			}
		case "sell":
			h.TotalQty -= t.Qty
		}

		dayWiseHoldings = append(dayWiseHoldings, DayWiseHoldings{Date: t.Date, Holdings: holdings})

	}

	return e.JSON(http.StatusOK, map[string]any{
		"status":   "success",
		"holdings": dayWiseHoldings,
	})
}
