package services

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"os"
	"strings"
	"testing"

	_ "github.com/lib/pq"
	"github.com/xuri/excelize/v2"
)

// A workbook shaped like the price lists shops actually send: Tally-ish column names,
// a row with trailing blanks, and a second sheet holding something else entirely.
func buildWorkbook(t *testing.T) []byte {
	t.Helper()

	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	rows := [][]interface{}{
		{"Particulars", "Alias Code", "HSN/SAC", "Base Unit", "Selling Rate", "Closing Stock"},
		{"Excel Tile 600x600", "XL-600", "69072100", "Box", 1250.50, 40},
		{"Excel Tile 800x800", "XL-800", "69072100", "Box", 2450, 12},
		// Trailing cells left empty, which Excel returns as a short row.
		{"Excel Spacer 2mm", "XL-SP2"},
	}
	for i, row := range rows {
		cell, err := excelize.CoordinatesToCellName(1, i+1)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.SetSheetRow(sheet, cell, &row); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := f.NewSheet("Last year"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue("Last year", "A1", "Old prices nobody wants imported"); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestXLSXToCSVReadsTheFirstSheet(t *testing.T) {
	data := buildWorkbook(t)

	if !LooksLikeXLSX(data) {
		t.Fatal("a workbook was not recognised as one")
	}

	out, err := XLSXToCSV(data)
	if err != nil {
		t.Fatalf("converting the workbook failed: %v", err)
	}

	records, err := csv.NewReader(strings.NewReader(out)).ReadAll()
	if err != nil {
		t.Fatalf("the conversion did not produce readable CSV: %v", err)
	}

	if len(records) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(records))
	}

	// Every row is the width of the header. A short row left short would have the
	// parser read each field after the gap from the wrong column.
	for i, row := range records {
		if len(row) != 6 {
			t.Errorf("row %d has %d cells, expected 6 — short rows must be padded", i, len(row))
		}
	}

	if records[0][0] != "Particulars" || records[0][4] != "Selling Rate" {
		t.Errorf("headers came through wrong: %v", records[0])
	}
	if records[1][4] != "1250.5" {
		t.Errorf("a decimal price came through as %q", records[1][4])
	}
	if records[3][0] != "Excel Spacer 2mm" || records[3][2] != "" {
		t.Errorf("the short row came through wrong: %v", records[3])
	}

	// The second sheet is somebody's old prices, not part of the catalogue.
	if strings.Contains(out, "Old prices") {
		t.Error("the second sheet was imported; only the first should be")
	}
}

func TestXLSXToCSVFeedsTheSameParser(t *testing.T) {
	// The parser reads the company's existing items to spot duplicates, so this one
	// needs a database. Set INVO_TEST_DB to a throwaway Postgres to run it.
	dsn := os.Getenv("INVO_TEST_DB")
	if dsn == "" {
		t.Skip("set INVO_TEST_DB to run the workbook-through-the-parser check")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skip("no database:", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Skip("no database:", err)
	}

	out, err := XLSXToCSV(buildWorkbook(t))
	if err != nil {
		t.Fatal(err)
	}

	// The whole point of converting rather than writing a second parser: the column
	// detection that already works for CSV has to work on a workbook unchanged.
	preview, err := ParseItemCSV(context.Background(),
		db, 0, out, nil)
	if err != nil {
		t.Fatalf("the converted sheet did not parse: %v", err)
	}

	if preview.Mapping["Particulars"] != ColName {
		t.Errorf("'Particulars' was not recognised as the name column: %v", preview.Mapping)
	}
	if preview.Mapping["Selling Rate"] != ColPrice {
		t.Errorf("'Selling Rate' was not recognised as the price: %v", preview.Mapping)
	}
	if preview.Mapping["Closing Stock"] != ColQuantity {
		t.Errorf("'Closing Stock' was not recognised as the quantity: %v", preview.Mapping)
	}
}

func TestLooksLikeXLSXRejectsText(t *testing.T) {
	// A CSV must not be mistaken for a workbook, or somebody's plain file would be
	// refused for being the wrong format when it is the right one.
	if LooksLikeXLSX([]byte("Name,Price\nTile,100\n")) {
		t.Error("a CSV was taken for an Excel workbook")
	}
	if LooksLikeXLSX(nil) {
		t.Error("nothing at all was taken for an Excel workbook")
	}
}
