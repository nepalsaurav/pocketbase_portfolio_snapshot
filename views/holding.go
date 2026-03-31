package views

import (
	_ "embed" // Required for go:embed
	"encoding/json"
	"math"
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
	holdingsMap := make(map[string][]HoldingLot)

	for _, ev := range events {
		h, exists := holdingsMap[ev.Symbol]
		if !exists {
			h = []HoldingLot{}
		}

		var runningQty float64
		for _, lot := range h {
			runningQty += lot.Qty
		}

		switch ev.Source {
		case "Transaction":
			var meta TransactionMeta
			if err := json.Unmarshal([]byte(ev.Metadata), &meta); err != nil {
				continue
			}

			switch meta.TrnType {
			case "buy":
				lotCost := (meta.Qty * meta.Rate) + meta.BrokerCommission + meta.SeboCommission + meta.NepseCommission + meta.DpCharge
				h = append(h, HoldingLot{
					Symbol:      ev.Symbol,
					Qty:         meta.Qty,
					TotalCost:   lotCost,
					WACC:        lotCost / meta.Qty,
					HoldingType: "Buy",
				})
			case "sell":
				sellQty := meta.Qty
				for sellQty > 0 && len(h) > 0 {
					if h[0].Qty <= sellQty {
						// Consume the entire oldest lot
						sellQty -= h[0].Qty
						h = h[1:] // Remove the first lot from the slice
					} else {
						// Consume a partial amount from the oldest lot
						h[0].Qty -= sellQty
						h[0].TotalCost -= sellQty * h[0].WACC // Deduct cost based on its specific WACC
						sellQty = 0
					}
				}
			}

			ledger = append(ledger, Ledger{
				ID:         ev.ID,
				Date:       ev.EventDate,
				ClientName: clientName,
				Symbol:     ev.Symbol,
				TrnType:    meta.TrnType,
				Qty:        meta.Qty,
				Rate:       meta.Rate,
				Holding:    h,
			})

		case "Bonus":
			var meta BonusMeta
			if err := json.Unmarshal([]byte(ev.Metadata), &meta); err != nil {
				continue
			}

			bonusQty := math.Round(runningQty * (meta.BonusPct / 100))
			if bonusQty <= 0 {
				continue
			}
			if meta.ListingDate == "" {
				h = append(h, HoldingLot{
					Symbol:      ev.Symbol,
					Qty:         bonusQty,
					TotalCost:   bonusQty * 100,
					WACC:        100,
					HoldingType: "Provisonal Bonus",
				})
			}
			ledger = append(ledger, Ledger{
				ID:         ev.ID,
				Date:       ev.EventDate,
				ClientName: clientName,
				Symbol:     ev.Symbol,
				TrnType:    "Provisonal Bonus",
				Qty:        bonusQty,
				Rate:       100,
				Holding:    h,
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

		holdingsMap[ev.Symbol] = h
	}

	err := bulkCreateLedger(ledger, e.App)
	if err != nil {
		return jsonError(e, err)
	}

	err = createHoldingsMap(holdingsMap, e.App, clientName)
	if err != nil {
		return jsonError(e, err)
	}

	return e.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"ledger": ledger,
	})
}

func bulkCreateLedger(ledger []Ledger, app core.App) error {
	return app.RunInTransaction(func(txApp core.App) error {
		collection, err := txApp.FindCollectionByNameOrId("ledgers")
		if err != nil {
			return err
		}
		for _, l := range ledger {
			var record *core.Record
			record, err = txApp.FindRecordById("ledgers", l.ID)

			if err != nil {
				record = core.NewRecord(collection)
				record.Set("id", l.ID)
			}
			record.Set("client_name", l.ClientName)
			record.Set("date", l.Date)
			record.Set("trn_type", l.TrnType)
			record.Set("symbol", l.Symbol)
			record.Set("qty", l.Qty)
			record.Set("rate", l.Rate)
			record.Set("holding", l.Holding)
			if err := txApp.Save(record); err != nil {
				return err
			}
		}
		return nil
	})
}

func createHoldingsMap(holdingsMap map[string][]HoldingLot, app core.App, clientName string) error {
	return app.RunInTransaction(func(txApp core.App) error {
		collection, err := txApp.FindCollectionByNameOrId("holdings")
		record, err := txApp.
			FindFirstRecordByFilter(
				"holdings",
				"client_name = {:client_name}",
				dbx.Params{"client_name": clientName})

		if err != nil {
			record = core.NewRecord(collection)
		}
		record.Set("client_name", clientName)
		record.Set("data", holdingsMap)
		if err := txApp.Save(record); err != nil {
			return err
		}
		return nil
	})
}
