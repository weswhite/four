package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"strconv"
	"time"
)

// BrokerageType identifies the brokerage format of a CSV file.
type BrokerageType string

const (
	BrokerSchwab   BrokerageType = "Schwab"
	BrokerFidelity BrokerageType = "Fidelity"
	BrokerVanguard BrokerageType = "Vanguard"
	BrokerSoFi     BrokerageType = "SoFi"
	BrokerUnknown  BrokerageType = "Unknown"
)

// ImportedHolding represents a single holding imported from a brokerage CSV or xlsx file.
type ImportedHolding struct {
	Symbol      string
	Name        string
	Shares      float64
	Price       float64
	MarketValue float64
	CostBasis   float64
	TotalGain   float64
	DivYield    float64
	LastDiv     float64
	Source      string // brokerage name or "xlsx"
}

// ImportResult holds holdings plus metadata extracted during import.
type ImportResult struct {
	Holdings []ImportedHolding
	Cash     float64
	Source   string
}

// DetectBrokerageFormat reads the first lines of a CSV file and detects
// which brokerage exported it based on header column patterns.
func DetectBrokerageFormat(path string) (BrokerageType, error) {
	f, err := os.Open(path)
	if err != nil {
		return BrokerUnknown, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1

	// Read up to 30 lines to find a header row.
	for i := 0; i < 30; i++ {
		record, err := reader.Read()
		if err != nil {
			break
		}

		joined := strings.Join(record, "|")
		upper := strings.ToUpper(joined)

		// Schwab: header contains "Symbol", "Quantity", and "Market Value"
		if containsAll(upper, "SYMBOL", "QUANTITY", "MARKET VALUE") {
			return BrokerSchwab, nil
		}

		// Fidelity: header contains "Account Name/Number" and "Symbol"
		if containsAll(upper, "ACCOUNT NAME/NUMBER", "SYMBOL") {
			return BrokerFidelity, nil
		}

		// Vanguard: header contains "Account Number", "Investment Name", and "Shares"
		if containsAll(upper, "ACCOUNT NUMBER", "INVESTMENT NAME", "SHARES") {
			return BrokerVanguard, nil
		}

		// SoFi: header contains "Symbol" and "Average Cost"
		if containsAll(upper, "SYMBOL", "AVERAGE COST") {
			return BrokerSoFi, nil
		}
	}

	return BrokerUnknown, fmt.Errorf("unable to detect brokerage format from headers")
}

// containsAll checks whether s contains every one of the provided substrings.
func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

// ParseCSV dispatches to the appropriate broker-specific parser.
func ParseCSV(path string, broker BrokerageType) ([]ImportedHolding, error) {
	switch broker {
	case BrokerSchwab:
		return parseSchwabCSV(path)
	case BrokerFidelity:
		return parseFidelityCSV(path)
	case BrokerVanguard:
		return parseVanguardCSV(path)
	case BrokerSoFi:
		return parseSoFiCSV(path)
	default:
		return nil, fmt.Errorf("unsupported brokerage format: %s", broker)
	}
}

// parseSchwabCSV parses a Charles Schwab CSV export.
// Schwab files may have account info lines before the actual header row.
// Header columns: Symbol, Name, Quantity, Price, Market Value, etc.
func parseSchwabCSV(path string) ([]ImportedHolding, error) {
	lines, err := readAllCSV(path)
	if err != nil {
		return nil, fmt.Errorf("read schwab csv: %w", err)
	}

	headerIdx, colMap := findHeaderRow(lines, []string{"Symbol", "Quantity", "Market Value"})
	if headerIdx < 0 {
		return nil, fmt.Errorf("schwab header row not found")
	}

	var holdings []ImportedHolding
	for i := headerIdx + 1; i < len(lines); i++ {
		row := lines[i]
		symbol := colGet(row, colMap, "Symbol")
		symbol = strings.TrimSpace(symbol)

		if symbol == "" || strings.EqualFold(symbol, "Cash") {
			continue
		}
		// Skip total/summary rows
		if strings.Contains(strings.ToLower(symbol), "total") {
			continue
		}

		h := ImportedHolding{
			Symbol:      symbol,
			Name:        colGet(row, colMap, "Name"),
			Shares:      parseNumber(colGet(row, colMap, "Quantity")),
			Price:       parseNumber(colGet(row, colMap, "Price")),
			MarketValue: parseNumber(colGet(row, colMap, "Market Value")),
			Source:      string(BrokerSchwab),
		}
		holdings = append(holdings, h)
	}

	return holdings, nil
}

// parseFidelityCSV parses a Fidelity CSV export.
// Fidelity uses "Last Price" for price and "Current Value" for market value.
func parseFidelityCSV(path string) ([]ImportedHolding, error) {
	lines, err := readAllCSV(path)
	if err != nil {
		return nil, fmt.Errorf("read fidelity csv: %w", err)
	}

	headerIdx, colMap := findHeaderRow(lines, []string{"Account Name/Number", "Symbol"})
	if headerIdx < 0 {
		return nil, fmt.Errorf("fidelity header row not found")
	}

	var holdings []ImportedHolding
	for i := headerIdx + 1; i < len(lines); i++ {
		row := lines[i]
		symbol := strings.TrimSpace(colGet(row, colMap, "Symbol"))

		if symbol == "" || strings.EqualFold(symbol, "Cash") {
			continue
		}
		if strings.Contains(strings.ToLower(symbol), "total") {
			continue
		}

		h := ImportedHolding{
			Symbol:      symbol,
			Name:        colGet(row, colMap, "Description"),
			Shares:      parseNumber(colGet(row, colMap, "Quantity")),
			Price:       parseNumber(colGet(row, colMap, "Last Price")),
			MarketValue: parseNumber(colGet(row, colMap, "Current Value")),
			Source:      string(BrokerFidelity),
		}
		holdings = append(holdings, h)
	}

	return holdings, nil
}

// parseVanguardCSV parses a Vanguard CSV export.
// Vanguard uses "Share Price" for price and "Total Value" for market value.
func parseVanguardCSV(path string) ([]ImportedHolding, error) {
	lines, err := readAllCSV(path)
	if err != nil {
		return nil, fmt.Errorf("read vanguard csv: %w", err)
	}

	headerIdx, colMap := findHeaderRow(lines, []string{"Account Number", "Investment Name", "Shares"})
	if headerIdx < 0 {
		return nil, fmt.Errorf("vanguard header row not found")
	}

	var holdings []ImportedHolding
	for i := headerIdx + 1; i < len(lines); i++ {
		row := lines[i]

		// Vanguard may use "Symbol" or "Ticker" column
		symbol := strings.TrimSpace(colGet(row, colMap, "Symbol"))
		if symbol == "" {
			symbol = strings.TrimSpace(colGet(row, colMap, "Ticker"))
		}
		name := strings.TrimSpace(colGet(row, colMap, "Investment Name"))

		if symbol == "" && name == "" {
			continue
		}
		lower := strings.ToLower(name)
		if strings.Contains(lower, "total") {
			continue
		}

		h := ImportedHolding{
			Symbol:      symbol,
			Name:        name,
			Shares:      parseNumber(colGet(row, colMap, "Shares")),
			Price:       parseNumber(colGet(row, colMap, "Share Price")),
			MarketValue: parseNumber(colGet(row, colMap, "Total Value")),
			Source:      string(BrokerVanguard),
		}
		holdings = append(holdings, h)
	}

	return holdings, nil
}

// parseSoFiCSV is a placeholder parser for SoFi CSV exports.
func parseSoFiCSV(_ string) ([]ImportedHolding, error) {
	return nil, fmt.Errorf("SoFi format not yet supported")
}

// parseNumber strips currency symbols, commas, whitespace, percent signs,
// and handles parentheses for negative values. Returns 0 on parse failure.
func parseNumber(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "--" || s == "n/a" || s == "N/A" {
		return 0
	}

	negative := false
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		negative = true
		s = s[1 : len(s)-1]
	}

	s = strings.ReplaceAll(s, "$", "")
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "%", "")
	s = strings.TrimSpace(s)

	if s == "" {
		return 0
	}

	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}

	if negative {
		val = -val
	}
	return val
}

// findHeaderRow scans lines for a row that contains all marker strings (case-insensitive).
// Returns the row index and a map of trimmed column header -> column index.
// Returns -1 and nil if no matching header row is found.
func findHeaderRow(lines [][]string, markers []string) (int, map[string]int) {
	for i, row := range lines {
		joined := strings.ToUpper(strings.Join(row, "|"))
		allFound := true
		for _, m := range markers {
			if !strings.Contains(joined, strings.ToUpper(m)) {
				allFound = false
				break
			}
		}
		if allFound {
			colMap := make(map[string]int)
			for j, cell := range row {
				trimmed := strings.TrimSpace(cell)
				if trimmed != "" {
					colMap[trimmed] = j
				}
			}
			return i, colMap
		}
	}
	return -1, nil
}

// colGet safely retrieves a column value from a row using the column map.
// Returns empty string if the column is not in the map or the row is too short.
func colGet(row []string, colMap map[string]int, col string) string {
	idx, ok := colMap[col]
	if !ok || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

// readAllCSV reads and parses all records from a CSV file, tolerating
// ragged rows and quoted fields.
func readAllCSV(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	return records, nil
}

// ImportedTransaction represents a single transaction from a CSV import.
type ImportedTransaction struct {
	Symbol string
	Type   TransactionType
	Date   time.Time
	Shares float64
	Price  float64
	Total  float64
	Source string
}

// DetectTransactionCSV checks whether a CSV looks like a transaction history
// (as opposed to positions). It looks for action/type columns.
func DetectTransactionCSV(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1

	for i := 0; i < 30; i++ {
		record, err := reader.Read()
		if err != nil {
			break
		}
		joined := strings.ToUpper(strings.Join(record, "|"))
		if containsAll(joined, "ACTION", "SYMBOL", "QUANTITY", "PRICE") ||
			containsAll(joined, "RUN DATE", "ACTION", "SYMBOL") ||
			containsAll(joined, "DATE", "ACTION", "SYMBOL", "AMOUNT") {
			return true
		}
	}
	return false
}

// ParseTransactionCSV parses transaction history from Schwab or Fidelity CSVs.
func ParseTransactionCSV(path string, broker BrokerageType) ([]ImportedTransaction, error) {
	lines, err := readAllCSV(path)
	if err != nil {
		return nil, fmt.Errorf("read transaction csv: %w", err)
	}

	switch broker {
	case BrokerSchwab:
		return parseSchwabTransactions(lines)
	case BrokerFidelity:
		return parseFidelityTransactions(lines)
	default:
		return nil, fmt.Errorf("transaction import not supported for %s", broker)
	}
}

func parseSchwabTransactions(lines [][]string) ([]ImportedTransaction, error) {
	headerIdx, colMap := findHeaderRow(lines, []string{"Date", "Action", "Symbol", "Quantity", "Price"})
	if headerIdx < 0 {
		return nil, fmt.Errorf("schwab transaction header not found")
	}

	var txns []ImportedTransaction
	for i := headerIdx + 1; i < len(lines); i++ {
		row := lines[i]
		symbol := strings.TrimSpace(colGet(row, colMap, "Symbol"))
		if symbol == "" {
			continue
		}

		action := strings.ToLower(strings.TrimSpace(colGet(row, colMap, "Action")))
		txType := classifyAction(action)
		if txType == "" {
			continue
		}

		dateStr := strings.TrimSpace(colGet(row, colMap, "Date"))
		date := parseDate(dateStr)

		shares := parseNumber(colGet(row, colMap, "Quantity"))
		price := parseNumber(colGet(row, colMap, "Price"))
		amount := parseNumber(colGet(row, colMap, "Amount"))

		total := amount
		if total == 0 {
			total = shares * price
		}

		txns = append(txns, ImportedTransaction{
			Symbol: symbol,
			Type:   txType,
			Date:   date,
			Shares: shares,
			Price:  price,
			Total:  total,
			Source: string(BrokerSchwab),
		})
	}
	return txns, nil
}

func parseFidelityTransactions(lines [][]string) ([]ImportedTransaction, error) {
	headerIdx, colMap := findHeaderRow(lines, []string{"Run Date", "Action", "Symbol"})
	if headerIdx < 0 {
		return nil, fmt.Errorf("fidelity transaction header not found")
	}

	var txns []ImportedTransaction
	for i := headerIdx + 1; i < len(lines); i++ {
		row := lines[i]
		symbol := strings.TrimSpace(colGet(row, colMap, "Symbol"))
		if symbol == "" {
			continue
		}

		action := strings.ToLower(strings.TrimSpace(colGet(row, colMap, "Action")))
		txType := classifyAction(action)
		if txType == "" {
			continue
		}

		dateStr := strings.TrimSpace(colGet(row, colMap, "Run Date"))
		date := parseDate(dateStr)

		shares := parseNumber(colGet(row, colMap, "Quantity"))
		price := parseNumber(colGet(row, colMap, "Price"))
		amount := parseNumber(colGet(row, colMap, "Amount"))

		total := amount
		if total == 0 {
			total = shares * price
		}

		txns = append(txns, ImportedTransaction{
			Symbol: symbol,
			Type:   txType,
			Date:   date,
			Shares: shares,
			Price:  price,
			Total:  total,
			Source: string(BrokerFidelity),
		})
	}
	return txns, nil
}

// classifyAction maps brokerage action strings to transaction types.
func classifyAction(action string) TransactionType {
	action = strings.ToLower(action)
	switch {
	case strings.Contains(action, "buy") || strings.Contains(action, "purchased"):
		return TxBuy
	case strings.Contains(action, "sell") || strings.Contains(action, "sold"):
		return TxSell
	case strings.Contains(action, "dividend") || strings.Contains(action, "div"):
		return TxDividend
	default:
		return ""
	}
}

// parseDate tries common date formats.
func parseDate(s string) time.Time {
	formats := []string{
		"01/02/2006",
		"1/2/2006",
		"2006-01-02",
		"Jan 02, 2006",
		"01/02/2006 as of 01/02/2006",
	}
	s = strings.TrimSpace(s)
	// Handle "MM/DD/YYYY as of MM/DD/YYYY" pattern
	if idx := strings.Index(s, " as of "); idx > 0 {
		s = s[:idx]
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Now()
}
