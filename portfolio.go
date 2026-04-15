package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

type Holding struct {
	Symbol          string
	Name            string
	Sector          string
	Shares          float64
	AvgPrice        float64
	CostBasis       float64
	CurrentPrice    float64
	MarketValue     float64
	LastDividend    float64
	PaymentsPerYear float64
	AnnualDivShare  float64 // per share
	AnnualDivTotal  float64 // all shares
	YieldOnCost     float64
	TotalGain       float64
	DayGain         float64
	Bucket          BucketType
	Source          string
	LastUpdated     time.Time
}

type Portfolio struct {
	Holdings        []Holding
	MarketValue     float64
	CostBasis       float64
	GainLoss        float64
	Cash            float64
	MonthlyDivInc   float64
	AnnualGoal      float64
	TotalAnnualDiv  float64
	AvgYield        float64
	SourceFile      string

	// Computed stats
	WeightedAvgYield float64
	ProjectedMonthly float64
	DailyIncome      float64
	PositionsGreen   int
	PositionsRed     int
	LargestPosition  string
	LargestPosPct    float64
	BestYielder      string
	BestYielderPct   float64
	TopGainer        string
	TopGainerAmt     float64
	TopLoser         string
	TopLoserAmt      float64

	Modified bool // in-memory edits exist
}

func loadPortfolio(path string) (*Portfolio, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)

	p := &Portfolio{SourceFile: path}

	// Summary cells (rows 1-8)
	p.MarketValue = cellFloat(f, sheet, "B2")
	p.CostBasis = cellFloat(f, sheet, "D2")
	p.GainLoss = cellFloat(f, sheet, "B5")
	p.Cash = cellFloat(f, sheet, "D5")
	p.MonthlyDivInc = cellFloat(f, sheet, "B8")
	p.AnnualGoal = cellFloat(f, sheet, "D8")

	// Holdings start at row 16 (row 15 is header)
	for row := 16; row <= 100; row++ {
		sym, _ := f.GetCellValue(sheet, fmt.Sprintf("A%d", row))
		if sym == "" {
			break
		}

		h := Holding{
			Symbol:          sym,
			Name:            cellStr(f, sheet, fmt.Sprintf("B%d", row)),
			Sector:          cellStr(f, sheet, fmt.Sprintf("C%d", row)),
			Shares:          cellFloat(f, sheet, fmt.Sprintf("E%d", row)),
			AvgPrice:        cellFloat(f, sheet, fmt.Sprintf("F%d", row)),
			CostBasis:       cellFloat(f, sheet, fmt.Sprintf("G%d", row)),
			CurrentPrice:    cellFloat(f, sheet, fmt.Sprintf("H%d", row)),
			MarketValue:     cellFloat(f, sheet, fmt.Sprintf("I%d", row)),
			LastDividend:    cellFloat(f, sheet, fmt.Sprintf("J%d", row)),
			PaymentsPerYear: cellFloat(f, sheet, fmt.Sprintf("K%d", row)),
			AnnualDivShare:  cellFloat(f, sheet, fmt.Sprintf("L%d", row)),
			AnnualDivTotal:  cellFloat(f, sheet, fmt.Sprintf("M%d", row)),
			YieldOnCost:     cellFloat(f, sheet, fmt.Sprintf("N%d", row)),
			TotalGain:       cellFloat(f, sheet, fmt.Sprintf("O%d", row)),
			DayGain:         cellFloat(f, sheet, fmt.Sprintf("P%d", row)),
		}
		p.Holdings = append(p.Holdings, h)
		p.TotalAnnualDiv += h.AnnualDivTotal
	}

	if p.CostBasis > 0 {
		p.AvgYield = (p.TotalAnnualDiv / p.CostBasis) * 100
	}

	p.computeStats()
	return p, nil
}

func (p *Portfolio) computeStats() {
	p.ProjectedMonthly = p.TotalAnnualDiv / 12
	p.DailyIncome = p.TotalAnnualDiv / 365

	// Weighted average yield (by market value)
	totalMktVal := 0.0
	weightedYield := 0.0
	p.PositionsGreen = 0
	p.PositionsRed = 0

	var largestMV float64
	var bestYOC float64
	var topGain float64
	var topLoss float64
	topGainInit := false
	topLossInit := false

	for _, h := range p.Holdings {
		totalMktVal += h.MarketValue
		if h.MarketValue > 0 && h.CostBasis > 0 {
			yld := h.AnnualDivTotal / h.MarketValue
			weightedYield += yld * h.MarketValue
		}

		if h.TotalGain >= 0 {
			p.PositionsGreen++
		} else {
			p.PositionsRed++
		}

		if h.MarketValue > largestMV {
			largestMV = h.MarketValue
			p.LargestPosition = h.Symbol
		}

		yoc := h.YieldOnCost * 100
		if yoc > bestYOC {
			bestYOC = yoc
			p.BestYielder = h.Symbol
			p.BestYielderPct = yoc
		}

		if !topGainInit || h.TotalGain > topGain {
			topGain = h.TotalGain
			p.TopGainer = h.Symbol
			p.TopGainerAmt = h.TotalGain
			topGainInit = true
		}
		if !topLossInit || h.TotalGain < topLoss {
			topLoss = h.TotalGain
			p.TopLoser = h.Symbol
			p.TopLoserAmt = h.TotalGain
			topLossInit = true
		}
	}

	if totalMktVal > 0 {
		p.WeightedAvgYield = (weightedYield / totalMktVal) * 100
		p.LargestPosPct = (largestMV / totalMktVal) * 100
	}
}

// recompute recalculates derived fields after in-memory edits
func (h *Holding) recompute() {
	h.CostBasis = h.Shares * h.AvgPrice
	h.MarketValue = h.Shares * h.CurrentPrice
	h.TotalGain = h.MarketValue - h.CostBasis
	h.AnnualDivTotal = h.AnnualDivShare * h.Shares
	if h.CostBasis > 0 {
		h.YieldOnCost = h.AnnualDivTotal / h.CostBasis
	}
}

func (p *Portfolio) recomputeAll() {
	p.TotalAnnualDiv = 0
	p.MarketValue = 0
	p.CostBasis = 0
	p.GainLoss = 0
	for _, h := range p.Holdings {
		p.TotalAnnualDiv += h.AnnualDivTotal
		p.MarketValue += h.MarketValue
		p.CostBasis += h.CostBasis
		p.GainLoss += h.TotalGain
	}
	if p.CostBasis > 0 {
		p.AvgYield = (p.TotalAnnualDiv / p.CostBasis) * 100
	}
	p.computeStats()
}

func sortHoldings(holdings []Holding, sortBy string, desc bool) {
	sort.SliceStable(holdings, func(i, j int) bool {
		var less bool
		switch sortBy {
		case "annual_div":
			less = holdings[i].AnnualDivTotal < holdings[j].AnnualDivTotal
		case "yield":
			less = holdings[i].YieldOnCost < holdings[j].YieldOnCost
		case "mkt_value":
			less = holdings[i].MarketValue < holdings[j].MarketValue
		case "gain":
			less = holdings[i].TotalGain < holdings[j].TotalGain
		default: // symbol
			less = holdings[i].Symbol < holdings[j].Symbol
		}
		if desc {
			return !less
		}
		return less
	})
}

func cellFloat(f *excelize.File, sheet, cell string) float64 {
	v, _ := f.GetCellValue(sheet, cell)
	v = strings.TrimSpace(v)
	v = strings.ReplaceAll(v, "$", "")
	v = strings.ReplaceAll(v, ",", "")
	v = strings.ReplaceAll(v, "(", "-")
	v = strings.ReplaceAll(v, ")", "")
	if strings.HasSuffix(v, "%") {
		v = strings.TrimSuffix(v, "%")
		var n float64
		fmt.Sscanf(v, "%f", &n)
		return n / 100
	}
	var n float64
	fmt.Sscanf(v, "%f", &n)
	return n
}

func cellStr(f *excelize.File, sheet, cell string) string {
	v, _ := f.GetCellValue(sheet, cell)
	return v
}

// ── Formatting helpers ──

func fmtMoney(v float64) string {
	prefix := "$"
	if v < 0 {
		prefix = "-$"
		v = -v
	}
	s := fmt.Sprintf("%.2f", v)
	parts := strings.Split(s, ".")
	intPart := parts[0]
	var result []byte
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return prefix + string(result) + "." + parts[1]
}

func fmtCompact(v float64) string {
	abs := math.Abs(v)
	prefix := ""
	if v < 0 {
		prefix = "-"
	}
	if abs >= 1000 {
		s := fmt.Sprintf("%.0f", abs)
		parts := []string{}
		for i := len(s); i > 0; i -= 3 {
			start := i - 3
			if start < 0 {
				start = 0
			}
			parts = append([]string{s[start:i]}, parts...)
		}
		return prefix + "$" + strings.Join(parts, ",")
	}
	return prefix + fmt.Sprintf("$%.2f", abs)
}

func fmtDiv(v float64) string {
	if v == 0 {
		return "  -"
	}
	return fmt.Sprintf("$%.4f", v)[:7]
}

func colorMoney(v float64) string {
	s := fmtMoney(v)
	if v >= 0 {
		return boldGreen + "+" + s + reset
	}
	return boldRed + s + reset
}

func colorPct(v float64) string {
	sign := "+"
	if v < 0 {
		sign = ""
	}
	s := fmt.Sprintf("%s%.1f%%", sign, v)
	if v >= 0 {
		return green + s + reset
	}
	return red + s + reset
}

func colorCompact(v float64) string {
	s := fmtCompact(v)
	if v >= 0 {
		return green + "+" + s + reset
	}
	return red + s + reset
}

func yieldColor(pct float64, s string) string {
	switch {
	case pct >= 1.0:
		return boldGreen + s + reset
	case pct >= 0.3:
		return yellow + s + reset
	default:
		return dimWhite + s + reset
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// DivCalendarEntry represents a single dividend payment in a calendar month.
type DivCalendarEntry struct {
	Symbol string
	Month  int     // 1-12
	Amount float64 // per-share dividend amount
}

// BuildDividendCalendar distributes known dividend events across 12 months.
// For holdings with Yahoo quotes that include dividend history, it uses the
// actual months. For holdings without history, it estimates evenly across months.
func BuildDividendCalendar(holdings []Holding, quotes map[string]YahooQuote) [12][]DivCalendarEntry {
	var cal [12][]DivCalendarEntry

	now := time.Now()
	oneYearAgo := now.AddDate(-1, 0, 0)

	placed := map[string]bool{}

	for _, h := range holdings {
		q, ok := quotes[h.Symbol]
		if ok && len(q.DividendHistory) > 0 {
			for _, d := range q.DividendHistory {
				if d.Date.After(oneYearAgo) {
					m := int(d.Date.Month()) - 1 // 0-indexed
					cal[m] = append(cal[m], DivCalendarEntry{
						Symbol: h.Symbol,
						Month:  int(d.Date.Month()),
						Amount: d.Amount,
					})
				}
			}
			placed[h.Symbol] = true
		}
	}

	// For holdings without dividend history, estimate from frequency
	for _, h := range holdings {
		if placed[h.Symbol] || h.PaymentsPerYear == 0 || h.LastDividend == 0 {
			continue
		}
		freq := int(h.PaymentsPerYear)
		if freq <= 0 {
			continue
		}
		gap := 12 / freq
		for i := 0; i < freq; i++ {
			m := (i * gap) % 12
			cal[m] = append(cal[m], DivCalendarEntry{
				Symbol: h.Symbol,
				Month:  m + 1,
				Amount: h.LastDividend,
			})
		}
	}

	return cal
}

// HoldingsFromImported converts ImportedHolding slice to Holding slice,
// assigning bucket from the symbol map.
func HoldingsFromImported(imported []ImportedHolding, symMap *SymbolMap) []Holding {
	var holdings []Holding
	for _, ih := range imported {
		h := Holding{
			Symbol:       ih.Symbol,
			Name:         ih.Name,
			Shares:       ih.Shares,
			CurrentPrice: ih.Price,
			MarketValue:  ih.MarketValue,
			Source:        ih.Source,
			Bucket:       symMap.Lookup(ih.Symbol),
			LastUpdated:  time.Now(),
		}
		if h.MarketValue == 0 && h.Shares > 0 && h.CurrentPrice > 0 {
			h.MarketValue = h.Shares * h.CurrentPrice
		}
		holdings = append(holdings, h)
	}
	return holdings
}

// MergeHoldings merges new holdings into existing ones, summing shares
// for duplicate symbols from the same source or updating if from a different source.
func MergeHoldings(existing, incoming []Holding) []Holding {
	bySymbol := map[string]*Holding{}
	for i := range existing {
		bySymbol[existing[i].Symbol] = &existing[i]
	}
	for _, h := range incoming {
		if ex, ok := bySymbol[h.Symbol]; ok {
			// Update with newer data
			ex.CurrentPrice = h.CurrentPrice
			ex.MarketValue = h.MarketValue
			ex.Shares = h.Shares
			ex.Source = h.Source
			ex.LastUpdated = h.LastUpdated
			if h.Name != "" {
				ex.Name = h.Name
			}
		} else {
			existing = append(existing, h)
			bySymbol[h.Symbol] = &existing[len(existing)-1]
		}
	}
	return existing
}

// ApplyYahooQuotes updates holdings with live price data from Yahoo Finance.
func ApplyYahooQuotes(holdings []Holding, quotes map[string]YahooQuote) {
	for i := range holdings {
		q, ok := quotes[holdings[i].Symbol]
		if !ok {
			continue
		}
		holdings[i].CurrentPrice = q.Price
		holdings[i].MarketValue = holdings[i].Shares * q.Price
		holdings[i].TotalGain = holdings[i].MarketValue - holdings[i].CostBasis
		holdings[i].LastDividend = q.LastDividend
		holdings[i].PaymentsPerYear = float64(q.DividendFreq)
		holdings[i].AnnualDivShare = q.LastDividend * float64(q.DividendFreq)
		holdings[i].AnnualDivTotal = holdings[i].AnnualDivShare * holdings[i].Shares
		if holdings[i].CostBasis > 0 {
			holdings[i].YieldOnCost = holdings[i].AnnualDivTotal / holdings[i].CostBasis
		}
		holdings[i].LastUpdated = q.LastUpdated
	}
}
