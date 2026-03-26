package wallet

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
	r.GET("/users/:id/wallet", h.GetBalance)
	r.POST("/wallets/topup", h.TopUp)
}

// GET /users/:id/wallet
func (h *Handler) GetBalance(c *gin.Context) {
	userID := c.Param("id")
	w, err := h.svc.GetBalance(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "wallet not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"user_id":  w.UserID,
		"balance":  w.Balance,
		"currency": w.Currency,
	})
}

// POST /wallets/topup
// Body: { "user_id": "...", "amount": 1000, "idempotency_key": "..." }
func (h *Handler) TopUp(c *gin.Context) {
	var req struct {
		UserID         string `json:"user_id" binding:"required"`
		Amount         int64  `json:"amount" binding:"required,gt=0"`
		IdempotencyKey string `json:"idempotency_key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.svc.TopUp(c.Request.Context(), req.UserID, req.Amount, req.IdempotencyKey); err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "wallet not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
