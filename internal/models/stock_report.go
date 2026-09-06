package models

// StockReportItem is one row in a stock report's item lists (low stock, out of
// stock, top value) — always carries enough to render a line without another lookup.
type StockReportItem struct {
	ID            int     `json:"id"`
	Name          string  `json:"name"`
	Quantity      int     `json:"quantity"`
	CostPrice     float64 `json:"cost_price"`
	Price         float64 `json:"price"`
	StockValue    float64 `json:"stock_value"` // cost_price * quantity
	LowStockAlert int     `json:"low_stock_alert"`
}

type StockReportResponse struct {
	AsOf             string            `json:"as_of"`
	TotalItems       int               `json:"total_items"`
	TotalStockUnits  int               `json:"total_stock_units"`
	TotalCostValue   float64           `json:"total_cost_value"`
	TotalRetailValue float64           `json:"total_retail_value"`
	PotentialProfit  float64           `json:"potential_profit"`
	LowStockCount    int               `json:"low_stock_count"`
	OutOfStockCount  int               `json:"out_of_stock_count"`
	LowStockItems    []StockReportItem `json:"low_stock_items"`
	OutOfStockItems  []StockReportItem `json:"out_of_stock_items"`
	TopValueItems    []StockReportItem `json:"top_value_items"`
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
