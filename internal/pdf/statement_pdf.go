package pdf

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/jung-kurt/gofpdf"
)

// A customer's statement of account, as one page they can be sent.
//
// Laid out like the minimal invoice — hairline rules, no fills, figures right-aligned —
// because it comes from the same shop and should look like it did. What it adds is the
// thing a statement is for: an opening balance, every movement in order, and a closing
// figure large enough to be read at a glance across a counter.

// StatementLine is one movement on the account.
type StatementLine struct {
	Date        time.Time
	Description string
	Reference   string
	Debit       float64
	Credit      float64
	Balance     float64
}

// StatementData is everything the page prints.
type StatementData struct {
	CompanyName    string
	CompanyGSTIN   string
	CompanyPhone   string
	CompanyAddress string

	ClientName    string
	ClientPhone   string
	ClientAddress string

	From    time.Time
	To      time.Time
	Opening float64
	Lines   []StatementLine
	Billed  float64
	Paid    float64
	Closing float64
}

type statementGenerator struct {
	pdf  *gofpdf.Fpdf
	data StatementData
}

// GenerateStatementPDF renders a customer's statement of account.
func GenerateStatementPDF(data StatementData) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(0, 0, 0)
	// Room at the foot for the page number, and for the table to break cleanly when a
	// year's trading does not fit on one page.
	pdf.SetAutoPageBreak(true, 18)

	g := &statementGenerator{pdf: pdf, data: data}
	return g.generate()
}

func (g *statementGenerator) generate() ([]byte, error) {
	pdf := g.pdf

	pdf.SetFooterFunc(func() {
		pdf.SetY(-12)
		pdf.SetFont("Helvetica", "", 7)
		pdf.SetTextColor(170, 170, 170)
		pdf.CellFormat(pageW, 5, fmt.Sprintf("Page %d", pdf.PageNo()), "", 0, "C", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
	})

	pdf.AddPage()

	y := g.drawHeader(marginT + 4)
	y = g.drawParties(y)
	y = g.drawSummary(y)
	g.drawTable(y)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (g *statementGenerator) drawHeader(y float64) float64 {
	pdf := g.pdf

	pdf.SetFont("Helvetica", "", 24)
	pdf.SetXY(marginL, y)
	pdf.Cell(pageW/2, 10, "Statement")

	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(150, 150, 150)
	pdf.SetXY(marginL+pageW/2, y+3)
	pdf.CellFormat(pageW/2, 5, "ACCOUNT STATEMENT", "", 1, "R", false, 0, "")
	pdf.SetXY(marginL+pageW/2, y+8)
	pdf.CellFormat(
		pageW/2, 5,
		fmt.Sprintf("%s to %s", shortDate(g.data.From), shortDate(g.data.To)),
		"", 0, "R", false, 0, "",
	)
	pdf.SetTextColor(0, 0, 0)

	return y + 20
}

func (g *statementGenerator) drawParties(y float64) float64 {
	pdf := g.pdf
	mid := marginL + pageW/2 + 10

	label := func(x, yy float64, text string) {
		pdf.SetFont("Helvetica", "", 7)
		pdf.SetTextColor(150, 150, 150)
		pdf.SetXY(x, yy)
		pdf.Cell(60, 4, strings.ToUpper(text))
		pdf.SetTextColor(0, 0, 0)
	}

	label(marginL, y, "From")
	label(mid, y, "Statement for")

	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetXY(marginL, y+4)
	pdf.Cell(pageW/2, 5, g.data.CompanyName)
	pdf.SetXY(mid, y+4)
	pdf.Cell(pageW/2-10, 5, g.data.ClientName)

	// Everything below the name is optional, and a blank line left behind by a missing
	// address reads as a broken document. Each side is drawn down its own cursor.
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(90, 90, 90)

	leftY := y + 10
	for _, line := range compact([]string{
		g.data.CompanyAddress,
		phoneLine(g.data.CompanyPhone),
		gstinLine(g.data.CompanyGSTIN),
	}) {
		pdf.SetXY(marginL, leftY)
		pdf.MultiCell(pageW/2-6, 4, line, "", "L", false)
		leftY = pdf.GetY()
	}

	rightY := y + 10
	for _, line := range compact([]string{
		g.data.ClientAddress,
		phoneLine(g.data.ClientPhone),
	}) {
		pdf.SetXY(mid, rightY)
		pdf.MultiCell(pageW/2-10, 4, line, "", "L", false)
		rightY = pdf.GetY()
	}

	pdf.SetTextColor(0, 0, 0)

	if rightY > leftY {
		return rightY + 6
	}
	return leftY + 6
}

// drawSummary is the part somebody reads first: what is owed now, and the three figures
// it came from.
func (g *statementGenerator) drawSummary(y float64) float64 {
	pdf := g.pdf

	pdf.SetDrawColor(220, 220, 220)
	pdf.SetLineWidth(0.1)
	pdf.Line(marginL, y, marginL+pageW, y)

	pdf.SetFont("Helvetica", "", 7)
	pdf.SetTextColor(150, 150, 150)
	pdf.SetXY(marginL, y+4)
	pdf.Cell(60, 4, strings.ToUpper(balanceLabel(g.data.Closing)))
	pdf.SetTextColor(0, 0, 0)

	pdf.SetFont("Helvetica", "B", 20)
	pdf.SetXY(marginL, y+8)
	pdf.Cell(80, 10, money(abs(g.data.Closing)))

	// The three that make it up, on the right, small.
	rows := [][2]string{
		{"Opening balance", money(g.data.Opening)},
		{"Billed this period", money(g.data.Billed)},
		{"Received this period", money(g.data.Paid)},
	}
	rowY := y + 6
	for _, row := range rows {
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetTextColor(90, 90, 90)
		pdf.SetXY(marginL+pageW-80, rowY)
		pdf.CellFormat(50, 5, row[0], "", 0, "R", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
		pdf.CellFormat(30, 5, row[1], "", 0, "R", false, 0, "")
		rowY += 5
	}

	return y + 26
}

func (g *statementGenerator) drawTable(y float64) float64 {
	pdf := g.pdf

	// Date, particulars, debit, credit, balance.
	widths := []float64{22, 86, 26, 26, 30}
	headers := []string{"Date", "Particulars", "Billed", "Received", "Balance"}

	drawHead := func(atY float64) float64 {
		pdf.SetFont("Helvetica", "", 7)
		pdf.SetTextColor(150, 150, 150)
		x := marginL
		for i, head := range headers {
			align := "L"
			if i >= 2 {
				align = "R"
			}
			pdf.SetXY(x, atY)
			pdf.CellFormat(widths[i], 5, strings.ToUpper(head), "", 0, align, false, 0, "")
			x += widths[i]
		}
		pdf.SetTextColor(0, 0, 0)
		pdf.SetDrawColor(220, 220, 220)
		pdf.Line(marginL, atY+5.5, marginL+pageW, atY+5.5)
		return atY + 8
	}

	y = drawHead(y)

	// The opening balance as the first line of the table, so the running balance
	// column starts from something rather than appearing out of nowhere.
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(90, 90, 90)
	pdf.SetXY(marginL, y)
	pdf.CellFormat(widths[0], 5, shortDate(g.data.From), "", 0, "L", false, 0, "")
	pdf.CellFormat(widths[1], 5, "Opening balance", "", 0, "L", false, 0, "")
	pdf.CellFormat(widths[2], 5, "", "", 0, "R", false, 0, "")
	pdf.CellFormat(widths[3], 5, "", "", 0, "R", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.CellFormat(widths[4], 5, money(g.data.Opening), "", 0, "R", false, 0, "")
	y += 6

	if len(g.data.Lines) == 0 {
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetTextColor(150, 150, 150)
		pdf.SetXY(marginL, y+4)
		pdf.CellFormat(pageW, 5, "Nothing on this account in this period.", "", 0, "C", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
		y += 14
	}

	for _, line := range g.data.Lines {
		// A new page needs the column headings again, or the figures below the break
		// are five unlabelled numbers.
		if y > 258 {
			pdf.AddPage()
			y = drawHead(marginT + 4)
		}

		particulars := line.Description
		if particulars == "" {
			particulars = line.Reference
		}

		pdf.SetFont("Helvetica", "", 8)
		pdf.SetXY(marginL, y)
		pdf.CellFormat(widths[0], 5, shortDate(line.Date), "", 0, "L", false, 0, "")
		pdf.CellFormat(widths[1], 5, truncate(particulars, 58), "", 0, "L", false, 0, "")
		pdf.CellFormat(widths[2], 5, moneyOrBlank(line.Debit), "", 0, "R", false, 0, "")
		pdf.CellFormat(widths[3], 5, moneyOrBlank(line.Credit), "", 0, "R", false, 0, "")
		pdf.CellFormat(widths[4], 5, money(line.Balance), "", 0, "R", false, 0, "")

		pdf.SetDrawColor(240, 240, 240)
		pdf.Line(marginL, y+5.5, marginL+pageW, y+5.5)
		y += 6
	}

	// The closing line, ruled off and in bold: the figure the whole page is about.
	pdf.SetDrawColor(180, 180, 180)
	pdf.Line(marginL, y+1, marginL+pageW, y+1)

	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetXY(marginL, y+3)
	pdf.CellFormat(widths[0]+widths[1], 6, balanceLabel(g.data.Closing), "", 0, "L", false, 0, "")
	pdf.CellFormat(widths[2], 6, money(g.data.Billed), "", 0, "R", false, 0, "")
	pdf.CellFormat(widths[3], 6, money(g.data.Paid), "", 0, "R", false, 0, "")
	pdf.CellFormat(widths[4], 6, money(abs(g.data.Closing)), "", 0, "R", false, 0, "")

	return y + 12
}

// MARK: - Small helpers

func shortDate(t time.Time) string { return t.Format("02 Jan 2006") }

// money prints Indian-style, which is how every other figure in this app is shown.
func money(v float64) string {
	negative := v < 0
	if negative {
		v = -v
	}
	whole := int64(v)
	paise := int64((v-float64(whole))*100 + 0.5)
	if paise == 100 {
		whole++
		paise = 0
	}

	// Last three digits, then in twos: 12,34,567.
	digits := fmt.Sprintf("%d", whole)
	var grouped string
	if len(digits) <= 3 {
		grouped = digits
	} else {
		head, tail := digits[:len(digits)-3], digits[len(digits)-3:]
		var parts []string
		for len(head) > 2 {
			parts = append([]string{head[len(head)-2:]}, parts...)
			head = head[:len(head)-2]
		}
		if head != "" {
			parts = append([]string{head}, parts...)
		}
		grouped = strings.Join(parts, ",") + "," + tail
	}

	out := fmt.Sprintf("%s.%02d", grouped, paise)
	if negative {
		return "-" + out
	}
	return out
}

func moneyOrBlank(v float64) string {
	if v == 0 {
		return ""
	}
	return money(v)
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// balanceLabel says which way round the balance is, in words. A minus sign in front of
// a rupee figure is read wrong about half the time.
func balanceLabel(balance float64) string {
	switch {
	case balance > 0.004:
		return "Amount due"
	case balance < -0.004:
		return "In credit"
	default:
		return "Settled in full"
	}
}

func compact(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

func phoneLine(phone string) string {
	if strings.TrimSpace(phone) == "" {
		return ""
	}
	return "Phone: " + phone
}

func gstinLine(gstin string) string {
	if strings.TrimSpace(gstin) == "" {
		return ""
	}
	return "GSTIN: " + gstin
}

func truncate(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit-1]) + "…"
}
