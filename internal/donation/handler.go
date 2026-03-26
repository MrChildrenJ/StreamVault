package donation

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/MrChildrenJ/streamvault/internal/wallet"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	r.POST("/donations", h.Donate)
}

// POST /donations
// Body: { "from_user_id": "...", "streamer_id": "...", "amount_cents": 500, "message": "...", "idempotency_key": "..." }
func (h *Handler) Donate(c *gin.Context) {
	var req struct {
		FromUserID     string `json:"from_user_id" binding:"required"`
		StreamerID     string `json:"streamer_id" binding:"required"`
		AmountCents    int64  `json:"amount_cents" binding:"required,gt=0"`
		Message        string `json:"message"`
		IdempotencyKey string `json:"idempotency_key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := h.svc.Donate(c.Request.Context(), req.FromUserID, req.StreamerID, req.AmountCents, req.Message, req.IdempotencyKey)
	if err != nil {
		status, msg := mapError(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func mapError(err error) (int, string) {
	switch {
	case errors.Is(err, wallet.ErrInsufficientFunds):
		return http.StatusUnprocessableEntity, "insufficient funds"
	case errors.Is(err, wallet.ErrMaxRetriesExceeded):
		return http.StatusConflict, "too much concurrent activity, please retry"
	default:
		return http.StatusInternalServerError, "internal error"
	}
}
