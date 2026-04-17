package main

import (
	"fmt"

	"github.com/xuri/excelize/v2"
)

// ExportXlsx writes the current portfolio state to a multi-sheet xlsx file.
func ExportXlsx(path string, buckets map[BucketType]*Bucket, totalValue float64, symMap *SymbolMap, cashStore *CashStore) error {
	f := excelize.NewFile()
	defer f.Close()

	// ── Sheet 1: Summary ──
	summarySheet := "Summary"
	f.SetSheetName("Sheet1", summarySheet)

	totalCost := 0.0
	totalGain := 0.0
	totalIncome := 0.0
	for _, b := range buckets {
		totalCost += b.CostBasis
		totalGain += b.UnrealizedGain
		totalIncome += b.AnnualIncome
	}
	cashTotal := cashStore.Total()
	totalYield := 0.0
	if totalCost > 0 {
		totalYield = (totalIncome / totalCost) * 100
	}

	summaryRows := [][]interface{}{
		{"PORTFOLIO SUMMARY"},
		{},
		{"Metric", "Value"},
		{"Total Market Value", totalValue},
		{"Total Cost Basis", totalCost},
		{"Total Gain/Loss", totalGain},
		{"Gain/Loss %", pctOrZero(totalGain, totalCost)},
		{"Cash Balance", cashTotal},
		{"Annual Dividend Income", totalIncome},
		{"Monthly Dividend Income", totalIncome / 12},
		{"Portfolio Yield on Cost", totalYield},
		{},
		{"CASH BALANCES BY BROKERAGE"},
		{"Brokerage", "Balance"},
	}
	for source, bal := range cashStore.Balances {
		summaryRows = append(summaryRows, []interface{}{source, bal})
	}

	writeRows(f, summarySheet, summaryRows)
	styleSummary(f, summarySheet, len(summaryRows))

	// ── Sheet 2: Holdings ──
	holdingsSheet := "Holdings"
	f.NewSheet(holdingsSheet)

	holdingsHeader := []interface{}{
		"Symbol", "Name", "Bucket", "Shares",
		"Avg Price", "Cost Basis", "Current Price", "Market Value",
		"Gain/Loss $", "Gain/Loss %",
		"Last Dividend", "Annual Dividend", "Yield on Cost %",
		"Source",
	}
	holdingsRows := [][]interface{}{holdingsHeader}

	for _, bt := range AllBuckets {
		b := buckets[bt]
		if b == nil {
			continue
		}
		for _, h := range b.Holdings {
			gainPct := pctOrZero(h.TotalGain, h.CostBasis)
			yoc := 0.0
			if h.YieldOnCost > 0 {
				yoc = h.YieldOnCost * 100
			}
			holdingsRows = append(holdingsRows, []interface{}{
				h.Symbol,
				h.Name,
				bt.Label(),
				h.Shares,
				h.AvgPrice,
				h.CostBasis,
				h.CurrentPrice,
				h.MarketValue,
				h.TotalGain,
				gainPct,
				h.LastDividend,
				h.AnnualDivTotal,
				yoc,
				h.Source,
			})
		}
	}

	writeRows(f, holdingsSheet, holdingsRows)
	styleHoldings(f, holdingsSheet, len(holdingsRows))

	// ── Sheet 3: Buckets ──
	bucketsSheet := "Buckets"
	f.NewSheet(bucketsSheet)

	bucketsHeader := []interface{}{
		"Bucket", "Market Value", "Cost Basis",
		"Unrealized Gain", "Unrealized %",
		"Realized Gain YTD", "Annual Income",
		"Allocation %", "Income Yield %",
		"Month Change $", "Month Change %",
	}
	bucketsRows := [][]interface{}{bucketsHeader}

	for _, bt := range AllBuckets {
		b := buckets[bt]
		if b == nil {
			continue
		}
		unrealizedPct := pctOrZero(b.UnrealizedGain, b.CostBasis)
		bucketsRows = append(bucketsRows, []interface{}{
			bt.Label(),
			b.MarketValue,
			b.CostBasis,
			b.UnrealizedGain,
			unrealizedPct,
			b.RealizedGain,
			b.AnnualIncome,
			b.AllocationPct,
			b.IncomeYield,
			b.MonthChange,
			b.MonthChangePct,
		})
	}

	// Totals row
	bucketsRows = append(bucketsRows, []interface{}{
		"TOTAL",
		totalValue,
		totalCost,
		totalGain,
		pctOrZero(totalGain, totalCost),
		"",
		totalIncome,
		100.0,
		totalYield,
		"",
		"",
	})

	writeRows(f, bucketsSheet, bucketsRows)
	styleBuckets(f, bucketsSheet, len(bucketsRows))

	return f.SaveAs(path)
}

func pctOrZero(gain, basis float64) float64 {
	if basis == 0 {
		return 0
	}
	return (gain / basis) * 100
}

func writeRows(f *excelize.File, sheet string, rows [][]interface{}) {
	for i, row := range rows {
		for j, val := range row {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+1)
			f.SetCellValue(sheet, cell, val)
		}
	}
}

// ── Styling helpers ──

func styleSummary(f *excelize.File, sheet string, rowCount int) {
	// Title bold
	titleStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 14}})
	f.SetCellStyle(sheet, "A1", "A1", titleStyle)

	// Section header bold
	sectionStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 11}})
	f.SetCellStyle(sheet, "A3", "B3", sectionStyle)

	// Find cash header row
	for r := 1; r <= rowCount; r++ {
		cell, _ := excelize.CoordinatesToCellName(1, r)
		val, _ := f.GetCellValue(sheet, cell)
		if val == "CASH BALANCES BY BROKERAGE" {
			f.SetCellStyle(sheet, cell, cell, titleStyle)
			next, _ := excelize.CoordinatesToCellName(1, r+1)
			nextB, _ := excelize.CoordinatesToCellName(2, r+1)
			f.SetCellStyle(sheet, next, nextB, sectionStyle)
			break
		}
	}

	// Currency format for value column
	moneyStyle, _ := f.NewStyle(&excelize.Style{
		NumFmt: 4, // #,##0.00
	})
	for r := 4; r <= rowCount; r++ {
		cell, _ := excelize.CoordinatesToCellName(2, r)
		f.SetCellStyle(sheet, cell, cell, moneyStyle)
	}

	f.SetColWidth(sheet, "A", "A", 30)
	f.SetColWidth(sheet, "B", "B", 18)
}

func styleHoldings(f *excelize.File, sheet string, rowCount int) {
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"D9E1F2"}},
	})
	lastCol, _ := excelize.CoordinatesToCellName(14, 1)
	f.SetCellStyle(sheet, "A1", lastCol, headerStyle)

	moneyStyle, _ := f.NewStyle(&excelize.Style{NumFmt: 4})
	pctStyle, _ := f.NewStyle(&excelize.Style{NumFmt: 2}) // 0.00

	moneyCols := []int{5, 6, 7, 8, 9, 11, 12} // AvgPrice, CostBasis, CurPrice, MktVal, Gain$, LastDiv, AnnDiv
	pctCols := []int{10, 13}                     // Gain%, YOC%

	for r := 2; r <= rowCount; r++ {
		for _, c := range moneyCols {
			cell, _ := excelize.CoordinatesToCellName(c, r)
			f.SetCellStyle(sheet, cell, cell, moneyStyle)
		}
		for _, c := range pctCols {
			cell, _ := excelize.CoordinatesToCellName(c, r)
			f.SetCellStyle(sheet, cell, cell, pctStyle)
		}
	}

	f.SetColWidth(sheet, "A", "A", 10)
	f.SetColWidth(sheet, "B", "B", 35)
	f.SetColWidth(sheet, "C", "C", 18)
	f.SetColWidth(sheet, "D", "N", 14)
}

func styleBuckets(f *excelize.File, sheet string, rowCount int) {
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"D9E1F2"}},
	})
	lastCol, _ := excelize.CoordinatesToCellName(11, 1)
	f.SetCellStyle(sheet, "A1", lastCol, headerStyle)

	// Bold totals row
	totalStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, NumFmt: 4})
	totalRow := fmt.Sprintf("A%d", rowCount)
	totalEnd, _ := excelize.CoordinatesToCellName(11, rowCount)
	f.SetCellStyle(sheet, totalRow, totalEnd, totalStyle)

	moneyStyle, _ := f.NewStyle(&excelize.Style{NumFmt: 4})
	pctStyle, _ := f.NewStyle(&excelize.Style{NumFmt: 2})

	moneyCols := []int{2, 3, 4, 6, 7, 10} // MktVal, Cost, Unrealized, Realized, Income, MonthChg$
	pctCols := []int{5, 8, 9, 11}          // Unrealized%, Alloc%, Yield%, MonthChg%

	for r := 2; r <= rowCount; r++ {
		for _, c := range moneyCols {
			cell, _ := excelize.CoordinatesToCellName(c, r)
			f.SetCellStyle(sheet, cell, cell, moneyStyle)
		}
		for _, c := range pctCols {
			cell, _ := excelize.CoordinatesToCellName(c, r)
			f.SetCellStyle(sheet, cell, cell, pctStyle)
		}
	}

	f.SetColWidth(sheet, "A", "A", 20)
	f.SetColWidth(sheet, "B", "K", 16)
}
