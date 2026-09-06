package services

import (
	"database/sql"
	"sort"
	"time"

	"invo-server/internal/models"
)

// GenerateStockReport summarizes current inventory for a company: total value at
// cost and at retail, potential profit if everything sold, and which items need
// attention (low stock / out of stock) or represent the most tied-up capital.
func GenerateStockReport(db *sql.DB, companyID int64) (*models.StockReportResponse, error) {
	rows, err := db.Query(`
		SELECT id, name, quantity, COALESCE(cost_price, 0), price, COALESCE(low_stock_alert, 0)
		FROM items
		WHERE company_id = $1
	`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var all []models.StockReportItem
	var totalCostValue, totalRetailValue float64
	var totalStockUnits int

	for rows.Next() {
		var item models.StockReportItem
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Quantity,
			&item.CostPrice, &item.Price, &item.LowStockAlert,
		); err != nil {
			return nil, err
		}
		item.StockValue = item.CostPrice * float64(item.Quantity)

		totalCostValue += item.StockValue
		totalRetailValue += item.Price * float64(item.Quantity)
		totalStockUnits += item.Quantity

		all = append(all, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	lowStock := make([]models.StockReportItem, 0)
	outOfStock := make([]models.StockReportItem, 0)
	for _, item := range all {
		if item.Quantity <= 0 {
			outOfStock = append(outOfStock, item)
		} else if item.LowStockAlert > 0 && item.Quantity <= item.LowStockAlert {
			lowStock = append(lowStock, item)
		}
	}

	topValue := make([]models.StockReportItem, len(all))
	copy(topValue, all)
	sort.Slice(topValue, func(i, j int) bool { return topValue[i].StockValue > topValue[j].StockValue })
	if len(topValue) > 10 {
		topValue = topValue[:10]
	}

	return &models.StockReportResponse{
		AsOf:             time.Now().Format("2006-01-02"),
		TotalItems:       len(all),
		TotalStockUnits:  totalStockUnits,
		TotalCostValue:   totalCostValue,
		TotalRetailValue: totalRetailValue,
		PotentialProfit:  totalRetailValue - totalCostValue,
		LowStockCount:    len(lowStock),
		OutOfStockCount:  len(outOfStock),
		LowStockItems:    lowStock,
		OutOfStockItems:  outOfStock,
		TopValueItems:    topValue,
	}, nil
}
