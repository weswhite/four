package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// XlsxSummary holds portfolio-level summary values read from the xlsx file.
type XlsxSummary struct {
	MarketValue    float64
	CostBasis      float64
	GainLoss       float64
	Cash           float64
	MonthlyDivInc  float64
	AnnualGoal     float64
}

// ParseXlsx opens an xlsx file and reads holdings starting at row 16.
// Returns ImportedHolding structs with Source set to "xlsx".
func ParseXlsx(path string) ([]ImportedHolding, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	if sheet == "" {
		return nil, fmt.Errorf("no sheets found in xlsx file")
	}

	var holdings []ImportedHolding
	for row := 16; ; row++ {
		symbol, _ := f.GetCellValue(sheet, fmt.Sprintf("A%d", row))
		symbol = strings.TrimSpace(symbol)
		if symbol == "" {
			break
		}

		name, _ := f.GetCellValue(sheet, fmt.Sprintf("B%d", row))
		shares := xlsxCellFloat(f, sheet, fmt.Sprintf("C%d", row))
		price := xlsxCellFloat(f, sheet, fmt.Sprintf("D%d", row))
		marketValue := xlsxCellFloat(f, sheet, fmt.Sprintf("E%d", row))

		h := ImportedHolding{
			Symbol:      symbol,
			Name:        strings.TrimSpace(name),
			Shares:      shares,
			Price:       price,
			MarketValue: marketValue,
			Source:      "xlsx",
		}
		holdings = append(holdings, h)
	}

	return holdings, nil
}

// ParseXlsxSummary reads portfolio summary values from specific cells:
//   - B2: MarketValue
//   - D2: CostBasis
//   - B5: GainLoss
//   - D5: Cash
//   - B8: MonthlyDivInc
//   - D8: AnnualGoal
func ParseXlsxSummary(path string) (*XlsxSummary, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	if sheet == "" {
		return nil, fmt.Errorf("no sheets found in xlsx file")
	}

	summary := &XlsxSummary{
		MarketValue:   xlsxCellFloat(f, sheet, "B2"),
		CostBasis:     xlsxCellFloat(f, sheet, "D2"),
		GainLoss:      xlsxCellFloat(f, sheet, "B5"),
		Cash:          xlsxCellFloat(f, sheet, "D5"),
		MonthlyDivInc: xlsxCellFloat(f, sheet, "B8"),
		AnnualGoal:    xlsxCellFloat(f, sheet, "D8"),
	}

	return summary, nil
}

// xlsxCellFloat reads a cell value from an excelize file and parses it as a float64.
// It strips currency symbols ($), commas, percent signs, whitespace, and handles
// parentheses for negative values. Returns 0 on any failure.
func xlsxCellFloat(f *excelize.File, sheet, cell string) float64 {
	raw, _ := f.GetCellValue(sheet, cell)
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "--" || raw == "n/a" || raw == "N/A" {
		return 0
	}

	negative := false
	if strings.HasPrefix(raw, "(") && strings.HasSuffix(raw, ")") {
		negative = true
		raw = raw[1 : len(raw)-1]
	}

	raw = strings.ReplaceAll(raw, "$", "")
	raw = strings.ReplaceAll(raw, ",", "")
	raw = strings.ReplaceAll(raw, "%", "")
	raw = strings.TrimSpace(raw)

	if raw == "" {
		return 0
	}

	val, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}

	if negative {
		val = -val
	}
	return val
}
