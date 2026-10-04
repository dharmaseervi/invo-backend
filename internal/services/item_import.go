package services

import (
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Importing a product catalogue from a spreadsheet.
//
// The job is nine tenths telling someone what is wrong with their file, and doing it
// before anything is written. A shop's catalogue is exported from whatever they used
// before, so the columns are never the ones we would have chosen, the prices carry ₹
// signs and thousands separators, and a few rows are quietly broken. An import that
// writes half a catalogue and then fails is worse than one that refuses: the first
// pass only reads, reports, and asks.

// ImportColumn is a field an item can be imported into.
type ImportColumn string

const (
	ColName     ImportColumn = "name"
	ColSKU      ImportColumn = "sku"
	ColHSN      ImportColumn = "hsn_code"
	ColUnit     ImportColumn = "unit"
	ColPrice    ImportColumn = "price"
	ColCost     ImportColumn = "cost_price"
	ColQuantity ImportColumn = "quantity"
	ColTaxRate  ImportColumn = "tax_rate"
	ColLowStock ImportColumn = "low_stock_alert"
	ColDesc     ImportColumn = "description"
)

// headerAliases are the column names seen in real exports, lowercased and stripped of
// anything but letters and digits. Tally, Vyapar, Excel templates and hand-made sheets
// all name these differently, and asking somebody to rename their columns before they
// can try the app is a poor welcome.
var headerAliases = map[string]ImportColumn{
	"name": ColName, "itemname": ColName, "productname": ColName, "product": ColName,
	"item": ColName, "description1": ColName, "particulars": ColName,

	"sku": ColSKU, "code": ColSKU, "itemcode": ColSKU, "productcode": ColSKU,
	"barcode": ColSKU, "aliascode": ColSKU,

	"hsn": ColHSN, "hsncode": ColHSN, "hsnsac": ColHSN, "hsnsaccode": ColHSN,

	"unit": ColUnit, "uom": ColUnit, "unitofmeasure": ColUnit, "baseunit": ColUnit,

	"price": ColPrice, "sellingprice": ColPrice, "saleprice": ColPrice, "rate": ColPrice,
	"mrp": ColPrice, "sellingrate": ColPrice, "unitprice": ColPrice,

	"costprice": ColCost, "cost": ColCost, "purchaseprice": ColCost, "purchaserate": ColCost,
	"buyingprice": ColCost,

	"quantity": ColQuantity, "qty": ColQuantity, "stock": ColQuantity, "openingstock": ColQuantity,
	"instock": ColQuantity, "closingstock": ColQuantity, "currentstock": ColQuantity,

	"taxrate": ColTaxRate, "gst": ColTaxRate, "gstrate": ColTaxRate, "tax": ColTaxRate,
	"gstpercent": ColTaxRate, "taxpercent": ColTaxRate,

	"lowstockalert": ColLowStock, "lowstock": ColLowStock, "reorderlevel": ColLowStock,
	"minstock": ColLowStock, "reorderpoint": ColLowStock,

	"description": ColDesc, "notes": ColDesc, "remarks": ColDesc,
}

// ImportItem is one row's worth of product, after parsing.
type ImportItem struct {
	Name          string  `json:"name"`
	SKU           string  `json:"sku"`
	HSNCode       string  `json:"hsn_code"`
	Unit          string  `json:"unit"`
	Description   string  `json:"description"`
	Price         float64 `json:"price"`
	CostPrice     float64 `json:"cost_price"`
	Quantity      int     `json:"quantity"`
	TaxRate       float64 `json:"tax_rate"`
	LowStockAlert int     `json:"low_stock_alert"`
}

// ImportDuplicate is an existing product this row appears to be.
type ImportDuplicate struct {
	ItemID int `json:"item_id"`
	// "sku" or "name": which one matched, because an SKU match is certain and a name
	// match is a guess worth showing differently.
	MatchedOn string  `json:"matched_on"`
	Name      string  `json:"name"`
	SKU       string  `json:"sku"`
	Price     float64 `json:"price"`
	Quantity  int     `json:"quantity"`
}

// ImportRow is a row as the preview screen shows it.
type ImportRow struct {
	// Line in the file, counting the header as line 1, so an error can be found in the
	// spreadsheet rather than in a list the person cannot map back.
	Line int        `json:"line"`
	Item ImportItem `json:"item"`
	// Why this row cannot be imported. Empty means it can.
	Errors []string `json:"errors"`
	// Things worth saying that do not stop the import, e.g. a GST rate that is not one
	// of the usual slabs.
	Warnings  []string         `json:"warnings"`
	Duplicate *ImportDuplicate `json:"duplicate,omitempty"`
}

// ImportPreview is the whole answer to "what is in this file?".
type ImportPreview struct {
	// The file's own header, so the app can show which column fed which field.
	Headers []string `json:"headers"`
	// Header name -> field, as detected. A caller can send a corrected mapping back.
	Mapping map[string]ImportColumn `json:"mapping"`
	// Fields no column was found for. name and price being here is why an import is
	// refused outright.
	Missing []ImportColumn `json:"missing"`
	Rows    []ImportRow    `json:"rows"`
	Summary ImportSummary  `json:"summary"`
}

type ImportSummary struct {
	Total      int `json:"total"`
	Valid      int `json:"valid"`
	Invalid    int `json:"invalid"`
	Duplicates int `json:"duplicates"`
}

// maxImportRows caps a single import. A spreadsheet with more rows than this is a
// migration rather than an import, and holding it all in memory to answer one request
// is how a server falls over.
const maxImportRows = 5000

// ImportInputError is a problem with the file itself, as opposed to a row: the caller
// is told what to fix rather than given a 500.
type ImportInputError struct{ Message string }

func (e ImportInputError) Error() string { return e.Message }

// normaliseHeader reduces a column name to letters and digits, lowercased, so
// "Selling Price (₹)", "selling_price" and "SELLING PRICE" are one thing.
func normaliseHeader(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ParseItemCSV reads the file and reports what it found, writing nothing.
//
// override lets the caller correct the detected mapping, keyed by the header exactly as
// it appears in the file.
func ParseItemCSV(
	db *sql.DB,
	companyID int,
	content string,
	override map[string]ImportColumn,
) (ImportPreview, error) {
	var out ImportPreview

	if !utf8.ValidString(content) {
		return out, ImportInputError{"That file isn't readable as text. Export it as CSV (UTF-8) and try again."}
	}
	// A UTF-8 BOM would otherwise become part of the first header's name, so the first
	// column never matches anything.
	content = strings.TrimPrefix(content, "\ufeff")

	reader := csv.NewReader(strings.NewReader(content))
	// Rows with a stray extra comma are a row-level problem, not a reason to reject the
	// file, so the reader is told not to enforce a width.
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	headers, err := reader.Read()
	if err == io.EOF {
		return out, ImportInputError{"That file is empty."}
	}
	if err != nil {
		return out, ImportInputError{"That file could not be read as a CSV: " + err.Error()}
	}
	for i := range headers {
		headers[i] = strings.TrimSpace(headers[i])
	}
	out.Headers = headers

	// Which column feeds which field.
	out.Mapping = map[string]ImportColumn{}
	for _, h := range headers {
		if col, ok := override[h]; ok {
			if col != "" {
				out.Mapping[h] = col
			}
			continue
		}
		if col, ok := headerAliases[normaliseHeader(h)]; ok {
			// First column wins: a sheet with both "Rate" and "Selling Price" should not
			// have the later one quietly replace the earlier.
			if !hasColumn(out.Mapping, col) {
				out.Mapping[h] = col
			}
		}
	}

	for _, needed := range []ImportColumn{ColName, ColPrice} {
		if !hasColumn(out.Mapping, needed) {
			out.Missing = append(out.Missing, needed)
		}
	}

	// Existing products, to recognise what is already there. One query rather than one
	// per row: a 2000-row file would otherwise be 2000 round trips.
	existing, err := loadExistingItems(db, companyID)
	if err != nil {
		return out, err
	}

	index := map[string]int{} // header -> position
	for i, h := range headers {
		index[h] = i
	}

	seenSKU := map[string]int{}  // sku -> line, for duplicates inside the file itself
	seenName := map[string]int{} // name -> line

	line := 1
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		line++
		if err != nil {
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				out.Rows = append(out.Rows, ImportRow{
					Line:   line,
					Errors: []string{"This line could not be read: " + pe.Err.Error()},
				})
				continue
			}
			return out, ImportInputError{"That file could not be read as a CSV: " + err.Error()}
		}

		if isBlankRecord(record) {
			continue
		}
		if len(out.Rows) >= maxImportRows {
			return out, ImportInputError{fmt.Sprintf(
				"That file has more than %d rows. Split it and import in parts.", maxImportRows)}
		}

		row := buildRow(line, record, headers, index, out.Mapping)

		// Duplicates within the file, which a shop's export does contain — the same
		// product listed twice with different stock. Importing both creates two
		// products with one SKU, which the database then refuses halfway through.
		key := strings.ToLower(strings.TrimSpace(row.Item.SKU))
		if key != "" {
			if first, ok := seenSKU[key]; ok {
				row.Errors = append(row.Errors,
					fmt.Sprintf("The same SKU is on line %d of this file.", first))
			} else {
				seenSKU[key] = line
			}
		}
		nameKey := strings.ToLower(strings.TrimSpace(row.Item.Name))
		if nameKey != "" && key == "" {
			if first, ok := seenName[nameKey]; ok {
				row.Warnings = append(row.Warnings,
					fmt.Sprintf("The same name is on line %d of this file.", first))
			} else {
				seenName[nameKey] = line
			}
		}

		row.Duplicate = existing.match(row.Item)
		out.Rows = append(out.Rows, row)
	}

	for _, r := range out.Rows {
		out.Summary.Total++
		if len(r.Errors) > 0 {
			out.Summary.Invalid++
		} else {
			out.Summary.Valid++
		}
		if r.Duplicate != nil {
			out.Summary.Duplicates++
		}
	}

	return out, nil
}

func hasColumn(m map[string]ImportColumn, col ImportColumn) bool {
	for _, v := range m {
		if v == col {
			return true
		}
	}
	return false
}

func isBlankRecord(record []string) bool {
	for _, v := range record {
		if strings.TrimSpace(v) != "" {
			return false
		}
	}
	return true
}

// buildRow turns one line into an item, collecting everything wrong with it rather than
// stopping at the first problem: a person fixing a spreadsheet wants the whole list.
func buildRow(
	line int,
	record, headers []string,
	index map[string]int,
	mapping map[string]ImportColumn,
) ImportRow {
	row := ImportRow{Line: line}

	value := func(col ImportColumn) string {
		for h, c := range mapping {
			if c != col {
				continue
			}
			if i, ok := index[h]; ok && i < len(record) {
				return strings.TrimSpace(record[i])
			}
		}
		return ""
	}

	row.Item.Name = value(ColName)
	if row.Item.Name == "" {
		row.Errors = append(row.Errors, "A name is required.")
	}
	row.Item.SKU = value(ColSKU)
	row.Item.HSNCode = value(ColHSN)
	if hsn := row.Item.HSNCode; hsn != "" {
		if !isDigits(hsn) || len(hsn) > 8 {
			row.Errors = append(row.Errors, "An HSN code is up to 8 digits.")
		}
	}
	row.Item.Unit = value(ColUnit)
	row.Item.Description = value(ColDesc)

	if v, err := parseMoney(value(ColPrice)); err != nil {
		row.Errors = append(row.Errors, "Selling price: "+err.Error())
	} else {
		row.Item.Price = v
	}
	if v, err := parseMoney(value(ColCost)); err != nil {
		row.Errors = append(row.Errors, "Cost price: "+err.Error())
	} else {
		row.Item.CostPrice = v
	}
	if v, err := parseCount(value(ColQuantity)); err != nil {
		row.Errors = append(row.Errors, "Stock: "+err.Error())
	} else {
		row.Item.Quantity = v
	}
	if v, err := parseCount(value(ColLowStock)); err != nil {
		row.Errors = append(row.Errors, "Low-stock alert: "+err.Error())
	} else {
		row.Item.LowStockAlert = v
	}
	if v, err := parseMoney(strings.TrimSuffix(value(ColTaxRate), "%")); err != nil {
		row.Errors = append(row.Errors, "GST rate: "+err.Error())
	} else {
		row.Item.TaxRate = v
		if v > 100 {
			row.Errors = append(row.Errors, "GST rate: a percentage cannot be above 100.")
		} else if !isGSTSlab(v) {
			// Not refused: a shop may have a reason, and refusing would block the
			// import over a figure that is merely unusual.
			row.Warnings = append(row.Warnings,
				fmt.Sprintf("%g%% isn't a usual GST slab (0, 5, 12, 18, 28).", v))
		}
	}

	return row
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func isGSTSlab(v float64) bool {
	for _, slab := range []float64{0, 0.25, 3, 5, 12, 18, 28} {
		if v == slab {
			return true
		}
	}
	return false
}

// parseMoney accepts what spreadsheets actually contain: ₹ and Rs prefixes, thousands
// separators, spaces, and blanks meaning zero.
func parseMoney(s string) (float64, error) {
	s = cleanNumber(s)
	if s == "" {
		return 0, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, errors.New("\"" + s + "\" is not a number.")
	}
	if v < 0 {
		return 0, errors.New("cannot be negative.")
	}
	return v, nil
}

// parseCount is the same for whole numbers. "12.0" is accepted and "12.5" is not:
// stock is counted in whole units, and a spreadsheet writing 12 as 12.0 is ordinary.
func parseCount(s string) (int, error) {
	s = cleanNumber(s)
	if s == "" {
		return 0, nil
	}
	if v, err := strconv.Atoi(s); err == nil {
		if v < 0 {
			return 0, errors.New("cannot be negative.")
		}
		return v, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, errors.New("\"" + s + "\" is not a number.")
	}
	if f < 0 {
		return 0, errors.New("cannot be negative.")
	}
	if f != float64(int(f)) {
		return 0, errors.New("is counted in whole units, so \"" + s + "\" can't be stored.")
	}
	return int(f), nil
}

func cleanNumber(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "₹")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "Rs."), "Rs")
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, " ", "")
	return strings.TrimSpace(s)
}

// existingItems is a company's catalogue, indexed for matching.
type existingItems struct {
	bySKU  map[string]ImportDuplicate
	byName map[string]ImportDuplicate
}

func (e existingItems) match(item ImportItem) *ImportDuplicate {
	if sku := strings.ToLower(strings.TrimSpace(item.SKU)); sku != "" {
		if d, ok := e.bySKU[sku]; ok {
			d.MatchedOn = "sku"
			return &d
		}
	}
	if name := strings.ToLower(strings.TrimSpace(item.Name)); name != "" {
		if d, ok := e.byName[name]; ok {
			d.MatchedOn = "name"
			return &d
		}
	}
	return nil
}

func loadExistingItems(db *sql.DB, companyID int) (existingItems, error) {
	out := existingItems{
		bySKU:  map[string]ImportDuplicate{},
		byName: map[string]ImportDuplicate{},
	}

	rows, err := db.Query(`
		SELECT id, name, COALESCE(sku, ''), price, quantity
		FROM items WHERE company_id = $1
	`, companyID)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	for rows.Next() {
		var d ImportDuplicate
		if err := rows.Scan(&d.ItemID, &d.Name, &d.SKU, &d.Price, &d.Quantity); err != nil {
			return out, err
		}
		if sku := strings.ToLower(strings.TrimSpace(d.SKU)); sku != "" {
			out.bySKU[sku] = d
		}
		if name := strings.ToLower(strings.TrimSpace(d.Name)); name != "" {
			// First one wins, so a catalogue with two products of the same name points
			// at the same one every time rather than whichever the database returned last.
			if _, ok := out.byName[name]; !ok {
				out.byName[name] = d
			}
		}
	}
	return out, rows.Err()
}

// ImportAction is what to do with one row, decided by the person reviewing the preview.
type ImportAction struct {
	Line int `json:"line"`
	// "create", "update" or "skip".
	Action string `json:"action"`
	// The existing product to update. Required for "update".
	ItemID int        `json:"item_id"`
	Item   ImportItem `json:"item"`
}

// ImportResult is what happened.
type ImportResult struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
	// Rows that could not be written, with the reason, by line number.
	Failed []ImportFailure `json:"failed"`
}

type ImportFailure struct {
	Line   int    `json:"line"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// ApplyItemImport writes the chosen rows.
//
// All of it or none of it: a half-written catalogue leaves somebody guessing which
// products made it, and the obvious next move — import the file again — then creates
// duplicates of the half that worked. Any row that fails rolls the lot back and the
// result says which line and why.
func ApplyItemImport(
	db *sql.DB,
	companyID, userID int,
	actions []ImportAction,
) (ImportResult, error) {
	var out ImportResult

	if len(actions) == 0 {
		return out, ImportInputError{"Nothing was selected to import."}
	}
	if len(actions) > maxImportRows {
		return out, ImportInputError{fmt.Sprintf("More than %d rows at once.", maxImportRows)}
	}

	tx, err := db.Begin()
	if err != nil {
		return out, err
	}
	defer tx.Rollback()

	for _, a := range actions {
		switch a.Action {
		case "skip":
			out.Skipped++
			continue

		case "create":
			if strings.TrimSpace(a.Item.Name) == "" {
				out.Failed = append(out.Failed, ImportFailure{a.Line, a.Item.Name, "A name is required."})
				return out, failedImport(out)
			}
			_, err := tx.Exec(`
				INSERT INTO items
					(name, sku, unit, description, cost_price, price, quantity,
					 low_stock_alert, tax_rate, hsn_code, company_id, user_id)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
			`,
				a.Item.Name, nullIfBlank(a.Item.SKU), a.Item.Unit, a.Item.Description,
				a.Item.CostPrice, a.Item.Price, a.Item.Quantity, a.Item.LowStockAlert,
				a.Item.TaxRate, nullIfBlank(a.Item.HSNCode), companyID, userID,
			)
			if err != nil {
				reason := "Could not be saved."
				if isDuplicateSKUErr(err) {
					reason = "That SKU is already used by another product."
				}
				out.Failed = append(out.Failed, ImportFailure{a.Line, a.Item.Name, reason})
				return out, failedImport(out)
			}
			out.Created++

		case "update":
			// Scoped to the company: an item id from the request is not trusted to
			// belong to this business just because the company id does.
			res, err := tx.Exec(`
				UPDATE items SET
					name = $1, sku = $2, unit = $3, description = $4,
					cost_price = $5, price = $6, quantity = $7,
					low_stock_alert = $8, tax_rate = $9, hsn_code = $10,
					updated_at = NOW()
				WHERE id = $11 AND company_id = $12
			`,
				a.Item.Name, nullIfBlank(a.Item.SKU), a.Item.Unit, a.Item.Description,
				a.Item.CostPrice, a.Item.Price, a.Item.Quantity, a.Item.LowStockAlert,
				a.Item.TaxRate, nullIfBlank(a.Item.HSNCode), a.ItemID, companyID,
			)
			if err != nil {
				reason := "Could not be saved."
				if isDuplicateSKUErr(err) {
					reason = "That SKU is already used by another product."
				}
				out.Failed = append(out.Failed, ImportFailure{a.Line, a.Item.Name, reason})
				return out, failedImport(out)
			}
			if n, _ := res.RowsAffected(); n == 0 {
				out.Failed = append(out.Failed, ImportFailure{
					a.Line, a.Item.Name, "That product no longer exists.",
				})
				return out, failedImport(out)
			}
			out.Updated++

		default:
			return out, ImportInputError{"Each row needs an action of create, update or skip."}
		}
	}

	if err := tx.Commit(); err != nil {
		return out, err
	}
	return out, nil
}

// failedImport carries the row that stopped the import, so the handler can report it
// without the caller having to guess from a bare error.
type ImportRowError struct{ Result ImportResult }

func (e ImportRowError) Error() string {
	if len(e.Result.Failed) == 0 {
		return "import failed"
	}
	f := e.Result.Failed[0]
	return fmt.Sprintf("line %d: %s", f.Line, f.Reason)
}

func failedImport(r ImportResult) error { return ImportRowError{Result: r} }

func nullIfBlank(s string) interface{} {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.TrimSpace(s)
}

// isDuplicateSKUErr recognises the unique-constraint violation on sku, which is the one
// database error worth turning into a sentence a shopkeeper can act on.
func isDuplicateSKUErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key") && strings.Contains(msg, "sku")
}
