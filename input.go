package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

type InputEvent int

const (
	EventNone InputEvent = iota
	EventQuit
	EventToggleHoldings
	EventToggleSectors
	EventToggleTopPayers
	EventToggleGoal
	EventCycleSort
	EventReverseSort
	EventToggleEdit
	EventToggleHelp
	EventUp
	EventDown
	EventLeft
	EventRight
	EventEnter
	EventEscape
	EventWrite
	EventTab
	EventImport      // 'i' key
	EventTransaction // 't' key
	EventHistory     // 'm' key
	EventBucket1     // these map to bucket selection
	EventBucket2
	EventBucket3
	EventBucket4
	EventDividendCal  // 'd' key
	EventRebalance    // 'b' key
	EventWatchlist    // 'a' key
	EventMoveBucket   // 'x' key
	EventAddWatchlist // '+' key
	EventRemoveWatchlist // '-' key
)

// readKey reads a single keypress from raw-mode stdin.
// Returns the event type.
func readKey(r *bufio.Reader) InputEvent {
	buf := make([]byte, 3)
	n, err := r.Read(buf)
	if err != nil || n == 0 {
		return EventNone
	}

	// Ctrl+C
	if buf[0] == 3 {
		return EventQuit
	}

	// Escape sequences (arrow keys)
	if buf[0] == 27 {
		if n == 1 {
			return EventEscape
		}
		if n >= 3 && buf[1] == '[' {
			switch buf[2] {
			case 'A':
				return EventUp
			case 'B':
				return EventDown
			case 'C':
				return EventRight
			case 'D':
				return EventLeft
			}
		}
		return EventEscape
	}

	switch buf[0] {
	case 'q':
		return EventQuit
	case '1':
		return EventBucket1
	case '2':
		return EventBucket2
	case '3':
		return EventBucket3
	case '4':
		return EventBucket4
	case 's':
		return EventCycleSort
	case 'r':
		return EventReverseSort
	case 'e':
		return EventToggleEdit
	case '?':
		return EventToggleHelp
	case 'w':
		return EventWrite
	case 'i':
		return EventImport
	case 't':
		return EventTransaction
	case 'm':
		return EventHistory
	case 'd':
		return EventDividendCal
	case 'b':
		return EventRebalance
	case 'a':
		return EventWatchlist
	case 'x':
		return EventMoveBucket
	case '+', '=':
		return EventAddWatchlist
	case '-':
		return EventRemoveWatchlist
	case 9: // Tab
		return EventTab
	case 13: // Enter
		return EventEnter
	}

	return EventNone
}

func handleEvent(ev InputEvent, state *AppState) bool {
	cfg := state.Config
	p := state.Portfolio

	switch ev {
	case EventQuit:
		return true

	case EventBucket1:
		if state.Nav != nil {
			state.Nav.SelectBucket(0)
		}

	case EventBucket2:
		if state.Nav != nil {
			state.Nav.SelectBucket(1)
		}

	case EventBucket3:
		if state.Nav != nil {
			state.Nav.SelectBucket(2)
		}

	case EventBucket4:
		if state.Nav != nil {
			state.Nav.SelectBucket(3)
		}

	case EventTab:
		if state.Nav != nil {
			if state.Nav.View == ViewBucketDetail {
				state.Nav.NextBucket()
			}
		}

	case EventEscape:
		if state.EditMode {
			state.EditMode = false
		} else if state.Nav != nil {
			state.Nav.Back()
		}

	case EventEnter:
		if state.EditMode {
			promptEdit(state)
		} else if state.Nav != nil && state.Nav.View == ViewDashboard {
			state.Nav.SelectBucket(state.Nav.SelectedBucket)
		}

	case EventImport:
		if state.Nav != nil {
			promptImport(state)
		}

	case EventTransaction:
		if state.Nav != nil {
			promptTransaction(state)
		}

	case EventHistory:
		if state.Nav != nil {
			state.Nav.GoTo(ViewHistory)
		}

	case EventDividendCal:
		if state.Nav != nil {
			state.Nav.GoTo(ViewDividendCal)
		}

	case EventRebalance:
		if state.Nav != nil {
			state.Nav.GoTo(ViewRebalance)
		}

	case EventWatchlist:
		if state.Nav != nil {
			state.Nav.GoTo(ViewWatchlist)
		}

	case EventMoveBucket:
		if state.Nav != nil && state.Nav.View == ViewBucketDetail {
			promptMoveBucket(state)
		}

	case EventAddWatchlist:
		if state.Nav != nil && state.Nav.View == ViewWatchlist {
			promptAddWatchlist(state)
		}

	case EventRemoveWatchlist:
		if state.Nav != nil && state.Nav.View == ViewWatchlist {
			removeSelectedWatchlist(state)
		}

	case EventCycleSort:
		for i, f := range state.SortFields {
			if f == cfg.SortBy {
				state.SortIndex = (i + 1) % len(state.SortFields)
				break
			}
		}
		cfg.SortBy = state.SortFields[state.SortIndex]
		_ = cfg.Save()

	case EventReverseSort:
		cfg.SortDesc = !cfg.SortDesc
		_ = cfg.Save()

	case EventToggleEdit:
		state.EditMode = !state.EditMode
		if state.EditMode {
			state.EditRow = 0
			state.EditCol = 0
		}

	case EventToggleHelp:
		state.ShowHelp = !state.ShowHelp

	case EventUp:
		if state.EditMode && state.EditRow > 0 {
			state.EditRow--
		} else if state.Nav != nil && state.Nav.View == ViewWatchlist && state.Watchlist != nil {
			if state.WatchlistCursor > 0 {
				state.WatchlistCursor--
			}
		} else if state.Nav != nil && state.Nav.View == ViewDashboard {
			state.Nav.PrevBucket()
		}

	case EventDown:
		if state.EditMode && state.EditRow < len(p.Holdings)-1 {
			state.EditRow++
		} else if state.Nav != nil && state.Nav.View == ViewWatchlist && state.Watchlist != nil {
			if state.WatchlistCursor < len(state.Watchlist.Entries)-1 {
				state.WatchlistCursor++
			}
		} else if state.Nav != nil && state.Nav.View == ViewDashboard {
			state.Nav.NextBucket()
		}

	case EventLeft:
		if state.EditMode && state.EditCol > 0 {
			state.EditCol--
		}

	case EventRight:
		if state.EditMode && state.EditCol < 1 {
			state.EditCol++
		}

	case EventWrite:
		if p.Modified {
			writeToXlsx(state)
		}
	}

	return false
}

func promptImport(state *AppState) {
	fmt.Print(showCur)
	fmt.Printf("\033[999B")
	fmt.Printf(clearLine)
	fmt.Printf("  %sImport file path: %s", yellow, white)

	var input []byte
	buf := make([]byte, 1)
	for {
		_, err := os.Stdin.Read(buf)
		if err != nil {
			break
		}
		if buf[0] == 13 || buf[0] == 10 {
			break
		}
		if buf[0] == 27 {
			fmt.Print(hideCur)
			return
		}
		if buf[0] == 127 || buf[0] == 8 {
			if len(input) > 0 {
				input = input[:len(input)-1]
				fmt.Print("\b \b")
			}
			continue
		}
		input = append(input, buf[0])
		fmt.Print(string(buf[0]))
	}
	fmt.Print(reset + hideCur)

	path := strings.TrimSpace(string(input))
	path = strings.ReplaceAll(path, "\\ ", " ")
	path = strings.Trim(path, "'\"")
	if path == "" {
		return
	}

	// Detect and parse
	if strings.HasSuffix(strings.ToLower(path), ".xlsx") {
		imported, err := ParseXlsx(path)
		if err != nil {
			state.Nav.SetStatus("Import error: "+err.Error(), 5*time.Second)
			return
		}
		importHoldings(state, imported)
		return
	}

	// Check if this is a transaction CSV
	if DetectTransactionCSV(path) {
		broker, detectErr := DetectBrokerageFormat(path)
		if detectErr != nil {
			// Try Schwab as default for transactions
			broker = BrokerSchwab
		}
		txns, err := ParseTransactionCSV(path, broker)
		if err != nil {
			state.Nav.SetStatus("Import error: "+err.Error(), 5*time.Second)
			return
		}

		// Check for unknown symbols and prompt assignment
		symSet := map[string]bool{}
		for _, tx := range txns {
			symSet[tx.Symbol] = true
		}
		var syms []string
		for s := range symSet {
			syms = append(syms, s)
		}
		unknown := state.SymMap.UnknownSymbols(syms)
		for _, sym := range unknown {
			assignBucketPrompt(state, sym)
		}

		// Add each transaction
		for _, itx := range txns {
			tx := Transaction{
				Symbol: itx.Symbol,
				Type:   itx.Type,
				Date:   itx.Date,
				Shares: itx.Shares,
				Price:  itx.Price,
				Total:  itx.Total,
				Source: itx.Source,
			}
			state.TxStore.AddTransaction(tx)
		}
		_ = state.TxStore.Save()
		_ = state.SymMap.Save()

		// Recompute
		realizedByBucket := state.TxStore.RealizedGainByBucket(state.SymMap)
		prevValues := PreviousMonthValues()
		state.Buckets, state.TotalValue = AggregateBuckets(state.Portfolio.Holdings, state.SymMap, realizedByBucket, prevValues)

		state.Nav.SetStatus(fmt.Sprintf("Imported %d transactions from %s", len(txns), broker), 5*time.Second)
		return
	}

	// Otherwise, treat as positions CSV
	broker, detectErr := DetectBrokerageFormat(path)
	if detectErr != nil {
		state.Nav.SetStatus("Error: "+detectErr.Error(), 5*time.Second)
		return
	}
	imported, err := ParseCSV(path, broker)
	if err != nil {
		state.Nav.SetStatus("Import error: "+err.Error(), 5*time.Second)
		return
	}
	importHoldings(state, imported)
	state.Nav.SetStatus(fmt.Sprintf("Imported %d holdings from %s", len(imported), broker), 5*time.Second)
}

// importHoldings handles the common flow of importing position holdings.
func importHoldings(state *AppState, imported []ImportedHolding) {
	symbols := make([]string, len(imported))
	for i, h := range imported {
		symbols[i] = h.Symbol
	}
	unknown := state.SymMap.UnknownSymbols(symbols)
	for _, sym := range unknown {
		assignBucketPrompt(state, sym)
	}

	holdings := HoldingsFromImported(imported, state.SymMap)
	state.Portfolio.Holdings = MergeHoldings(state.Portfolio.Holdings, holdings)
	state.Portfolio.recomputeAll()

	realizedByBucket := state.TxStore.RealizedGainByBucket(state.SymMap)
	prevValues := PreviousMonthValues()
	state.Buckets, state.TotalValue = AggregateBuckets(state.Portfolio.Holdings, state.SymMap, realizedByBucket, prevValues)

	_ = state.SymMap.Save()
	state.Nav.SetStatus(fmt.Sprintf("Imported %d holdings", len(imported)), 5*time.Second)
}

func assignBucketPrompt(state *AppState, symbol string) {
	fmt.Print(showCur)
	fmt.Printf("\033[999B")
	fmt.Printf(clearLine)
	fmt.Printf("  %s%s%s", yellow, BucketPromptText(symbol), white)
	fmt.Printf(" ")

	buf := make([]byte, 1)
	for {
		_, err := os.Stdin.Read(buf)
		if err != nil {
			break
		}
		switch buf[0] {
		case '1':
			state.SymMap.Assign(symbol, BucketCash, "manual")
			fmt.Print(reset + hideCur)
			return
		case '2':
			state.SymMap.Assign(symbol, BucketBonds, "manual")
			fmt.Print(reset + hideCur)
			return
		case '3':
			state.SymMap.Assign(symbol, BucketEquityIncome, "manual")
			fmt.Print(reset + hideCur)
			return
		case '4':
			state.SymMap.Assign(symbol, BucketLongTermEquity, "manual")
			fmt.Print(reset + hideCur)
			return
		case 27: // Escape - default to equity income
			state.SymMap.Assign(symbol, BucketEquityIncome, "default")
			fmt.Print(reset + hideCur)
			return
		}
	}
	fmt.Print(reset + hideCur)
}

func promptTransaction(state *AppState) {
	fmt.Print(showCur)

	// Prompt for type
	fmt.Printf("\033[999B")
	fmt.Printf(clearLine)
	fmt.Printf("  %sTransaction type [b]uy / [s]ell / [d]ividend: %s", yellow, white)

	buf := make([]byte, 1)
	_, err := os.Stdin.Read(buf)
	if err != nil || buf[0] == 27 {
		fmt.Print(reset + hideCur)
		return
	}

	var txType TransactionType
	switch buf[0] {
	case 'b':
		txType = TxBuy
	case 's':
		txType = TxSell
	case 'd':
		txType = TxDividend
	default:
		fmt.Print(reset + hideCur)
		return
	}

	// Prompt for symbol
	symbol := promptLine("Symbol")
	if symbol == "" {
		fmt.Print(reset + hideCur)
		return
	}
	symbol = strings.ToUpper(strings.TrimSpace(symbol))

	// Prompt for shares (not needed for dividends in $)
	sharesStr := promptLine("Shares")
	shares, _ := strconv.ParseFloat(strings.TrimSpace(sharesStr), 64)

	// Prompt for price/amount
	priceLabel := "Price per share"
	if txType == TxDividend {
		priceLabel = "Total amount"
	}
	priceStr := promptLine(priceLabel)
	price, _ := strconv.ParseFloat(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(priceStr), "$", ""), ",", ""), 64)

	fmt.Print(reset + hideCur)

	total := shares * price
	if txType == TxDividend {
		total = price // for dividends, price IS the total
	}

	tx := Transaction{
		Symbol: symbol,
		Type:   txType,
		Date:   time.Now(),
		Shares: shares,
		Price:  price,
		Total:  total,
		Source: "manual",
	}

	state.TxStore.AddTransaction(tx)
	_ = state.TxStore.Save()

	// Check if symbol needs bucket assignment
	if !state.SymMap.IsKnown(symbol) {
		assignBucketPrompt(state, symbol)
		_ = state.SymMap.Save()
	}

	// Recompute
	realizedByBucket := state.TxStore.RealizedGainByBucket(state.SymMap)
	prevValues := PreviousMonthValues()
	state.Buckets, state.TotalValue = AggregateBuckets(state.Portfolio.Holdings, state.SymMap, realizedByBucket, prevValues)

	if state.Nav != nil {
		state.Nav.SetStatus(fmt.Sprintf("Recorded %s %s %.0f shares", txType, symbol, shares), 5*time.Second)
	}
}

func promptLine(label string) string {
	fmt.Printf("\033[999B")
	fmt.Printf(clearLine)
	fmt.Printf("  %s%s: %s", yellow, label, white)

	var input []byte
	buf := make([]byte, 1)
	for {
		_, err := os.Stdin.Read(buf)
		if err != nil {
			break
		}
		if buf[0] == 13 || buf[0] == 10 {
			break
		}
		if buf[0] == 27 {
			return ""
		}
		if buf[0] == 127 || buf[0] == 8 {
			if len(input) > 0 {
				input = input[:len(input)-1]
				fmt.Print("\b \b")
			}
			continue
		}
		input = append(input, buf[0])
		fmt.Print(string(buf[0]))
	}
	return string(input)
}

func promptEdit(state *AppState) {
	p := state.Portfolio
	if state.EditRow >= len(p.Holdings) {
		return
	}

	// Temporarily show cursor and restore normal mode for input
	fmt.Print(showCur)

	h := &p.Holdings[state.EditRow]
	var fieldName string
	var currentVal float64
	switch state.EditCol {
	case 0:
		fieldName = "Shares"
		currentVal = h.Shares
	case 1:
		fieldName = "Avg Price"
		currentVal = h.AvgPrice
	}

	// Move to bottom and prompt
	fmt.Printf("\033[999B") // move to bottom
	fmt.Printf(clearLine)
	fmt.Printf("  %sEdit %s for %s%s (current: %.2f): %s",
		yellow, fieldName, h.Symbol, reset, currentVal, white)

	// Read line in cooked mode — we temporarily leave raw mode
	// We can't easily switch modes, so read byte-by-byte until Enter
	var input []byte
	buf := make([]byte, 1)
	for {
		_, err := os.Stdin.Read(buf)
		if err != nil {
			break
		}
		if buf[0] == 13 || buf[0] == 10 { // Enter
			break
		}
		if buf[0] == 27 { // Escape
			fmt.Print(hideCur)
			return
		}
		if buf[0] == 127 || buf[0] == 8 { // Backspace
			if len(input) > 0 {
				input = input[:len(input)-1]
				fmt.Print("\b \b")
			}
			continue
		}
		input = append(input, buf[0])
		fmt.Print(string(buf[0]))
	}

	fmt.Print(reset + hideCur)

	val, err := strconv.ParseFloat(strings.TrimSpace(string(input)), 64)
	if err != nil {
		return
	}

	switch state.EditCol {
	case 0:
		h.Shares = val
	case 1:
		h.AvgPrice = val
	}

	h.recompute()
	p.recomputeAll()
	p.Modified = true
}

func writeToXlsx(state *AppState) {
	p := state.Portfolio

	fmt.Print(showCur)
	fmt.Printf("\033[999B")
	fmt.Printf(clearLine)
	fmt.Printf("  %sWrite changes to %s? (y/n): %s", yellow, p.SourceFile, white)

	buf := make([]byte, 1)
	_, err := os.Stdin.Read(buf)
	fmt.Print(reset + hideCur)
	if err != nil || (buf[0] != 'y' && buf[0] != 'Y') {
		return
	}

	f, err := excelize.OpenFile(p.SourceFile)
	if err != nil {
		return
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	for i, h := range p.Holdings {
		row := 16 + i
		_ = f.SetCellFloat(sheet, fmt.Sprintf("E%d", row), h.Shares, 0, 64)
		_ = f.SetCellFloat(sheet, fmt.Sprintf("F%d", row), h.AvgPrice, 2, 64)
	}

	if err := f.Save(); err != nil {
		return
	}

	p.Modified = false
}

// promptMoveBucket reassigns the currently selected holding to a different bucket.
func promptMoveBucket(state *AppState) {
	bt := state.Nav.CurrentBucket()
	bkt := state.Buckets[bt]
	if bkt == nil || len(bkt.Holdings) == 0 {
		return
	}

	// Show the list of holdings and prompt which to move
	fmt.Print(showCur)
	fmt.Printf("\033[999B")
	fmt.Printf(clearLine)
	fmt.Printf("  %sEnter symbol to move: %s", yellow, white)

	symbol := ""
	var input []byte
	buf := make([]byte, 1)
	for {
		_, err := os.Stdin.Read(buf)
		if err != nil {
			break
		}
		if buf[0] == 13 || buf[0] == 10 {
			break
		}
		if buf[0] == 27 {
			fmt.Print(reset + hideCur)
			return
		}
		if buf[0] == 127 || buf[0] == 8 {
			if len(input) > 0 {
				input = input[:len(input)-1]
				fmt.Print("\b \b")
			}
			continue
		}
		input = append(input, buf[0])
		fmt.Print(string(buf[0]))
	}
	symbol = strings.ToUpper(strings.TrimSpace(string(input)))
	if symbol == "" {
		fmt.Print(reset + hideCur)
		return
	}

	// Verify the symbol is in this bucket
	found := false
	for _, h := range bkt.Holdings {
		if h.Symbol == symbol {
			found = true
			break
		}
	}
	if !found {
		fmt.Print(reset + hideCur)
		state.Nav.SetStatus(symbol+" not found in this bucket", 3*time.Second)
		return
	}

	// Prompt for target bucket
	assignBucketPrompt(state, symbol)
	_ = state.SymMap.Save()

	// Recompute buckets
	realizedByBucket := state.TxStore.RealizedGainByBucket(state.SymMap)
	prevValues := PreviousMonthValues()
	state.Buckets, state.TotalValue = AggregateBuckets(state.Portfolio.Holdings, state.SymMap, realizedByBucket, prevValues)

	state.Nav.SetStatus(fmt.Sprintf("Moved %s to %s", symbol, state.SymMap.Lookup(symbol).Label()), 3*time.Second)
}

// promptAddWatchlist prompts for a symbol and adds it to the watchlist.
func promptAddWatchlist(state *AppState) {
	if state.Watchlist == nil {
		return
	}

	fmt.Print(showCur)
	symbol := promptLine("Add symbol")
	fmt.Print(reset + hideCur)

	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return
	}

	state.Watchlist.Add(symbol, "")
	_ = state.Watchlist.Save()

	// Register with Yahoo agent so we get price data
	if state.YahooAgent != nil {
		allSyms := make([]string, len(state.Portfolio.Holdings))
		for i, h := range state.Portfolio.Holdings {
			allSyms[i] = h.Symbol
		}
		allSyms = append(allSyms, state.Watchlist.Symbols()...)
		state.YahooAgent.SetSymbols(allSyms)
	}

	state.Nav.SetStatus(fmt.Sprintf("Added %s to watchlist", symbol), 3*time.Second)
}

// removeSelectedWatchlist removes the entry at the current cursor position.
func removeSelectedWatchlist(state *AppState) {
	if state.Watchlist == nil || len(state.Watchlist.Entries) == 0 {
		return
	}
	if state.WatchlistCursor >= len(state.Watchlist.Entries) {
		state.WatchlistCursor = len(state.Watchlist.Entries) - 1
	}
	sym := state.Watchlist.Entries[state.WatchlistCursor].Symbol
	state.Watchlist.Remove(sym)
	_ = state.Watchlist.Save()
	if state.WatchlistCursor >= len(state.Watchlist.Entries) && state.WatchlistCursor > 0 {
		state.WatchlistCursor--
	}
	state.Nav.SetStatus(fmt.Sprintf("Removed %s from watchlist", sym), 3*time.Second)
}
