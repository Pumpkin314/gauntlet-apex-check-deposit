package handlers

import (
	"context"
	"net/http"

	"github.com/apex-checkout/check-deposit/cmd/api/middleware"
)

// CorrespondentMetric holds a per-correspondent metric.
type CorrespondentMetric struct {
	CorrespondentID string  `json:"correspondent_id"`
	Rate            float64 `json:"rate,omitempty"`
	Total           int     `json:"total,omitempty"`
	Rejected        int     `json:"rejected,omitempty"`
	Completed       int     `json:"completed,omitempty"`
	Returned        int     `json:"returned,omitempty"`
	Amount          float64 `json:"amount,omitempty"`
}

// InvestorMetric holds a per-investor metric.
type InvestorMetric struct {
	AccountID string  `json:"account_id"`
	Amount    float64 `json:"amount"`
}

// RiskQuerier provides risk dashboard metrics.
type RiskQuerier interface {
	RejectionRate(ctx context.Context) ([]CorrespondentMetric, error)
	FloatExposure(ctx context.Context) ([]CorrespondentMetric, error)
	ReturnRate(ctx context.Context) ([]CorrespondentMetric, error)
	TopInvestorsByFloat(ctx context.Context, limit int) ([]InvestorMetric, error)
	AvgProcessingTimeSecs(ctx context.Context) (float64, error)
}

// RiskHandler handles the risk dashboard endpoint.
type RiskHandler struct {
	Querier RiskQuerier
}

// Dashboard handles GET /admin/risk-dashboard.
func (h *RiskHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	role := middleware.RoleFromContext(ctx)
	if role != "admin" && role != "apex_admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin access required"})
		return
	}

	rejectionRate, _ := h.Querier.RejectionRate(ctx)
	floatExposure, _ := h.Querier.FloatExposure(ctx)
	returnRate, _ := h.Querier.ReturnRate(ctx)
	topInvestors, _ := h.Querier.TopInvestorsByFloat(ctx, 10)
	avgTime, _ := h.Querier.AvgProcessingTimeSecs(ctx)

	if rejectionRate == nil {
		rejectionRate = []CorrespondentMetric{}
	}
	if floatExposure == nil {
		floatExposure = []CorrespondentMetric{}
	}
	if returnRate == nil {
		returnRate = []CorrespondentMetric{}
	}
	if topInvestors == nil {
		topInvestors = []InvestorMetric{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"rejection_rate":          rejectionRate,
		"float_exposure":          floatExposure,
		"return_rate":             returnRate,
		"top_investors":           topInvestors,
		"avg_processing_time_secs": avgTime,
	})
}
