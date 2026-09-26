package handlers

import "github.com/fjaeckel/ninerlog-api/internal/service"

// logbookScope returns the licence logbook scope resolver.
func (h *APIHandler) logbookScope() *service.LogbookScope {
	var credit service.CreditScoper
	if h.currencyService != nil {
		credit = h.currencyService
	}
	return service.NewLogbookScope(h.licenseService, h.classRatingService, h.aircraftService, credit)
}
