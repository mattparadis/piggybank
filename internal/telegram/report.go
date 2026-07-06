package telegram

import (
	"bytes"
	"fmt"
	"strings"

	"expense_monitor/internal/category"

	chart "github.com/wcharczuk/go-chart/v2"
	"github.com/wcharczuk/go-chart/v2/drawing"
)

const reportCategoryLimit = 8 // top categories listed in the text report
const chartCategoryLimit = 10 // bars drawn in the chart image

// formatReport builds the text body of a monthly report. Kept concise so it fits
// within Telegram's photo caption limit when sent alongside a chart.
func formatReport(month string, sum category.Summary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "📊 Monthly report — %s\n\n", month)
	fmt.Fprintf(&b, "Spent:  %.2f\n", sum.Spent)
	fmt.Fprintf(&b, "Saved:  %.2f\n", sum.Saved)
	fmt.Fprintf(&b, "Income: %.2f\n", sum.Income)
	fmt.Fprintf(&b, "Net:    %.2f\n", sum.Income-sum.Spent-sum.Saved)

	if len(sum.Categories) > 0 {
		b.WriteString("\nTop categories:\n")
		for i, cs := range sum.Categories {
			if i >= reportCategoryLimit {
				break
			}
			fmt.Fprintf(&b, "• %s %s: %.2f\n", cs.Category.Icon, cs.Category.Name, cs.Spent)
		}
	}
	return b.String()
}

// renderCategoryChart renders a PNG bar chart of category spending for the month.
func renderCategoryChart(month string, sum category.Summary) ([]byte, error) {
	var bars []chart.Value
	for i, cs := range sum.Categories {
		if i >= chartCategoryLimit {
			break
		}
		bars = append(bars, chart.Value{
			Label: cs.Category.Name,
			Value: cs.Spent,
			Style: chart.Style{FillColor: parseColor(cs.Category.Color)},
		})
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("no categories to chart")
	}

	bc := chart.BarChart{
		Title:      "Spending by category — " + month,
		Width:      700,
		Height:     400,
		BarWidth:   45,
		Background: chart.Style{Padding: chart.Box{Top: 45, Left: 20, Right: 20, Bottom: 20}},
		Bars:       bars,
	}
	var buf bytes.Buffer
	if err := bc.Render(chart.PNG, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func parseColor(hex string) drawing.Color {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return chart.ColorBlue
	}
	return drawing.ColorFromHex(hex)
}
