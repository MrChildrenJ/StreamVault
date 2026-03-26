package subscription

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
	r.POST("/subscriptions", h.Subscribe)
	r.POST("/subscriptions/gift", h.GiftSubscription)
}

// POST /subscriptions
// Body: { "subscriber_id": "...", "tier_id": "...", "idempotency_key": "..." }
func (h *Handler) Subscribe(c *gin.Context) {
	var req struct {
		SubscriberID   string `json:"subscriber_id" binding:"required"`
		TierID         string `json:"tier_id" binding:"required"`
		IdempotencyKey string `json:"idempotency_key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sub, err := h.svc.Subscribe(c.Request.Context(), req.SubscriberID, req.TierID, req.IdempotencyKey)
	if err != nil {
		status, msg := mapError(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"subscription_id": sub.ID,
		"expires_at":      sub.ExpiresAt,
	})
}

// POST /subscriptions/gift
// Body: { "gifter_id": "...", "recipient_id": "...", "tier_id": "...", "idempotency_key": "..." }
func (h *Handler) GiftSubscription(c *gin.Context) {
	var req struct {
		GifterID       string `json:"gifter_id" binding:"required"`
		RecipientID    string `json:"recipient_id" binding:"required"`
		TierID         string `json:"tier_id" binding:"required"`
		IdempotencyKey string `json:"idempotency_key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sub, err := h.svc.GiftSubscription(c.Request.Context(), req.GifterID, req.RecipientID, req.TierID, req.IdempotencyKey)
	if err != nil {
		status, msg := mapError(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"subscription_id": sub.ID,
		"expires_at":      sub.ExpiresAt,
	})
}

func mapError(err error) (int, string) {
	switch {
	case errors.Is(err, ErrTierNotFound):
		return http.StatusNotFound, "subscription tier not found"
	case errors.Is(err, wallet.ErrInsufficientFunds):
		return http.StatusUnprocessableEntity, "insufficient funds"
	case errors.Is(err, wallet.ErrMaxRetriesExceeded):
		return http.StatusConflict, "too much concurrent activity, please retry"
	default:
		return http.StatusInternalServerError, "internal error"
	}
}
