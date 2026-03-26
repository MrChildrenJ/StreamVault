package bits

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
	r.GET("/users/:id/bits", h.GetBalance)
	r.POST("/bits/purchase", h.Purchase)
	r.POST("/bits/cheer", h.Cheer)
}

// GET /users/:id/bits
func (h *Handler) GetBalance(c *gin.Context) {
	b, err := h.svc.GetBalance(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "bits balance not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user_id": b.UserID, "balance": b.Balance})
}

// POST /bits/purchase
// Body: { "user_id": "...", "bits": 100, "paid_cents": 140, "idempotency_key": "..." }
func (h *Handler) Purchase(c *gin.Context) {
	var req struct {
		UserID         string `json:"user_id" binding:"required"`
		Bits           int64  `json:"bits" binding:"required,gt=0"`
		PaidCents      int64  `json:"paid_cents" binding:"required,gt=0"`
		IdempotencyKey string `json:"idempotency_key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.Purchase(c.Request.Context(), req.UserID, req.Bits, req.PaidCents, req.IdempotencyKey); err != nil {
		status, msg := mapError(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// POST /bits/cheer
// Body: { "user_id": "...", "streamer_id": "...", "bits": 100, "idempotency_key": "..." }
func (h *Handler) Cheer(c *gin.Context) {
	var req struct {
		UserID         string `json:"user_id" binding:"required"`
		StreamerID     string `json:"streamer_id" binding:"required"`
		Bits           int64  `json:"bits" binding:"required,gt=0"`
		IdempotencyKey string `json:"idempotency_key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.Cheer(c.Request.Context(), req.UserID, req.StreamerID, req.Bits, req.IdempotencyKey); err != nil {
		status, msg := mapError(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func mapError(err error) (int, string) {
	switch {
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound, "bits balance not found"
	case errors.Is(err, ErrInsufficientBits):
		return http.StatusUnprocessableEntity, "insufficient bits"
	case errors.Is(err, ErrMaxRetriesExceeded):
		return http.StatusConflict, "too much concurrent activity, please retry"
	default:
		return http.StatusInternalServerError, "internal error"
	}
}
