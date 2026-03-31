package views

import (
	_ "embed" // Required for go:embed
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

//go:embed query/combine_data.sql
var combineDateSQL string

// jsonError standardizes error responses and handles standard Go errors correctly
func jsonError(e *core.RequestEvent, msg any) error {
	if err, ok := msg.(error); ok {
		msg = err.Error()
	}

	return e.JSON(http.StatusBadRequest, map[string]any{
		"status":  "failed",
		"message": msg,
	})
}

// parseMeta is a generic helper to reduce JSON unmarshaling boilerplate
func parseMeta[T any](data string) (T, error) {
	var meta T
	err := json.Unmarshal([]byte(data), &meta)
	return meta, err
}

func CurrentHolding(e *core.RequestEvent) error {
	clientName := e.Request.URL.Query().Get("client_name")
	if clientName == "" {
		return jsonError(e, "client_name query parameter is required")
	}

	var events []CombineDate
	if err := e.App.DB().NewQuery(combineDateSQL).Bind(dbx.Params{"client_name": clientName}).All(&events); err != nil {
		return jsonError(e, err)
	}

	// Pre-allocate memory for performance based on event count
	ledgers := make([]Ledger, 0, len(events))
	holdingsMap := make(map[string][]HoldingLot)

	for _, ev := range events {
		h := holdingsMap[ev.Symbol]

		var runningQty float64
		for _, lot := range h {
			runningQty += lot.Qty
		}

		var newLedger *Ledger

		switch ev.Source {
		case "Transaction":
			h, newLedger = processTransaction(ev, h, clientName)
		case "Bonus":
			h, newLedger = processBonus(ev, h, clientName, runningQty)
		case "Right Share":
			h, newLedger = processRightShare(ev, h, clientName, runningQty)
		case "Dividend":
			h, newLedger = processDividend(ev, h, clientName, runningQty)
		}

		holdingsMap[ev.Symbol] = h
		if newLedger != nil {
			ledgers = append(ledgers, *newLedger)
		}
	}

	if err := bulkCreateLedger(ledgers, e.App); err != nil {
		return jsonError(e, err)
	}

	if err := createHoldingsMap(holdingsMap, e.App, clientName); err != nil {
		return jsonError(e, err)
	}

	return e.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"ledger": ledgers,
	})
}

func processTransaction(ev CombineDate, h []HoldingLot, clientName string) ([]HoldingLot, *Ledger) {
	meta, err := parseMeta[TransactionMeta](ev.Metadata)
	if err != nil {
		return h, nil
	}

	switch meta.TrnType {
	case "buy":
		return processBuy(ev, meta, h, clientName)
	case "sell":
		return processSell(ev, meta, h, clientName)
	default:
		return h, nil
	}
}

func processBuy(ev CombineDate, meta TransactionMeta, h []HoldingLot, clientName string) ([]HoldingLot, *Ledger) {
	lotCost := (meta.Qty * meta.Rate) + meta.BrokerCommission + meta.SeboCommission + meta.NepseCommission + meta.DpCharge

	h = append(h, HoldingLot{
		Symbol:      ev.Symbol,
		Qty:         meta.Qty,
		TotalCost:   lotCost,
		WACC:        lotCost / meta.Qty,
		HoldingType: "Buy",
		Date:        ev.EventDate,
	})

	return h, &Ledger{
		ID:         ev.ID,
		Date:       ev.EventDate,
		ClientName: clientName,
		Symbol:     ev.Symbol,
		TrnType:    meta.TrnType,
		Qty:        meta.Qty,
		Rate:       meta.Rate,
	}
}

func processSell(ev CombineDate, meta TransactionMeta, h []HoldingLot, clientName string) ([]HoldingLot, *Ledger) {
	sellQty := meta.Qty
	sellDate, _ := time.Parse(time.DateOnly, ev.EventDate)

	totalCommissions := meta.BrokerCommission + meta.SeboCommission + meta.NepseCommission + meta.DpCharge
	netSellRate := ((meta.Qty * meta.Rate) - totalCommissions) / meta.Qty

	var totalCGT, totalProfit float64

	for sellQty > 0 {
		// Fallback: No holdings left in the queue
		if len(h) == 0 {
			profit := (netSellRate - 100) * sellQty
			totalProfit += profit
			if profit > 0 {
				totalCGT += profit * 0.075
			}
			break
		}

		// Always process the oldest lot (FIFO)
		lot := h[0]
		consumedQty := min(sellQty, lot.Qty)

		profit := (netSellRate - lot.WACC) * consumedQty
		totalProfit += profit

		if profit > 0 {
			buyDate, _ := time.Parse(time.DateOnly, lot.Date)
			if sellDate.Before(buyDate.AddDate(0, 6, 0)) {
				totalCGT += profit * 0.075 // Short term
			} else {
				totalCGT += profit * 0.05 // Long term
			}
		}

		// Queue management
		if lot.Qty <= sellQty {
			sellQty -= lot.Qty
			h = h[1:] // Remove the fully consumed lot by slicing
		} else {
			h[0].Qty -= sellQty
			h[0].TotalCost -= sellQty * h[0].WACC
			sellQty = 0
		}
	}

	return h, &Ledger{
		ID:          ev.ID,
		Date:        ev.EventDate,
		ClientName:  clientName,
		Symbol:      ev.Symbol,
		TrnType:     meta.TrnType,
		Qty:         meta.Qty,
		Rate:        meta.Rate,
		CapitalGain: totalProfit,
		CGT:         totalCGT,
	}
}

func processBonus(ev CombineDate, h []HoldingLot, clientName string, runningQty float64) ([]HoldingLot, *Ledger) {
	meta, err := parseMeta[BonusMeta](ev.Metadata)
	if err != nil {
		return h, nil
	}

	bonusQty := math.Round(runningQty * (meta.BonusPct / 100))
	if bonusQty <= 0 {
		return h, nil
	}

	isProvisional := false
	date := ev.EventDate

	switch meta.ListingDate {
	case "":
		isProvisional = true
	default:
		date = meta.ListingDate
	}

	h = append(h, HoldingLot{
		Symbol:        ev.Symbol,
		Qty:           bonusQty,
		TotalCost:     bonusQty * 100,
		WACC:          100,
		HoldingType:   "Bonus",
		IsProvisional: isProvisional,
		Date:          date,
	})

	return h, &Ledger{
		ID:            ev.ID,
		Date:          ev.EventDate,
		ClientName:    clientName,
		Symbol:        ev.Symbol,
		TrnType:       "Bonus",
		Qty:           bonusQty,
		Rate:          100,
		IsProvisional: isProvisional,
	}
}

func processRightShare(ev CombineDate, h []HoldingLot, clientName string, runningQty float64) ([]HoldingLot, *Ledger) {
	meta, err := parseMeta[RightShareMeta](ev.Metadata)
	if err != nil || meta.RightShareRatio == "" {
		return h, nil
	}

	parts := strings.Split(meta.RightShareRatio, ":")
	if len(parts) != 2 {
		return h, nil
	}

	baseRatio, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	rightRatio, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)

	if err1 != nil || err2 != nil || baseRatio <= 0 {
		return h, nil
	}

	rightSharesQty := math.Round(runningQty * (rightRatio / baseRatio))
	if rightSharesQty <= 0 {
		return h, nil
	}

	isProvisional := meta.ListingDate == ""
	costPerShare := 100.0

	h = append(h, HoldingLot{
		Symbol:        ev.Symbol,
		Qty:           rightSharesQty,
		TotalCost:     rightSharesQty * costPerShare,
		WACC:          costPerShare,
		HoldingType:   "Right Share",
		Date:          ev.EventDate,
		IsProvisional: isProvisional,
	})

	return h, &Ledger{
		ID:            ev.ID,
		Date:          ev.EventDate,
		ClientName:    clientName,
		Symbol:        ev.Symbol,
		TrnType:       "Right Share",
		Qty:           rightSharesQty,
		Rate:          costPerShare,
		IsProvisional: isProvisional,
	}
}

func processDividend(ev CombineDate, h []HoldingLot, clientName string, runningQty float64) ([]HoldingLot, *Ledger) {
	meta, err := parseMeta[DividendMeta](ev.Metadata)
	if err != nil || meta.CashDividendPct <= 0 || runningQty <= 0 {
		return h, nil
	}

	return h, &Ledger{
		ID:         ev.ID,
		Date:       ev.EventDate,
		ClientName: clientName,
		Symbol:     ev.Symbol,
		TrnType:    "Cash Dividend",
		Qty:        0,
		Rate:       runningQty * meta.CashDividendPct,
	}
}

func bulkCreateLedger(ledger []Ledger, app core.App) error {
	if len(ledger) == 0 {
		return nil
	}

	return app.RunInTransaction(func(txApp core.App) error {
		// Optimization: Find collection ONLY ONCE outside the loop
		collection, err := txApp.FindCollectionByNameOrId("ledgers")
		if err != nil {
			return err
		}

		for _, l := range ledger {
			record, err := txApp.FindRecordById("ledgers", l.ID)

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
			record.Set("is_provisional", l.IsProvisional)
			record.Set("capital_gain", l.CapitalGain)
			record.Set("cgt", l.CGT)

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
		if err != nil {
			return err
		}

		record, err := txApp.FindFirstRecordByFilter(
			"holdings",
			"client_name = {:client_name}",
			dbx.Params{"client_name": clientName},
		)

		if err != nil {
			record = core.NewRecord(collection)
		}

		record.Set("client_name", clientName)
		record.Set("data", holdingsMap)

		return txApp.Save(record)
	})
}
