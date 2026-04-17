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

// ParseXlsx opens an xlsx file and auto-detects the format.
// Supports Schwab position exports and the app's own fixed-layout xlsx.
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

	// Auto-detect: scan first 10 rows for a Schwab-style header
	for row := 1; row <= 10; row++ {
		colMap := xlsxDetectHeader(f, sheet, row)
		if colMap != nil {
			return parseBrokerageXlsx(f, sheet, row, colMap)
		}
	}

	// Fall back to fixed-layout format (holdings start at row 16)
	return parseFixedXlsx(f, sheet)
}

// xlsxDetectHeader checks if the given row contains known brokerage header columns.
// Returns a column-name-to-letter map, or nil if not a recognized header.
func xlsxDetectHeader(f *excelize.File, sheet string, row int) map[string]string {
	colMap := make(map[string]string)
	for col := 'A'; col <= 'Z'; col++ {
		cell := fmt.Sprintf("%c%d", col, row)
		val, _ := f.GetCellValue(sheet, cell)
		val = strings.TrimSpace(val)
		if val == "" {
			continue
		}
		upper := strings.ToUpper(val)

		switch {
		case upper == "SYMBOL":
			colMap["Symbol"] = string(col)
		case upper == "DESCRIPTION" || upper == "NAME":
			colMap["Name"] = string(col)
		case strings.HasPrefix(upper, "QTY") || upper == "QUANTITY" || upper == "SHARES":
			colMap["Shares"] = string(col)
		case upper == "PRICE":
			colMap["Price"] = string(col)
		case strings.HasPrefix(upper, "MKT VAL") || upper == "MARKET VALUE":
			colMap["MarketValue"] = string(col)
		case strings.HasPrefix(upper, "COST") || upper == "COST BASIS":
			colMap["CostBasis"] = string(col)
		case strings.HasPrefix(upper, "DIV YLD") || upper == "DIVIDEND YIELD":
			colMap["DivYield"] = string(col)
		case strings.HasPrefix(upper, "LAST DIV") || upper == "LAST DIVIDEND":
			colMap["LastDiv"] = string(col)
		}
	}

	// Need at least Symbol, Shares, and Price to be a valid header
	if colMap["Symbol"] != "" && colMap["Shares"] != "" && colMap["Price"] != "" {
		return colMap
	}
	return nil
}

// parseBrokerageXlsx reads holdings from a brokerage xlsx using a detected column map.
// Returns an ImportResult with holdings and extracted cash balance.
func parseBrokerageXlsx(f *excelize.File, sheet string, headerRow int, colMap map[string]string) ([]ImportedHolding, error) {
	var holdings []ImportedHolding
	var cashBalance float64
	for row := headerRow + 1; ; row++ {
		symbol, _ := f.GetCellValue(sheet, fmt.Sprintf("%s%d", colMap["Symbol"], row))
		symbol = strings.TrimSpace(symbol)
		if symbol == "" {
			break
		}
		lower := strings.ToLower(symbol)
		if strings.Contains(lower, "total") {
			continue
		}
		// Extract cash balance from "Cash & Cash Investments" row
		if strings.Contains(lower, "cash") {
			if col := colMap["MarketValue"]; col != "" {
				cashBalance += xlsxColFloat(f, sheet, col, row)
			}
			continue
		}

		name := ""
		if col := colMap["Name"]; col != "" {
			name, _ = f.GetCellValue(sheet, fmt.Sprintf("%s%d", col, row))
			name = strings.TrimSpace(name)
		}

		shares := xlsxColFloat(f, sheet, colMap["Shares"], row)
		price := xlsxColFloat(f, sheet, colMap["Price"], row)
		mktVal := xlsxColFloat(f, sheet, colMap["MarketValue"], row)
		if mktVal == 0 && shares > 0 && price > 0 {
			mktVal = shares * price
		}
		costBasis := xlsxColFloat(f, sheet, colMap["CostBasis"], row)
		totalGain := mktVal - costBasis
		divYield := xlsxColFloat(f, sheet, colMap["DivYield"], row)
		lastDiv := xlsxColFloat(f, sheet, colMap["LastDiv"], row)

		holdings = append(holdings, ImportedHolding{
			Symbol:      symbol,
			Name:        name,
			Shares:      shares,
			Price:       price,
			MarketValue: mktVal,
			CostBasis:   costBasis,
			TotalGain:   totalGain,
			DivYield:    divYield,
			LastDiv:     lastDiv,
			Source:      "Schwab",
		})
	}

	// Store cash balance for this source
	if cashBalance > 0 {
		cs := LoadCashStore()
		cs.Set("Schwab", cashBalance)
		_ = cs.Save()
	}

	return holdings, nil
}

// parseFixedXlsx reads holdings from the app's own fixed-layout xlsx (data at row 16).
func parseFixedXlsx(f *excelize.File, sheet string) ([]ImportedHolding, error) {
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

		holdings = append(holdings, ImportedHolding{
			Symbol:      symbol,
			Name:        strings.TrimSpace(name),
			Shares:      shares,
			Price:       price,
			MarketValue: marketValue,
			Source:      "xlsx",
		})
	}
	return holdings, nil
}

// xlsxColFloat reads a float from a column letter + row number.
func xlsxColFloat(f *excelize.File, sheet, col string, row int) float64 {
	if col == "" {
		return 0
	}
	return xlsxCellFloat(f, sheet, fmt.Sprintf("%s%d", col, row))
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
