package models

// StockReportItem is one row in a stock report — always carries enough to render a
// line, filter it by category/status, and export it to CSV without another lookup.
type StockReportItem struct {
	ID            int     `json:"id"`
	Name          string  `json:"name"`
	SKU           string  `json:"sku"`
	CategoryID    *int    `json:"category_id"`
	CategoryName  string  `json:"category_name"`
	Unit          string  `json:"unit"`
	Quantity      int     `json:"quantity"`
	CostPrice     float64 `json:"cost_price"`
	Price         float64 `json:"price"`
	StockValue    float64 `json:"stock_value"`  // cost_price * quantity
	RetailValue   float64 `json:"retail_value"` // price * quantity
	LowStockAlert int     `json:"low_stock_alert"`
	Status        string  `json:"status"` // in | low | out
}

// StockCategorySummary rolls the item list up per category, so a shop owner can see
// which part of the catalogue is holding their money rather than only a single total.
type StockCategorySummary struct {
	CategoryID      *int    `json:"category_id"` // nil == the synthetic "Uncategorised" bucket
	CategoryName    string  `json:"category_name"`
	ItemCount       int     `json:"item_count"`
	TotalUnits      int     `json:"total_units"`
	CostValue       float64 `json:"cost_value"`
	RetailValue     float64 `json:"retail_value"`
	PotentialProfit float64 `json:"potential_profit"`
	LowStockCount   int     `json:"low_stock_count"`
	OutOfStockCount int     `json:"out_of_stock_count"`
	ShareOfValue    float64 `json:"share_of_value"` // percent of the company's total cost value
}

type StockReportResponse struct {
	AsOf             string                 `json:"as_of"`
	TotalItems       int                    `json:"total_items"`
	TotalStockUnits  int                    `json:"total_stock_units"`
	TotalCostValue   float64                `json:"total_cost_value"`
	TotalRetailValue float64                `json:"total_retail_value"`
	PotentialProfit  float64                `json:"potential_profit"`
	LowStockCount    int                    `json:"low_stock_count"`
	OutOfStockCount  int                    `json:"out_of_stock_count"`
	Items            []StockReportItem      `json:"items"`
	Categories       []StockCategorySummary `json:"categories"`
	LowStockItems    []StockReportItem      `json:"low_stock_items"`
	OutOfStockItems  []StockReportItem      `json:"out_of_stock_items"`
	TopValueItems    []StockReportItem      `json:"top_value_items"`
}

// StockMovement is one entry in an item's audit trail — every restock, sale-driven
// deduction, and manual adjustment is logged so stock changes are never silent.
type StockMovement struct {
	ID               int     `json:"id"`
	ItemID           int     `json:"item_id"`
	MovementType     string  `json:"movement_type"` // restock | sale | adjustment | initial
	QuantityChange   int     `json:"quantity_change"`
	PreviousQuantity int     `json:"previous_quantity"`
	NewQuantity      int     `json:"new_quantity"`
	Reference        *string `json:"reference,omitempty"`
	Note             *string `json:"note,omitempty"`
	CreatedAt        string  `json:"created_at"`
}

type RestockRequestDTO struct {
	Quantity  int     `json:"quantity" binding:"required,gt=0"`
	Reference *string `json:"reference"`
	Note      *string `json:"note"`
}
