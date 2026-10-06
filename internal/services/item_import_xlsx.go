package services

import (
	"bytes"
	"encoding/csv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Importing a catalogue straight from an Excel file.
//
// Every shop that has a price list has it in Excel. Asking somebody to open it, choose
// "Save as", find CSV among the formats and hope the encoding survives is a real reason
// people never get as far as trying the app at all — and the file they eventually send
// is usually the .xlsx anyway.
//
// The sheet is turned into the same rows the CSV path already reads, so everything after
// this — the column detection, the duplicate checking, the preview, the all-or-nothing
// apply — is one piece of code with one set of behaviour, rather than two that drift.

// LooksLikeXLSX reports whether these bytes are an Excel workbook.
//
// By content, not by filename: a file named .xlsx that is really a CSV is common when
// somebody has renamed it, and so is the reverse. A real .xlsx is a zip, and every zip
// starts "PK".
func LooksLikeXLSX(data []byte) bool {
	return len(data) > 1 && data[0] == 'P' && data[1] == 'K'
}

// XLSXToCSV reads the first sheet of a workbook and returns it as CSV text.
//
// The first sheet, because a price list that has been worked on usually has the list on
// sheet one and notes, pivot tables or last year's version behind it. Importing all of
// them would merge a catalogue with whatever else the file happens to carry.
func XLSXToCSV(data []byte) (string, error) {
	file, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return "", ImportInputError{"That file could not be opened as an Excel workbook."}
	}
	defer file.Close()

	sheets := file.GetSheetList()
	if len(sheets) == 0 {
		return "", ImportInputError{"That workbook has no sheets in it."}
	}

	rows, err := file.GetRows(sheets[0])
	if err != nil {
		return "", ImportInputError{"That sheet could not be read: " + err.Error()}
	}
	if len(rows) == 0 {
		return "", ImportInputError{"The first sheet of that workbook is empty."}
	}

	// Excel gives short rows when the trailing cells are blank, so a row with nothing
	// in its last two columns comes back two cells shorter than the header. Padded out
	// here, because a parser reading by column position would otherwise read every
	// field after the gap from the wrong column.
	width := 0
	for _, row := range rows {
		if len(row) > width {
			width = len(row)
		}
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	for _, row := range rows {
		padded := make([]string, width)
		for i := range row {
			padded[i] = strings.TrimSpace(row[i])
		}
		if err := writer.Write(padded); err != nil {
			return "", err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", err
	}

	return buf.String(), nil
}
