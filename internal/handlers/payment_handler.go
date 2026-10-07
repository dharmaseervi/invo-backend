package handlers

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"

	database "invo-server/internal/db"
	"invo-server/internal/models"
	"invo-server/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

type PaymentHandler struct {
	db      *database.Database
	service *services.PaymentService
}

func NewPaymentHandler(db *database.Database, service *services.PaymentService) *PaymentHandler {
	return &PaymentHandler{
		db:      db,
		service: service,
	}
}

// POST /api/v1/payments
func (h *PaymentHandler) RecordPayment(c *gin.Context) {
	var req models.PaymentRequestDTO
	userID := c.GetInt("user_id")

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	tx, err := h.db.DB.BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tx failed"})
		return
	}
	defer tx.Rollback()

	// auth
	var companyID int64
	err = tx.QueryRowContext(c.Request.Context(),
		`
		SELECT c.id
		FROM clients cl
		JOIN companies c ON c.id = cl.company_id
		WHERE cl.id = $1 AND c.id IN (SELECT company_id FROM companies_for_user($2))
	`, req.ClientID, userID).Scan(&companyID)

	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "unauthorized"})
		return
	}

	err = h.service.RecordPaymentTx(c.Request.Context(),
		tx, companyID, req.ClientID, req)
	if err != nil {
		var input services.PaymentInputError
		if errors.As(err, &input) {
			c.JSON(http.StatusBadRequest, gin.H{"error": input.Msg})
			return
		}
		log.Println("failed to record payment:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to record payment"})
		return
	}

	if err = tx.Commit(); err != nil {
		log.Println("failed to commit payment:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record payment"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Payment recorded successfully",
	})
}

// GetPayments lists a company's payments, most recent first.
// GET /api/v1/companies/:companyId/payments
//
// POST was previously the only payment route, so a recorded payment could not be
// viewed or verified afterwards — a mis-keyed amount was invisible until someone
// noticed a client's balance was wrong. Each row carries the invoices it was applied
// to, since a payment settling three invoices is otherwise impossible to interpret.
func (h *PaymentHandler) GetPayments(c *gin.Context) {
	companyID, err := strconv.ParseInt(c.Param("companyId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid company id"})
		return
	}
	userID := c.GetInt("user_id")

	owned, err := companyBelongsToUser(c.Request.Context(),
		h.db.DB, companyID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify company"})
		return
	}
	if !owned {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
		return
	}

	limit := clampPageSize(mustAtoi(c.DefaultQuery("limit", "50")), 50)
	offset := mustAtoi(c.DefaultQuery("offset", "0"))

	rows, err := h.db.DB.QueryContext(c.Request.Context(),
		`
		SELECT p.id, p.client_id, COALESCE(cl.name, ''), p.amount,
		       COALESCE(p.payment_method, ''), COALESCE(p.reference, ''), COALESCE(p.notes, ''),
		       TO_CHAR(p.payment_date, 'YYYY-MM-DD'),
		       TO_CHAR(p.created_at, 'YYYY-MM-DD HH24:MI'),
		       COALESCE((
		           SELECT string_agg(i.invoice_number, ', ' ORDER BY i.invoice_number)
		           FROM payment_allocations pa
		           JOIN invoices i ON i.id = pa.invoice_id
		           WHERE pa.payment_id = p.id
		       ), ''),
		       COALESCE(p.status, 'recorded'), COALESCE(p.reversal_reason, ''),
		       COALESCE(p.unapplied_amount, 0)
		FROM payments p
		LEFT JOIN clients cl ON cl.id = p.client_id
		WHERE p.company_id = $1
		ORDER BY p.payment_date DESC, p.id DESC
		LIMIT $2 OFFSET $3
	`, companyID, limit, offset)
	if err != nil {
		log.Println("failed to fetch payments:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch payments"})
		return
	}
	defer rows.Close()

	payments := []models.PaymentHistoryRow{}
	for rows.Next() {
		var p models.PaymentHistoryRow
		if err := rows.Scan(
			&p.ID, &p.ClientID, &p.ClientName, &p.Amount,
			&p.PaymentMethod, &p.Reference, &p.Notes,
			&p.PaymentDate, &p.CreatedAt, &p.AppliedTo,
			&p.Status, &p.ReversalReason, &p.UnappliedAmount,
		); err != nil {
			log.Println("failed to scan payment:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch payments"})
			return
		}
		payments = append(payments, p)
	}
	if err := rows.Err(); err != nil {
		log.Println("failed to read payments:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch payments"})
		return
	}

	// The invoices each payment settled, for the whole page in one query rather than
	// one per payment: moving a payment needs to show what it is on now, and a screen
	// that costs fifty round trips to open is a screen nobody opens twice.
	if err := attachAllocations(c.Request.Context(),
		h.db.DB, payments); err != nil {
		log.Println("failed to fetch payment allocations:", err)
	}

	c.JSON(http.StatusOK, gin.H{"payments": payments})
}

// attachAllocations fills in each payment's invoice breakdown.
func attachAllocations(ctx context.Context, db *sql.DB, payments []models.PaymentHistoryRow) error {
	if len(payments) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(payments))
	for _, p := range payments {
		ids = append(ids, p.ID)
	}

	rows, err := db.QueryContext(ctx,
		`
		SELECT pa.payment_id, pa.invoice_id, COALESCE(i.invoice_number, ''), pa.amount
		FROM payment_allocations pa
		LEFT JOIN invoices i ON i.id = pa.invoice_id
		WHERE pa.payment_id = ANY($1)
		ORDER BY pa.payment_id, pa.id
	`, pq.Array(ids))
	if err != nil {
		return err
	}
	defer rows.Close()

	byPayment := map[int64][]models.PaymentAllocationRow{}
	for rows.Next() {
		var paymentID int64
		var a models.PaymentAllocationRow
		if err := rows.Scan(&paymentID, &a.InvoiceID, &a.InvoiceNumber, &a.Amount); err != nil {
			return err
		}
		byPayment[paymentID] = append(byPayment[paymentID], a)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for i := range payments {
		payments[i].Allocations = byPayment[payments[i].ID]
	}
	return nil
}
