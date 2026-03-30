package views

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	"github.com/xuri/excelize/v2"
)

const (
	ColTransactionNo = 1
	ColShareType     = 2
	ColStock         = 3
	ColSector        = 4
	ColBrokerNo      = 5
	ColClientName    = 6
	ColQty           = 9
	ColRate          = 10
	ColAmount        = 11
	ColDateAD        = 13
	ColDateBS        = 14
	ColType          = 15
	ColNepseCom      = 16
	ColSeboCom       = 17
	ColBrokerCom     = 18
	DataStartRow     = 22
)

type DailyTransactions struct {
	TransactionNo    string  `json:"transaction_no"`
	ShareType        string  `json:"share_type"`
	Stock            string  `json:"stock"`
	Sector           string  `json:"sector"`
	BrokerNo         string  `json:"broker_no"`
	ClientName       string  `json:"client_name"`
	Qty              int     `json:"qty"`
	Rate             float64 `json:"rate"`
	Amount           float64 `json:"amount"`
	DateAD           string  `json:"date_ad"`
	DateBS           string  `json:"date_bs"`
	Type             string  `json:"type"`
	NepseCommission  float64 `json:"nepse_commission"`
	SeboCommission   float64 `json:"sebo_commission"`
	BrokerCommission float64 `json:"broker_commission"`
}

func parseMoney(s string) float64 {
	clean := strings.ReplaceAll(s, ",", "")
	val, _ := strconv.ParseFloat(clean, 64)
	return val
}

func GetDateFromTrn(trn string) string {
	if len(trn) < 8 {
		return ""
	}
	year := trn[0:4]
	month := trn[4:6]
	day := trn[6:8]
	return fmt.Sprintf("%s-%s-%s", year, month, day)
}

func toInt(s string) int {
	clean := strings.ReplaceAll(s, ",", "")
	if strings.Contains(clean, ".") {
		clean = strings.Split(clean, ".")[0]
	}
	val, _ := strconv.Atoi(clean)
	return val
}

func ImportDailyTransactions(e *core.RequestEvent) error {

	file, header, err := e.Request.FormFile("file")
	if err != nil {
		return e.BadRequestError("Failed to retrieve file", err)
	}
	defer file.Close()

	f, err := excelize.OpenReader(file)
	if err != nil {
		return e.BadRequestError("Invalid Excel file", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return e.BadRequestError("Excel file has no sheets", nil)
	}

	firstSheet := sheets[0]
	rows, err := f.GetRows(firstSheet)
	if err != nil {
		return e.BadRequestError(fmt.Sprintf("Could not read sheet: %s", firstSheet), err)
	}

	var transactions []DailyTransactions

	for i := 23; i < len(rows)-1; i++ {
		row := rows[i]
		qty := toInt(row[ColQty])

		trn := DailyTransactions{
			TransactionNo:    row[ColTransactionNo],
			ShareType:        row[ColShareType],
			Stock:            row[ColStock],
			Sector:           row[ColSector],
			BrokerNo:         row[ColBrokerNo],
			ClientName:       row[ColClientName],
			Qty:              qty,
			Rate:             parseMoney(row[ColRate]),
			Amount:           parseMoney(row[ColAmount]),
			DateAD:           row[ColDateAD],
			DateBS:           row[ColDateBS],
			Type:             row[ColType],
			NepseCommission:  parseMoney(row[ColNepseCom]),
			SeboCommission:   parseMoney(row[ColSeboCom]),
			BrokerCommission: parseMoney(row[ColBrokerCom]),
		}
		transactions = append(transactions, trn)
	}

	dpMap := make(map[string]int)
	for _, t := range transactions {
		key := fmt.Sprintf("%s_%s_%s", strings.ToLower(t.Type), t.Stock, GetDateFromTrn(t.TransactionNo))
		dpMap[key]++
	}

	collection, _ := e.App.FindCollectionByNameOrId("daily_transactions")
	err = e.App.RunInTransaction(func(txApp core.App) error {
		for _, t := range transactions {
			record := core.NewRecord(collection)

			var dpCharge float64 = 0.0

			// dp charge calc logic
			typeKey := strings.ToLower(t.Type)
			key := fmt.Sprintf("%s_%s_%s", typeKey, t.Stock, GetDateFromTrn(t.TransactionNo))
			count := dpMap[key]
			if count > 0 {
				dpCharge = 25.0 / float64(count)
				dpCharge = math.Round((25.0/float64(count))*100) / 100
			}

			record.Set("dp_charge", dpCharge)
			record.Set("trn_no", t.TransactionNo)
			record.Set("client_name", t.ClientName)
			record.Set("symbol", t.Stock)
			record.Set("trn_type", strings.ToLower(t.Type))
			record.Set("date", GetDateFromTrn(t.TransactionNo))
			record.Set("qty", t.Qty)
			record.Set("rate", t.Rate)
			record.Set("broker_commission", t.BrokerCommission)
			record.Set("nepse_commission", t.NepseCommission)
			record.Set("sebo_commission", t.SeboCommission)

			if err := txApp.Save(record); err != nil {
				if strings.Contains(err.Error(), "UNIQUE constraint failed") {
					continue // ignore if unique constraint
				}
				return err // rollback
			}
		}
		return nil
	})

	if err != nil {
		return e.JSON(http.StatusOK, map[string]any{
			"status": "failed",
			"error":  err,
		})
	}

	return e.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"file":   header.Filename,
		"rows":   transactions,
	})
}
