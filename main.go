package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"
)

func main() {
	source := flag.String("source", "", "Path to portfolio xlsx file")
	setSource := flag.String("set-source", "", "Set default source file in config and exit")
	importFile := flag.String("import", "", "Import a CSV/XLSX file into bucket tracker")
	legacy := flag.Bool("legacy", false, "Force legacy single-portfolio view")
	debug := flag.Bool("debug", false, "Print terminal size and exit")
	flag.Parse()

	if *debug {
		w, h := getTermSize()
		fmt.Printf("terminal: %dx%d (cols x rows)\n", w, h)
		fmt.Printf("layout tier: %v\n", layoutTier(w))
		return
	}

	cfg := LoadConfig()

	// --set-source: update config and exit
	if *setSource != "" {
		cfg.SourceFile = *setSource
		if err := cfg.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Default source set to: %s\n", *setSource)
		return
	}

	// Resolve source file: flag > positional arg > config
	filePath := *source
	if filePath == "" && flag.NArg() > 0 {
		filePath = flag.Arg(0)
	}
	if filePath == "" {
		filePath = cfg.SourceFile
	}

	// If no source and no import, show usage
	if filePath == "" && *importFile == "" {
		fmt.Println("Usage: four [options] [portfolio.xlsx]")
		fmt.Println()
		fmt.Println("Options:")
		fmt.Println("  --source <path>       Path to portfolio xlsx file")
		fmt.Println("  --set-source <path>   Set default source and exit")
		fmt.Println("  --import <path>       Import CSV/XLSX into bucket tracker")
		fmt.Println("  --legacy              Force legacy single-portfolio view")
		fmt.Println()
		fmt.Println("Drag and drop an Excel file onto this command.")
		fmt.Println("After first run, the source is remembered in ~/.config/four/config.json")
		os.Exit(1)
	}

	// Handle shell-escaped paths from drag-and-drop
	if filePath != "" {
		filePath = strings.TrimSpace(filePath)
		filePath = strings.ReplaceAll(filePath, "\\ ", " ")
	}

	// Load portfolio from xlsx if provided
	var portfolio *Portfolio
	if filePath != "" {
		p, err := loadPortfolio(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading %s: %v\n", filePath, err)
			os.Exit(1)
		}
		portfolio = p
		cfg.SourceFile = filePath
		_ = cfg.Save()
	} else {
		portfolio = &Portfolio{}
	}

	// Initialize state
	state := NewAppState(portfolio, cfg)

	// Load persisted data
	state.SymMap = LoadSymbolMap()
	state.TxStore = LoadTransactionStore()
	state.Watchlist = LoadWatchlist()

	// Determine mode — only legacy if explicitly requested
	if *legacy {
		state.LegacyMode = true
	}

	// Auto-assign xlsx holdings to buckets if no symbol map exists yet
	if !state.LegacyMode && filePath != "" && len(state.SymMap.Entries) == 0 {
		for _, h := range portfolio.Holdings {
			if !state.SymMap.IsKnown(h.Symbol) {
				// Default: assign to equity income bucket
				state.SymMap.Assign(h.Symbol, BucketEquityIncome, "auto")
			}
		}
		_ = state.SymMap.Save()
	}

	// Handle --import flag
	if *importFile != "" {
		state.LegacyMode = false
		importPath := strings.TrimSpace(*importFile)
		importPath = strings.ReplaceAll(importPath, "\\ ", " ")

		var imported []ImportedHolding
		var err error

		if strings.HasSuffix(strings.ToLower(importPath), ".xlsx") {
			imported, err = ParseXlsx(importPath)
		} else {
			broker, detectErr := DetectBrokerageFormat(importPath)
			if detectErr != nil {
				fmt.Fprintf(os.Stderr, "Error detecting format: %v\n", detectErr)
				os.Exit(1)
			}
			imported, err = ParseCSV(importPath, broker)
			if err == nil {
				fmt.Printf("Detected %s format, %d holdings\n", broker, len(imported))
			}
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error importing: %v\n", err)
			os.Exit(1)
		}

		// Prompt for unknown symbols before entering raw mode
		symbols := make([]string, len(imported))
		for i, h := range imported {
			symbols[i] = h.Symbol
		}
		unknown := state.SymMap.UnknownSymbols(symbols)
		if len(unknown) > 0 {
			fmt.Printf("Found %d new symbols that need bucket assignment.\n", len(unknown))
			fmt.Println("Buckets: [1] Cash  [2] Bonds  [3] Equity Income  [4] Long-Term Equity")
			reader := bufio.NewReader(os.Stdin)
			for _, sym := range unknown {
				fmt.Printf("  %s -> ", sym)
				b, _ := reader.ReadByte()
				switch b {
				case '1':
					state.SymMap.Assign(sym, BucketCash, "import")
				case '2':
					state.SymMap.Assign(sym, BucketBonds, "import")
				case '3':
					state.SymMap.Assign(sym, BucketEquityIncome, "import")
				case '4':
					state.SymMap.Assign(sym, BucketLongTermEquity, "import")
				default:
					state.SymMap.Assign(sym, BucketEquityIncome, "default")
				}
				fmt.Printf("%s\n", state.SymMap.Lookup(sym).Label())
			}
			_ = state.SymMap.Save()
		}

		// Convert and merge
		holdings := HoldingsFromImported(imported, state.SymMap)
		state.Portfolio.Holdings = MergeHoldings(state.Portfolio.Holdings, holdings)
		state.Portfolio.recomputeAll()
	}

	// Compute buckets
	if !state.LegacyMode {
		realizedByBucket := state.TxStore.RealizedGainByBucket(state.SymMap)
		prevValues := PreviousMonthValues()
		state.Buckets, state.TotalValue = AggregateBuckets(state.Portfolio.Holdings, state.SymMap, realizedByBucket, prevValues)

		// Auto-snapshot
		_ = AutoSnapshot(state.Buckets, state.TotalValue)
	}

	// Switch terminal to raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error entering raw mode: %v\n", err)
		os.Exit(1)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	// Clear screen + scrollback, hide cursor. \033[3J clears scrollback buffer.
	fmt.Print("\033[3J" + clearScr + hideCur + home)
	defer fmt.Print(showCur + clearScr + "\033[3J" + home)

	// Start Yahoo Finance agent if not in legacy mode
	if !state.LegacyMode {
		symbols := make([]string, 0, len(state.Portfolio.Holdings)+len(state.Watchlist.Entries))
		for _, h := range state.Portfolio.Holdings {
			symbols = append(symbols, h.Symbol)
		}
		// Merge watchlist symbols (avoid duplicates)
		seen := map[string]bool{}
		for _, s := range symbols {
			seen[s] = true
		}
		for _, s := range state.Watchlist.Symbols() {
			if !seen[s] {
				symbols = append(symbols, s)
				seen[s] = true
			}
		}
		if len(symbols) > 0 {
			refreshSecs := cfg.YahooRefreshSecs
			if refreshSecs == 0 {
				refreshSecs = 60
			}
			state.YahooAgent = NewYahooAgent(symbols, refreshSecs)
			state.YahooAgent.Start()
			defer state.YahooAgent.Stop()
		}
	}

	// Initial render
	render(state)

	// Signal handling
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	// Key input channel
	keyCh := make(chan InputEvent, 1)
	reader := bufio.NewReader(os.Stdin)
	go func() {
		for {
			ev := readKey(reader)
			if ev != EventNone {
				keyCh <- ev
			}
		}
	}()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Yahoo update channel
	var yahooCh <-chan map[string]YahooQuote
	if state.YahooAgent != nil {
		yahooCh = state.YahooAgent.Updates()
	}

	// Debounce: skip ticker renders within 500ms of a data/event render
	lastRender := time.Now()

	for {
		select {
		case <-sig:
			return
		case ev := <-keyCh:
			if handleEvent(ev, state) {
				return
			}
			render(state)
			lastRender = time.Now()
		case quotes := <-yahooCh:
			ApplyYahooQuotes(state.Portfolio.Holdings, quotes)
			state.Portfolio.recomputeAll()
			if !state.LegacyMode {
				realizedByBucket := state.TxStore.RealizedGainByBucket(state.SymMap)
				prevValues := PreviousMonthValues()
				state.Buckets, state.TotalValue = AggregateBuckets(state.Portfolio.Holdings, state.SymMap, realizedByBucket, prevValues)
			}
			render(state)
			lastRender = time.Now()
		case <-ticker.C:
			// Skip if we just rendered from a real event
			if time.Since(lastRender) < 500*time.Millisecond {
				continue
			}
			render(state)
		}
	}
}
