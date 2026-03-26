package dashboard

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	repo *Repository
}

func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("/streamers/:id/revenue", h.GetRevenue)
	r.GET("/streamers/:id/transactions", h.ListTransactions)
}

// GET /streamers/:id/revenue
func (h *Handler) GetRevenue(c *gin.Context) {
	s, err := h.repo.GetRevenueSummary(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "no revenue data found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"streamer_id":          s.StreamerID,
		"total_revenue":        s.TotalRevenue,
		"subscription_revenue": s.SubscriptionRevenue,
		"donation_revenue":     s.DonationRevenue,
		"bits_revenue":         s.BitsRevenue,
		"updated_at":           s.UpdatedAt,
	})
}

// GET /streamers/:id/transactions?limit=20&before=<unix_ms>
// Cursor-based pagination: `before` is the created_at (unix milliseconds) of the
// last item from the previous page. Omit for the first page (defaults to now).
func (h *Handler) ListTransactions(c *gin.Context) {
	limit := 20
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}

	before := time.Now()
	if b := c.Query("before"); b != "" {
		if ms, err := strconv.ParseInt(b, 10, 64); err == nil {
			before = time.UnixMilli(ms)
		}
	}

	rows, err := h.repo.ListTransactions(c.Request.Context(), c.Param("id"), limit, before)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// Return the cursor for the next page (created_at of the last item).
	var nextCursor *int64
	if len(rows) == limit {
		ms := rows[len(rows)-1].CreatedAt.UnixMilli()
		nextCursor = &ms
	}

	c.JSON(http.StatusOK, gin.H{
		"transactions": rows,
		"next_cursor":  nextCursor, // nil means no more pages
	})
}
