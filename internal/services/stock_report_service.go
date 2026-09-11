package services

import (
	"database/sql"
	"sort"
	"strings"
	"time"

	"invo-server/internal/models"
)

const uncategorisedLabel = "Uncategorised"

// GenerateStockReport summarizes current inventory for a company: total value at
// cost and at retail, potential profit if everything sold, a per-category breakdown,
// and the full item list so the client can filter, sort and export without paging.
func GenerateStockReport(db *sql.DB, companyID int64) (*models.StockReportResponse, error) {
	rows, err := db.Query(`
		SELECT i.id, i.name, COALESCE(i.sku, ''), i.category_id,
		       COALESCE(c.name, ''), COALESCE(i.unit, ''),
		       COALESCE(i.quantity, 0), COALESCE(i.cost_price, 0), i.price,
		       COALESCE(i.low_stock_alert, 0)
		FROM items i
		LEFT JOIN categories c ON c.id = i.category_id
		WHERE i.company_id = $1
		ORDER BY i.name
	`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	all := make([]models.StockReportItem, 0)
	var totalCostValue, totalRetailValue float64
	var totalStockUnits int

	for rows.Next() {
		var item models.StockReportItem
		var categoryID sql.NullInt64
		var categoryName string

		if err := rows.Scan(
			&item.ID, &item.Name, &item.SKU, &categoryID, &categoryName, &item.Unit,
			&item.Quantity, &item.CostPrice, &item.Price, &item.LowStockAlert,
		); err != nil {
			return nil, err
		}

		if categoryID.Valid {
			id := int(categoryID.Int64)
			item.CategoryID = &id
		}
		// An item can have no category, and a category row can exist with a blank
		// name — both land in the same bucket so nothing vanishes from the breakdown.
		if strings.TrimSpace(categoryName) == "" {
			item.CategoryName = uncategorisedLabel
			item.CategoryID = nil
		} else {
			item.CategoryName = categoryName
		}

		item.StockValue = item.CostPrice * float64(item.Quantity)
		item.RetailValue = item.Price * float64(item.Quantity)
		item.Status = stockStatus(item.Quantity, item.LowStockAlert)

		totalCostValue += item.StockValue
		totalRetailValue += item.RetailValue
		totalStockUnits += item.Quantity

		all = append(all, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	lowStock := make([]models.StockReportItem, 0)
	outOfStock := make([]models.StockReportItem, 0)
	for _, item := range all {
		switch item.Status {
		case "out":
			outOfStock = append(outOfStock, item)
		case "low":
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
		Items:            all,
		Categories:       summarizeByCategory(all, totalCostValue),
		LowStockItems:    lowStock,
		OutOfStockItems:  outOfStock,
		TopValueItems:    topValue,
	}, nil
}

func stockStatus(quantity, lowStockAlert int) string {
	switch {
	case quantity <= 0:
		return "out"
	case lowStockAlert > 0 && quantity <= lowStockAlert:
		return "low"
	default:
		return "in"
	}
}

// summarizeByCategory groups items by category, ordered by how much money each has
// tied up (biggest first) so the top of the list is the part worth acting on.
func summarizeByCategory(items []models.StockReportItem, totalCostValue float64) []models.StockCategorySummary {
	index := make(map[string]int)
	summaries := make([]models.StockCategorySummary, 0)

	for _, item := range items {
		pos, seen := index[item.CategoryName]
		if !seen {
			summaries = append(summaries, models.StockCategorySummary{
				CategoryID:   item.CategoryID,
				CategoryName: item.CategoryName,
			})
			pos = len(summaries) - 1
			index[item.CategoryName] = pos
		}

		s := &summaries[pos]
		s.ItemCount++
		s.TotalUnits += item.Quantity
		s.CostValue += item.StockValue
		s.RetailValue += item.RetailValue
		switch item.Status {
		case "out":
			s.OutOfStockCount++
		case "low":
			s.LowStockCount++
		}
	}

	for i := range summaries {
		summaries[i].PotentialProfit = summaries[i].RetailValue - summaries[i].CostValue
		if totalCostValue > 0 {
			summaries[i].ShareOfValue = summaries[i].CostValue / totalCostValue * 100
		}
	}

	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].CostValue != summaries[j].CostValue {
			return summaries[i].CostValue > summaries[j].CostValue
		}
		return summaries[i].CategoryName < summaries[j].CategoryName
	})

	return summaries
}
