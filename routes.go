package main

import (
	"net/http"
	"portfolio_snapshot/views"

	"github.com/pocketbase/pocketbase/core"
)

func initRoutes(se *core.ServeEvent) {
	se.Router.GET("/ping", func(e *core.RequestEvent) error {
		return e.String(http.StatusOK, "Pong")
	})

	se.Router.POST("/import_daily_transactions", func(e *core.RequestEvent) error {
		return views.ImportDailyTransactions(e)
	})

	se.Router.GET("/current_holding", func(e *core.RequestEvent) error {
		return views.CurrentHolding(e)
	})

	se.Router.GET("/sync_corporate_action", func(e *core.RequestEvent) error {
		return views.SyncCorporateAction(e)
	})
}
