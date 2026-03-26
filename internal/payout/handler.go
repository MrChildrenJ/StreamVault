package payout

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	r.POST("/payouts", h.RequestPayout)
}

// POST /payouts
// Body: { "streamer_id": "...", "amount_cents": 5000, "idempotency_key": "..." }
func (h *Handler) RequestPayout(c *gin.Context) {
	var req struct {
		StreamerID     string `json:"streamer_id" binding:"required"`
		AmountCents    int64  `json:"amount_cents" binding:"required,gt=0"`
		IdempotencyKey string `json:"idempotency_key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := h.svc.RequestPayout(c.Request.Context(), req.StreamerID, req.AmountCents, req.IdempotencyKey)
	if err != nil {
		if errors.Is(err, ErrInsufficientRevenue) {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "insufficient revenue balance"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "pending"})
}
