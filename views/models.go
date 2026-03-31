package views

type CorporateActionsBonus struct {
	ID            string  `json:"id"`
	Symbol        string  `json:"symbol"`
	BookCloseDate string  `json:"book_close_date"`
	BonusPct      float64 `json:"bonus_pct"`
	ListingDate   string  `json:"listing_date,omitempty"` // Optional field
	Created       string  `json:"created"`
	Updated       string  `json:"updated"`
}

type CorporateActionsCashDividend struct {
	ID              string  `json:"id"`
	Symbol          string  `json:"symbol"`
	BookCloseDate   string  `json:"book_close_date"`
	CashDividendPct float64 `json:"cash_dividend_pct"`
	ListingDate     string  `json:"listing_date"`
	Created         string  `json:"created"`
	Updated         string  `json:"updated"`
}

type CorporateActionsRightShare struct {
	ID              string `json:"id"`
	Symbol          string `json:"symbol"`
	BookCloseDate   string `json:"book_close_date"`
	ListingDate     string `json:"listing_date"`
	RightShareRatio string `json:"right_share_ratio"` // Text type in schema
	Created         string `json:"created"`
	Updated         string `json:"updated"`
}

type TransactionsModel struct {
	ID               string  `json:"id"`
	TrnNo            string  `json:"trn_no"`
	ClientName       string  `json:"client_name"`
	Symbol           string  `json:"symbol"`
	TrnType          string  `json:"trn_type"` // "buy" or "sell"
	Date             string  `json:"date"`
	Qty              float64 `json:"qty"`
	Rate             float64 `json:"rate"`
	BrokerCommission float64 `json:"broker_commission"`
	NepseCommission  float64 `json:"nepse_commission"`
	SeboCommission   float64 `json:"sebo_commission"`
	DpCharge         float64 `json:"dp_charge"`
	Created          string  `json:"created"`
	Updated          string  `json:"updated"`
}

type Holding struct {
	Symbol           string  `json:"symbol"`
	RunningQty       float64 `json:"running_qty"`
	TotalCost        float64 `json:"total_cost"`
	AverageCost      float64 `json:"average_cost"`
	ProvisionalBonus float64 `json:"provisional_bonus"`
}

type Ledger struct {
	Date    string  `json:"date"`
	TrnType string  `json:"trn_type"`
	Qty     float64 `json:"qty"`
	Rate    float64 `json:"rate"`
	Holding Holding `json:"holding"`
}

// CombineDate represents the shared columns from the UNION SQL
type CombineDate struct {
	ID             string `db:"id" json:"id"`
	Symbol         string `db:"symbol" json:"symbol"`
	EventDate      string `db:"event_date" json:"event_date"`
	Source         string `db:"source" json:"source"`
	CollectionName string `db:"collection_name" json:"collection_name"`
	Created        string `db:"created" json:"created"`
	Updated        string `db:"updated" json:"updated"`
	Metadata       string `db:"metadata" json:"metadata"` // JSON string containing source-specific data
}

// Specific Metadata Structs for strict typing (No Pointers Needed!)
type TransactionMeta struct {
	TrnNo            string  `json:"trn_no"`
	ClientName       string  `json:"client_name"`
	TrnType          string  `json:"trn_type"`
	Qty              float64 `json:"qty"`
	Rate             float64 `json:"rate"`
	BrokerCommission float64 `json:"broker_commission"`
	NepseCommission  float64 `json:"nepse_commission"`
	SeboCommission   float64 `json:"sebo_commission"`
	DpCharge         float64 `json:"dp_charge"`
}

type BonusMeta struct {
	BonusPct    float64 `json:"bonus_pct"`
	ListingDate string  `json:"listing_date"`
}

type DividendMeta struct {
	CashDividendPct float64 `json:"cash_dividend_pct"`
	ListingDate     string  `json:"listing_date"`
}

type RightShareMeta struct {
	RightShareRatio string `json:"right_share_ratio"`
	ListingDate     string `json:"listing_date"`
}
