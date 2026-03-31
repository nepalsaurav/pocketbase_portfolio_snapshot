package views

import (
	_ "embed" // Required for go:embed
	"encoding/json"
	"net/http"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

//go:embed query/combine_data.sql
var combineDateSQL string

// jsonError standardizes error responses and handles standard Go errors correctly
func jsonError(e *core.RequestEvent, msg any) error {
	// Extract the error string if the message is a standard Go error
	if err, ok := msg.(error); ok {
		msg = err.Error()
	}

	return e.JSON(http.StatusBadRequest, map[string]any{
		"status":  "failed",
		"message": msg,
	})
}

func CurrentHolding(e *core.RequestEvent) error {
	clientName := e.Request.URL.Query().Get("client_name")
	if clientName == "" {
		return jsonError(e, "client_name query parameter is required")
	}

	var events []CombineDate
	if err := e.App.DB().NewQuery(combineDateSQL).Bind(dbx.Params{"client_name": clientName}).All(&events); err != nil {
		return jsonError(e, err) // Will now properly output the DB error text if one occurs
	}

	var ledger []Ledger
	holdingsMap := make(map[string]*Holding)

	for _, ev := range events {
		h, exists := holdingsMap[ev.Symbol]
		if !exists {
			h = &Holding{Symbol: ev.Symbol}
			holdingsMap[ev.Symbol] = h
		}

		switch ev.Source {
		case "Transaction":
			var meta TransactionMeta
			if err := json.Unmarshal([]byte(ev.Metadata), &meta); err != nil {
				continue
			}

			switch meta.TrnType {
			case "buy":
				h.RunningQty += meta.Qty
				h.TotalCost += (meta.Qty * meta.Rate) + meta.BrokerCommission + meta.SeboCommission + meta.NepseCommission + meta.DpCharge
				h.AverageCost = h.TotalCost / h.RunningQty
			case "sell":
				h.RunningQty -= meta.Qty
			}

			ledger = append(ledger, Ledger{
				Date:    ev.EventDate,
				TrnType: meta.TrnType,
				Qty:     meta.Qty,
				Rate:    meta.Rate,
				Holding: *h,
			})

		case "Bonus":
			var meta BonusMeta
			if err := json.Unmarshal([]byte(ev.Metadata), &meta); err != nil {
				continue
			}

			bonusQty := h.RunningQty * (meta.BonusPct / 100)
			if bonusQty <= 0 {
				continue
			}

			h.RunningQty += bonusQty
			h.TotalCost += bonusQty * 100
			h.AverageCost = h.TotalCost / h.RunningQty

			if meta.ListingDate == "" {
				h.ProvisionalBonus += bonusQty
			}

			ledger = append(ledger, Ledger{
				Date:    ev.EventDate,
				TrnType: "Bonus",
				Qty:     bonusQty,
				Rate:    100,
				Holding: *h,
			})

		case "Right Share":
			var meta RightShareMeta
			if err := json.Unmarshal([]byte(ev.Metadata), &meta); err != nil {
				continue
			}
			// Parse logic for meta.RightShareRatio goes here

		case "Dividend":
			var meta DividendMeta
			if err := json.Unmarshal([]byte(ev.Metadata), &meta); err != nil {
				continue
			}
			// Cash dividend logic using meta.CashDividendPct goes here
		}
	}

	return e.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"ledger": ledger,
	})
}
