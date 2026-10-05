package handlers

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"invo-server/internal/models"
	"invo-server/internal/services"

	"github.com/gin-gonic/gin"
)

// companyFromQuery reads and authorises company_id for the correction routes, which
// take it as a query parameter the way the rest of the payment routes do.
func (h *PaymentHandler) companyFromQuery(c *gin.Context) (int64, bool) {
	companyID, err := strconv.ParseInt(c.Query("company_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id is required"})
		return 0, false
	}
	owned, err := companyBelongsToUser(h.db.DB, companyID, c.GetInt("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify company"})
		return 0, false
	}
	if !owned {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
		return 0, false
	}
	return companyID, true
}

// ReversePayment undoes a payment recorded by mistake.
//
// POST /api/v1/payments/:id/reverse?company_id=1  {"reason": "entered twice"}
func (h *PaymentHandler) ReversePayment(c *gin.Context) {
	paymentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payment id"})
		return
	}
	companyID, ok := h.companyFromQuery(c)
	if !ok {
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)

	if err := h.service.ReversePayment(companyID, paymentID, req.Reason); err != nil {
		var input services.PaymentInputError
		if errors.As(err, &input) {
			c.JSON(http.StatusBadRequest, gin.H{"error": input.Msg})
			return
		}
		log.Println("failed to reverse payment:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reverse that payment"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Payment reversed"})
}

// ReallocatePayment moves a payment onto different invoices.
//
// PUT /api/v1/payments/:id/allocations?company_id=1
//
//	{"allocations": [{"invoice_id": 7, "amount": 500}]}
//
// An empty list leaves the whole payment on account, which is how a payment put against
// the wrong invoice becomes an advance the customer can use elsewhere.
func (h *PaymentHandler) ReallocatePayment(c *gin.Context) {
	paymentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payment id"})
		return
	}
	companyID, ok := h.companyFromQuery(c)
	if !ok {
		return
	}

	var req struct {
		Allocations []models.PaymentAllocationDTO `json:"allocations"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	if err := h.service.ReallocatePayment(companyID, paymentID, req.Allocations); err != nil {
		var input services.PaymentInputError
		if errors.As(err, &input) {
			c.JSON(http.StatusBadRequest, gin.H{"error": input.Msg})
			return
		}
		log.Println("failed to reallocate payment:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to move that payment"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Payment moved"})
}

// RecordRefund gives money back to a customer.
//
// POST /api/v1/refunds?company_id=1
//
//	{"client_id": 3, "credit_note_id": 12, "amount": 500, "method": "UPI"}
func (h *PaymentHandler) RecordRefund(c *gin.Context) {
	companyID, ok := h.companyFromQuery(c)
	if !ok {
		return
	}

	var req services.RefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	id, err := h.service.RecordRefund(companyID, req)
	if err != nil {
		var input services.PaymentInputError
		if errors.As(err, &input) {
			c.JSON(http.StatusBadRequest, gin.H{"error": input.Msg})
			return
		}
		log.Println("failed to record refund:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record that refund"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Refund recorded", "refund_id": id})
}

// GetRefunds lists refunds, newest first.
//
// GET /api/v1/refunds?company_id=1&client_id=&limit=&offset=
func (h *PaymentHandler) GetRefunds(c *gin.Context) {
	companyID, ok := h.companyFromQuery(c)
	if !ok {
		return
	}

	query := `
		SELECT r.id, r.client_id, COALESCE(cl.name, ''), r.credit_note_id,
		       COALESCE(cn.credit_number, ''), r.amount, r.method,
		       COALESCE(r.reference, ''), COALESCE(r.notes, ''), r.refund_date
		FROM refunds r
		JOIN clients cl ON cl.id = r.client_id
		LEFT JOIN credit_notes cn ON cn.id = r.credit_note_id
		WHERE r.company_id = $1
	`
	args := []interface{}{companyID}
	if v := c.Query("client_id"); v != "" {
		if clientID, err := strconv.ParseInt(v, 10, 64); err == nil {
			query += " AND r.client_id = $2"
			args = append(args, clientID)
		}
	}
	query += " ORDER BY r.refund_date DESC, r.id DESC"
	if limit := clampPageSize(mustAtoi(c.Query("limit")), 0); limit > 0 {
		query += " LIMIT $" + strconv.Itoa(len(args)+1) + " OFFSET $" + strconv.Itoa(len(args)+2)
		args = append(args, limit, mustAtoi(c.Query("offset")))
	}

	rows, err := h.db.DB.Query(query, args...)
	if err != nil {
		log.Println("failed to fetch refunds:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch refunds"})
		return
	}
	defer rows.Close()

	out := []gin.H{}
	for rows.Next() {
		var (
			id, clientID             int64
			creditNoteID             *int64
			clientName, creditNumber string
			amount                   float64
			method, reference, notes string
			refundDate               string
		)
		if err := rows.Scan(
			&id, &clientID, &clientName, &creditNoteID, &creditNumber,
			&amount, &method, &reference, &notes, &refundDate,
		); err != nil {
			log.Println("failed to scan refund:", err)
			continue
		}
		out = append(out, gin.H{
			"id": id, "client_id": clientID, "client_name": clientName,
			"credit_note_id": creditNoteID, "credit_number": creditNumber,
			"amount": amount, "method": method, "reference": reference,
			"notes": notes, "refund_date": refundDate,
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": out})
}
