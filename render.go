package main

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

const (
	reset     = "\033[0m"
	bold      = "\033[1m"
	dim       = "\033[2m"
	green     = "\033[32m"
	red       = "\033[31m"
	yellow    = "\033[33m"
	cyan      = "\033[36m"
	white     = "\033[37m"
	boldGreen = "\033[1;32m"
	boldRed   = "\033[1;31m"
	boldCyan  = "\033[1;36m"
	boldWhite = "\033[1;37m"
	dimWhite  = "\033[2;37m"
	dimCyan   = "\033[2;36m"
	dimGreen  = "\033[2;32m"
	magenta   = "\033[35m"
	bgWhite   = "\033[47m"
	bgBlue    = "\033[44m"
	home      = "\033[H"
	hideCur   = "\033[?25l"
	showCur   = "\033[?25h"
	clearScr  = "\033[2J"
	clearLine = "\033[2K"
)

type LayoutTier int

const (
	LayoutNarrow LayoutTier = iota // <100 cols
	LayoutMedium                   // 100-130
	LayoutWide                     // >130
)

func getTermSize() (int, int) {
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w < 40 {
		w = 130
	}
	if err != nil || h < 10 {
		h = 40
	}
	return w, h
}

func getTermWidth() int {
	w, _ := getTermSize()
	return w
}

func layoutTier(w int) LayoutTier {
	if w < 100 {
		return LayoutNarrow
	}
	if w < 160 {
		return LayoutMedium
	}
	return LayoutWide
}

// truncLine truncates a string with ANSI codes to fit within maxVisible columns.
// It preserves escape sequences, handles UTF-8 runes, and appends a reset.
func truncLine(s string, maxVisible int) string {
	if maxVisible <= 0 {
		return reset
	}
	var out strings.Builder
	vis := 0
	i := 0
	for i < len(s) {
		// ANSI escape sequence: pass through entirely
		if s[i] == '\033' {
			out.WriteByte(s[i])
			i++
			for i < len(s) {
				out.WriteByte(s[i])
				if (s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z') {
					i++
					break
				}
				i++
			}
			continue
		}
		if vis >= maxVisible {
			break
		}
		// Decode one UTF-8 rune
		r := rune(s[i])
		size := 1
		if r >= 0x80 {
			// Multi-byte UTF-8
			if r&0xE0 == 0xC0 {
				size = 2
			} else if r&0xF0 == 0xE0 {
				size = 3
			} else if r&0xF8 == 0xF0 {
				size = 4
			}
		}
		if i+size > len(s) {
			break
		}
		out.WriteString(s[i : i+size])
		i += size
		vis++
	}
	out.WriteString(reset)
	return out.String()
}

// writeLine writes a single line to b, truncated to w visible columns, with a newline.
func writeLine(b *strings.Builder, w int, s string) {
	b.WriteString(clearLine)
	b.WriteString(truncLine(s, w))
	b.WriteString("\n")
}

type AppState struct {
	Portfolio       *Portfolio
	Config          *Config
	Nav             *NavState
	SymMap          *SymbolMap
	TxStore         *TransactionStore
	Buckets         map[BucketType]*Bucket
	TotalValue      float64
	YahooAgent      *YahooAgent
	Watchlist       *Watchlist
	WatchlistCursor int
	EditMode        bool
	EditRow         int
	EditCol         int // 0=shares, 1=avg price
	EditBuf         string
	ShowHelp        bool
	SortFields      []string
	SortIndex       int
	LegacyMode      bool
}

func NewAppState(p *Portfolio, cfg *Config) *AppState {
	return &AppState{
		Portfolio:  p,
		Config:     cfg,
		Nav:        NewNavState(),
		SymMap:     NewSymbolMap(),
		TxStore:    NewTransactionStore(),
		Buckets:    map[BucketType]*Bucket{},
		SortFields: []string{
			"symbol", "annual_div", "yield", "mkt_value", "gain",
		},
	}
}

func (s *AppState) CurrentSortLabel() string {
	switch s.Config.SortBy {
	case "annual_div":
		return "Annual Div"
	case "yield":
		return "Yield"
	case "mkt_value":
		return "Mkt Value"
	case "gain":
		return "Gain"
	default:
		return "Symbol"
	}
}

func render(state *AppState) {
	if state.LegacyMode || state.Nav == nil {
		renderLegacy(state)
		return
	}
	switch state.Nav.View {
	case ViewDashboard:
		renderDashboard(state)
	case ViewBucketDetail:
		renderBucketDetail(state)
	case ViewHistory:
		renderHistory(state)
	case ViewDividendCal:
		renderDividendCalendar(state)
	case ViewRebalance:
		renderRebalance(state)
	case ViewWatchlist:
		renderWatchlist(state)
	default:
		renderDashboard(state)
	}
}

// flushBuffer takes rendered content, truncates lines, and writes the entire
// frame as a single atomic write to stdout.
func flushBuffer(b *strings.Builder, w int) {
	_, h := getTermSize()
	raw := b.String()
	lines := strings.Split(raw, "\n")
	maxLines := h - 1
	if maxLines < 5 {
		maxLines = 5
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}

	var frame strings.Builder
	frame.WriteString("\033[H") // cursor home

	for i, line := range lines {
		cleaned := strings.ReplaceAll(line, "\033[H", "")
		cleaned = strings.ReplaceAll(cleaned, "\033[2J", "")
		frame.WriteString("\033[2K") // clear this line
		frame.WriteString(truncLine(cleaned, w))
		if i < len(lines)-1 {
			frame.WriteString("\r\n")
		}
	}
	frame.WriteString("\033[J") // clear from cursor to end of screen

	os.Stdout.WriteString(frame.String())
}

// visibleLen returns the number of visible (non-ANSI) characters in a string.
func visibleLen(s string) int {
	n := 0
	i := 0
	for i < len(s) {
		if s[i] == '\033' {
			i++
			for i < len(s) {
				if (s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z') {
					i++
					break
				}
				i++
			}
			continue
		}
		r := rune(s[i])
		size := 1
		if r >= 0x80 {
			if r&0xE0 == 0xC0 {
				size = 2
			} else if r&0xF0 == 0xE0 {
				size = 3
			} else if r&0xF8 == 0xF0 {
				size = 4
			}
		}
		i += size
		n++
	}
	return n
}

// ── Dashboard View ──

func renderDashboard(state *AppState) {
	w := getTermWidth()
	p := state.Portfolio

	var b strings.Builder


	// Header
	renderLine(&b, w)
	b.WriteString(boldCyan + " BUCKET STRATEGY" + reset)
	b.WriteString(dimWhite + "  Investment Tracker" + reset)
	now := time.Now().Format("Jan 02 2006  15:04:05")
	pad := w - 38 - len(now)
	if pad < 1 {
		pad = 1
	}
	b.WriteString(strings.Repeat(" ", pad))
	b.WriteString(dimWhite + now + reset + "\n")
	renderLine(&b, w)

	// Portfolio totals
	totalUnrealized := 0.0
	totalIncome := 0.0
	for _, bkt := range state.Buckets {
		totalUnrealized += bkt.UnrealizedGain
		totalIncome += bkt.AnnualIncome
	}

	gainPct := 0.0
	if p.CostBasis > 0 {
		gainPct = (p.GainLoss / p.CostBasis) * 100
	}

	b.WriteString(fmt.Sprintf("  %sMkt Value%s   %-14s", dimWhite, reset, boldWhite+fmtMoney(state.TotalValue)+reset))
	b.WriteString(fmt.Sprintf("  %sUnrealized%s  %s (%s)", dimWhite, reset, colorMoney(totalUnrealized), colorPct(gainPct)))
	realizedYTD := state.TxStore.RealizedGainYTD()
	b.WriteString(fmt.Sprintf("  %sRealized YTD%s  %s", dimWhite, reset, colorMoney(realizedYTD)))
	b.WriteString(fmt.Sprintf("  %sIncome%s  %s/yr\n", dimWhite, reset, green+fmtMoney(totalIncome)+reset))
	renderLine(&b, w)

	// Bucket cards
	b.WriteString(boldWhite + "  BUCKETS" + reset + "\n\n")

	for i, bt := range AllBuckets {
		bkt := state.Buckets[bt]
		if bkt == nil {
			bkt = &Bucket{Type: bt}
		}
		renderBucketCard(&b, bkt, state, i)
		b.WriteString("\n")
	}

	renderLine(&b, w)

	// Help overlay
	if state.ShowHelp {
		renderHelp(&b, w)
		renderLine(&b, w)
	}

	// Status bar
	renderDashboardStatusBar(&b, state, w)
	flushBuffer(&b, w)
}

func renderBucketCard(b *strings.Builder, bkt *Bucket, state *AppState, index int) {
	color := bkt.Type.Color()
	label := bkt.Type.Label()

	sel := "  "
	if state.Nav != nil && state.Nav.SelectedBucket == index {
		sel = boldWhite + "► " + reset
	}

	monthlyIncome := bkt.AnnualIncome / 12

	unrealPct := 0.0
	if bkt.CostBasis > 0 {
		unrealPct = (bkt.UnrealizedGain / bkt.CostBasis) * 100
	}

	b.WriteString(fmt.Sprintf("%s%s%s%s %s(%d holdings)%s\n",
		sel, color+bold, label, reset, dimWhite, len(bkt.Holdings), reset))
	b.WriteString(fmt.Sprintf("    %s%-14s%s  %s%.1f%% of portfolio%s\n",
		boldWhite, fmtMoney(bkt.MarketValue), reset, dimWhite, bkt.AllocationPct, reset))
	b.WriteString(fmt.Sprintf("    Unrealized: %s (%s)    Realized: %s YTD\n",
		colorMoney(bkt.UnrealizedGain), colorPct(unrealPct), colorMoney(bkt.RealizedGain)))
	b.WriteString(fmt.Sprintf("    %sIncome:%s  %s/yr (%s/mo)",
		dimWhite, reset, green+fmtMoney(bkt.AnnualIncome)+reset, green+fmtMoney(monthlyIncome)+reset))
	if bkt.MonthChange != 0 {
		b.WriteString(fmt.Sprintf("    %sMoM:%s %s (%s)",
			dimWhite, reset, colorMoney(bkt.MonthChange), colorPct(bkt.MonthChangePct)))
	}
	b.WriteString("\n")
}

func renderDashboardStatusBar(b *strings.Builder, state *AppState, w int) {
	left := "  Dashboard"
	if state.Nav != nil {
		if s := state.Nav.Status(); s != "" {
			left += "  " + s
		}
	}

	yahooMark := ""
	if state.YahooAgent != nil {
		if errs := state.YahooAgent.LastErrors(); errs > 0 {
			yahooMark = fmt.Sprintf(" [Y!%d]", errs)
		} else {
			yahooMark = " [Y]"
		}
	}

	hints := "1-4 bucket  d divs  b rebal  a watch  i import  t txn  m history  ? help  q quit"

	pad := w - visibleLen(left) - visibleLen(yahooMark) - visibleLen(hints)
	if pad < 1 {
		pad = 1
	}
	b.WriteString(dimWhite + left + reset + yahooMark + strings.Repeat(" ", pad) + dimWhite + hints + reset + "\n")
}

// ── Bucket Detail View ──

func renderBucketDetail(state *AppState) {
	w := getTermWidth()
	tier := layoutTier(w)

	var b strings.Builder


	bt := state.Nav.CurrentBucket()
	bkt := state.Buckets[bt]
	if bkt == nil {
		bkt = &Bucket{Type: bt}
	}

	// Header
	renderLine(&b, w)
	color := bt.Color()
	b.WriteString(color + bold + " " + bt.Label() + reset)
	b.WriteString(dimWhite + fmt.Sprintf("  %d holdings  %s  Alloc: %.1f%%",
		len(bkt.Holdings), fmtMoney(bkt.MarketValue), bkt.AllocationPct) + reset)
	now := time.Now().Format("15:04:05")
	pad := w - 60 - len(now)
	if pad < 1 {
		pad = 1
	}
	b.WriteString(strings.Repeat(" ", pad))
	b.WriteString(dimWhite + now + reset + "\n")
	renderLine(&b, w)

	// Bucket summary
	unrealPct := 0.0
	if bkt.CostBasis > 0 {
		unrealPct = (bkt.UnrealizedGain / bkt.CostBasis) * 100
	}
	b.WriteString(fmt.Sprintf("  %sUnrealized:%s %s (%s)", dimWhite, reset, colorMoney(bkt.UnrealizedGain), colorPct(unrealPct)))
	b.WriteString(fmt.Sprintf("    %sRealized:%s %s", dimWhite, reset, colorMoney(bkt.RealizedGain)))
	b.WriteString(fmt.Sprintf("    %sIncome:%s %s/yr  %sYield:%s %.2f%%\n",
		dimWhite, reset, green+fmtMoney(bkt.AnnualIncome)+reset,
		dimWhite, reset, bkt.IncomeYield))
	renderLine(&b, w)

	// Holdings table for this bucket
	tmpPortfolio := &Portfolio{
		Holdings:       bkt.Holdings,
		MarketValue:    bkt.MarketValue,
		CostBasis:      bkt.CostBasis,
		TotalAnnualDiv: bkt.AnnualIncome,
	}
	tmpState := &AppState{
		Portfolio: tmpPortfolio,
		Config:    state.Config,
		EditMode:  state.EditMode,
		EditRow:   state.EditRow,
		EditCol:   state.EditCol,
	}
	renderHoldings(&b, tmpState, w, tier)
	renderLine(&b, w)

	// Help overlay
	if state.ShowHelp {
		renderHelp(&b, w)
		renderLine(&b, w)
	}

	// Status bar
	hints := "Esc back  Tab bucket  x move  s sort  e edit  ? help  q quit"
	left := fmt.Sprintf("  %s", state.Nav.ViewLabel())
	p := w - visibleLen(left) - visibleLen(hints)
	if p < 1 {
		p = 1
	}
	b.WriteString(dimWhite + left + strings.Repeat(" ", p) + hints + reset + "\n")
	flushBuffer(&b, w)
}

// ── Dividend Calendar View ──

func renderDividendCalendar(state *AppState) {
	w := getTermWidth()
	var b strings.Builder

	renderLine(&b, w)
	b.WriteString(boldCyan + " DIVIDEND CALENDAR" + reset)
	b.WriteString(dimWhite + fmt.Sprintf("  %d", time.Now().Year()) + reset + "\n")
	renderLine(&b, w)

	// Build calendar from holdings + Yahoo quotes
	quotes := make(map[string]YahooQuote)
	if state.YahooAgent != nil {
		for _, h := range state.Portfolio.Holdings {
			if q, ok := state.YahooAgent.GetQuote(h.Symbol); ok {
				quotes[h.Symbol] = q
			}
		}
	}
	cal := BuildDividendCalendar(state.Portfolio.Holdings, quotes)

	monthNames := []string{"JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC"}

	// Header row
	b.WriteString("  ")
	colW := (w - 4) / 12
	if colW < 10 {
		colW = 10
	}
	for _, mn := range monthNames {
		b.WriteString(fmt.Sprintf("%s%-*s%s", boldWhite, colW, mn, reset))
	}
	b.WriteString("\n")

	// Find max entries per month for row iteration
	maxEntries := 0
	for m := 0; m < 12; m++ {
		if len(cal[m]) > maxEntries {
			maxEntries = len(cal[m])
		}
	}
	if maxEntries > 8 {
		maxEntries = 8
	}

	for row := 0; row < maxEntries; row++ {
		b.WriteString("  ")
		for m := 0; m < 12; m++ {
			if row < len(cal[m]) {
				e := cal[m][row]
				cell := fmt.Sprintf("%s $%.2f", e.Symbol, e.Amount)
				if len(cell) > colW-1 {
					cell = cell[:colW-1]
				}
				b.WriteString(fmt.Sprintf("%s%-*s%s", green, colW, cell, reset))
			} else {
				b.WriteString(strings.Repeat(" ", colW))
			}
		}
		b.WriteString("\n")
	}

	// Monthly totals
	renderLine(&b, w)
	b.WriteString("  ")
	for m := 0; m < 12; m++ {
		total := 0.0
		for _, e := range cal[m] {
			total += e.Amount
		}
		label := fmt.Sprintf("$%.0f", total)
		b.WriteString(fmt.Sprintf("%s%-*s%s", dimWhite, colW, label, reset))
	}
	b.WriteString("\n")
	renderLine(&b, w)

	// Dividend growth table
	if len(quotes) > 0 {
		growth := ComputeDivGrowth(quotes)
		if len(growth) > 0 {
			b.WriteString(boldWhite + "  DIVIDEND GROWTH (YoY)" + reset + "\n")
			b.WriteString(fmt.Sprintf("  %s%-6s %10s %10s %8s%s\n",
				dimWhite, "SYM", "CURRENT", "PRIOR", "GROWTH", reset))
			b.WriteString("  " + dim + strings.Repeat("─", w-4) + reset + "\n")

			showN := len(growth)
			if showN > 15 {
				showN = 15
			}
			for i := 0; i < showN; i++ {
				g := growth[i]
				growthStr := colorPct(g.GrowthPct)
				cutLabel := ""
				if g.IsCut {
					cutLabel = red + " ← CUT" + reset
				}
				b.WriteString(fmt.Sprintf("  %s%-6s%s %s%10s%s %s%10s%s %s%s\n",
					cyan, g.Symbol, reset,
					white, fmtMoney(g.CurrentAnnual), reset,
					dimWhite, fmtMoney(g.PriorAnnual), reset,
					growthStr, cutLabel))
			}
			renderLine(&b, w)
		}
	}

	// Status bar
	hints := "Esc back  ? help  q quit"
	left := "  Dividend Calendar"
	pad := w - visibleLen(left) - visibleLen(hints)
	if pad < 1 {
		pad = 1
	}
	b.WriteString(dimWhite + left + strings.Repeat(" ", pad) + hints + reset + "\n")
	flushBuffer(&b, w)
}

// ── Rebalance View ──

func renderRebalance(state *AppState) {
	w := getTermWidth()
	var b strings.Builder

	renderLine(&b, w)
	b.WriteString(boldCyan + " REBALANCE" + reset + "\n")
	renderLine(&b, w)

	goals := state.Config.BucketGoals
	if len(goals) == 0 {
		b.WriteString("\n")
		b.WriteString(dimWhite + "  No bucket targets configured." + reset + "\n")
		b.WriteString(dimWhite + "  Add target allocations to config.json under \"bucket_goals\":" + reset + "\n")
		b.WriteString(dimWhite + "    {\"cash\": {\"target_pct\": 10}, \"bonds\": {\"target_pct\": 20}, ...}" + reset + "\n")
		b.WriteString("\n")
	} else {
		actions := ComputeRebalance(state.Buckets, goals, state.TotalValue)

		b.WriteString(fmt.Sprintf("  %s%-22s %8s %8s %8s %12s%s\n",
			dimWhite, "BUCKET", "TARGET", "ACTUAL", "DELTA", "ACTION", reset))
		b.WriteString("  " + dim + strings.Repeat("─", w-4) + reset + "\n")

		for _, a := range actions {
			actionStr := ""
			if a.DeltaPct > 0.1 {
				actionStr = red + "Sell " + fmtMoney(a.DeltaDollar) + reset
			} else if a.DeltaPct < -0.1 {
				actionStr = green + "Buy  " + fmtMoney(a.DeltaDollar) + reset
			} else {
				actionStr = dimWhite + "  On target" + reset
			}
			color := a.Bucket.Color()
			b.WriteString(fmt.Sprintf("  %s%-22s%s %7.1f%% %7.1f%% %s %s\n",
				color, a.Bucket.Label(), reset,
				a.TargetPct, a.ActualPct,
				colorPct(a.DeltaPct),
				actionStr))
		}
		renderLine(&b, w)
	}

	// Income goals
	hasIncomeGoals := false
	for _, g := range goals {
		if g.TargetIncome > 0 {
			hasIncomeGoals = true
			break
		}
	}
	if hasIncomeGoals {
		b.WriteString(boldWhite + "  INCOME GOALS" + reset + "\n")
		for _, bt := range AllBuckets {
			g, ok := goals[bt]
			if !ok || g.TargetIncome <= 0 {
				continue
			}
			bkt := state.Buckets[bt]
			actual := 0.0
			if bkt != nil {
				actual = bkt.AnnualIncome
			}
			pct := 0.0
			if g.TargetIncome > 0 {
				pct = (actual / g.TargetIncome) * 100
			}
			barWidth := 30
			filled := int(math.Min(pct/100*float64(barWidth), float64(barWidth)))
			color := bt.Color()
			b.WriteString(fmt.Sprintf("  %s%-18s%s %s/%s/yr  [%s%s%s] %.0f%%\n",
				color, bt.Label(), reset,
				green+fmtMoney(actual)+reset,
				fmtMoney(g.TargetIncome),
				green+strings.Repeat("█", filled)+reset,
				dim+strings.Repeat("░", barWidth-filled)+reset,
				reset,
				pct))
		}
		renderLine(&b, w)
	}

	// Status bar
	hints := "Esc back  ? help  q quit"
	left := "  Rebalance"
	pad := w - visibleLen(left) - visibleLen(hints)
	if pad < 1 {
		pad = 1
	}
	b.WriteString(dimWhite + left + strings.Repeat(" ", pad) + hints + reset + "\n")
	flushBuffer(&b, w)
}

// ── Watchlist View ──

func renderWatchlist(state *AppState) {
	w := getTermWidth()
	var b strings.Builder

	renderLine(&b, w)
	b.WriteString(boldCyan + " WATCHLIST" + reset + "\n")
	renderLine(&b, w)

	wl := state.Watchlist
	if wl == nil || len(wl.Entries) == 0 {
		b.WriteString("\n")
		b.WriteString(dimWhite + "  No watchlist entries. Press + to add a symbol." + reset + "\n")
		b.WriteString("\n")
	} else {
		b.WriteString(fmt.Sprintf("  %s  %-6s %10s %10s %8s %6s%s\n",
			dimWhite, "SYM", "PRICE", "DIV/SHR", "YIELD", "FREQ", reset))
		b.WriteString("  " + dim + strings.Repeat("─", w-4) + reset + "\n")

		for i, entry := range wl.Entries {
			prefix := "  "
			if i == state.WatchlistCursor {
				prefix = boldWhite + "► " + reset
			}

			price := 0.0
			divPerShare := 0.0
			yld := 0.0
			freq := 0
			if state.YahooAgent != nil {
				if q, ok := state.YahooAgent.GetQuote(entry.Symbol); ok {
					price = q.Price
					divPerShare = q.LastDividend
					freq = q.DividendFreq
					annualDiv := q.LastDividend * float64(q.DividendFreq)
					if price > 0 {
						yld = (annualDiv / price) * 100
					}
				}
			}

			b.WriteString(fmt.Sprintf("%s%s%-6s%s %s%10s%s %s%10s%s %s %s%6d%s\n",
				prefix,
				cyan, entry.Symbol, reset,
				white, fmtMoney(price), reset,
				green, fmtDiv(divPerShare), reset,
				yieldColor(yld, fmt.Sprintf("%7.1f%%", yld)),
				dimWhite, freq, reset,
			))
		}
	}

	renderLine(&b, w)

	if state.ShowHelp {
		renderHelp(&b, w)
		renderLine(&b, w)
	}

	// Status bar
	hints := "↑/↓ nav  + add  - remove  Esc back  ? help  q quit"
	left := "  Watchlist"
	if state.Nav != nil {
		if s := state.Nav.Status(); s != "" {
			left += "  " + s
		}
	}
	pad := w - visibleLen(left) - visibleLen(hints)
	if pad < 1 {
		pad = 1
	}
	b.WriteString(dimWhite + left + strings.Repeat(" ", pad) + hints + reset + "\n")
	flushBuffer(&b, w)
}

// ── History View ──

func renderHistory(state *AppState) {
	w := getTermWidth()

	var b strings.Builder


	renderLine(&b, w)
	b.WriteString(boldCyan + " HISTORY" + reset + dimWhite + "  Month-over-Month" + reset + "\n")
	renderLine(&b, w)

	snapshots, err := ListSnapshots()
	if err != nil || len(snapshots) == 0 {
		b.WriteString("  " + dimWhite + "No snapshots yet. Data is captured on the first run each month." + reset + "\n")
	} else {
		b.WriteString(fmt.Sprintf("  %s%-10s %14s %14s %14s%s\n",
			dimWhite, "MONTH", "TOTAL VALUE", "CHANGE", "INCOME", reset))
		b.WriteString("  " + dim + strings.Repeat("─", w-4) + reset + "\n")

		var prevValue float64
		for _, snap := range snapshots {
			changeStr := dimWhite + "         -" + reset
			if prevValue > 0 {
				change := snap.TotalValue - prevValue
				changePct := (change / prevValue) * 100
				changeStr = fmt.Sprintf("%s (%s)", colorMoney(change), colorPct(changePct))
			}

			b.WriteString(fmt.Sprintf("  %-10s %s%14s%s %s %s%14s%s\n",
				cyan+snap.Month+reset,
				white, fmtMoney(snap.TotalValue), reset,
				changeStr,
				green, fmtMoney(snap.TotalIncome), reset,
			))
			prevValue = snap.TotalValue
		}
	}

	renderLine(&b, w)
	hints := "Esc back  q quit"
	left := "  History"
	pad := w - visibleLen(left) - visibleLen(hints)
	if pad < 1 {
		pad = 1
	}
	b.WriteString(dimWhite + left + strings.Repeat(" ", pad) + hints + reset + "\n")
	flushBuffer(&b, w)
}

// ── Legacy View (original single-portfolio) ──

func renderLegacy(state *AppState) {
	w := getTermWidth()
	tier := layoutTier(w)
	p := state.Portfolio
	cfg := state.Config

	var b strings.Builder


	// ── Header ──
	renderLine(&b, w)
	b.WriteString(boldCyan + " DIVIDEND TRACKER" + reset)
	b.WriteString(dimWhite + "  Income Portfolio" + reset)
	now := time.Now().Format("Jan 02 2006  15:04:05")
	hdrLeft := 36
	if p.Modified {
		hdrLeft += 12
	}
	pad := w - hdrLeft - len(now)
	if pad < 1 {
		pad = 1
	}
	if p.Modified {
		b.WriteString(yellow + " [modified]" + reset)
	}
	b.WriteString(strings.Repeat(" ", pad))
	b.WriteString(dimWhite + now + reset + "\n")
	renderLine(&b, w)

	// ── Summary Dashboard ──
	goalPct := 0.0
	if p.AnnualGoal > 0 {
		goalPct = (p.TotalAnnualDiv / p.AnnualGoal) * 100
	}

	gainPct := 0.0
	if p.CostBasis > 0 {
		gainPct = (p.GainLoss / p.CostBasis) * 100
	}

	if tier == LayoutNarrow {
		b.WriteString(fmt.Sprintf("  %sMkt Value%s  %s", dimWhite, reset, boldWhite+fmtMoney(p.MarketValue)+reset))
		b.WriteString(fmt.Sprintf("  %sGain/Loss%s %s (%s)\n", dimWhite, reset, colorMoney(p.GainLoss), colorPct(gainPct)))
		b.WriteString(fmt.Sprintf("  %sCost Basis%s %s", dimWhite, reset, dimWhite+fmtMoney(p.CostBasis)+reset))
		b.WriteString(fmt.Sprintf("  %sCash%s %s\n", dimWhite, reset, fmtMoney(p.Cash)))
	} else {
		b.WriteString(fmt.Sprintf("  %sMkt Value%s   %-14s", dimWhite, reset, boldWhite+fmtMoney(p.MarketValue)+reset))
		b.WriteString(fmt.Sprintf("  %sCost Basis%s  %-14s", dimWhite, reset, dimWhite+fmtMoney(p.CostBasis)+reset))
		b.WriteString(fmt.Sprintf("  %sGain/Loss%s  %s", dimWhite, reset, colorMoney(p.GainLoss)))
		b.WriteString(fmt.Sprintf(" (%s)", colorPct(gainPct)))
		b.WriteString(fmt.Sprintf("  %sCash%s %s\n", dimWhite, reset, fmtMoney(p.Cash)))
	}

	renderLine(&b, w)

	// ── Dividend Income ──
	b.WriteString(boldWhite + "  DIVIDEND INCOME" + reset + "\n")
	if tier == LayoutNarrow {
		b.WriteString(fmt.Sprintf("  %sMonthly%s  %s", dimWhite, reset, boldGreen+fmtMoney(p.ProjectedMonthly)+reset))
		b.WriteString(fmt.Sprintf("  %sAnnual%s  %s", dimWhite, reset, green+fmtMoney(p.TotalAnnualDiv)+reset))
		b.WriteString(fmt.Sprintf("  %sYield%s %s\n", dimWhite, reset, yellow+fmt.Sprintf("%.2f%%", p.WeightedAvgYield)+reset))
	} else {
		b.WriteString(fmt.Sprintf("  %sMonthly%s    %-12s", dimWhite, reset, boldGreen+fmtMoney(p.ProjectedMonthly)+reset))
		b.WriteString(fmt.Sprintf("  %sAnnual%s        %-12s", dimWhite, reset, green+fmtMoney(p.TotalAnnualDiv)+reset))
		b.WriteString(fmt.Sprintf("  %sDaily%s  %-10s", dimWhite, reset, dimGreen+fmtMoney(p.DailyIncome)+reset))
		b.WriteString(fmt.Sprintf("  %sWgt Yield%s  %s\n", dimWhite, reset, yellow+fmt.Sprintf("%.2f%%", p.WeightedAvgYield)+reset))
	}

	// ── Additional Stats (medium/wide) ──
	if tier != LayoutNarrow {
		b.WriteString(fmt.Sprintf("  %sPositions%s  %s%d green%s / %s%d red%s",
			dimWhite, reset, green, p.PositionsGreen, reset, red, p.PositionsRed, reset))
		b.WriteString(fmt.Sprintf("    %sLargest%s  %s%s%s (%.1f%%)",
			dimWhite, reset, cyan, p.LargestPosition, reset, p.LargestPosPct))
		b.WriteString(fmt.Sprintf("    %sBest Yielder%s  %s%s%s %.2f%%",
			dimWhite, reset, cyan, p.BestYielder, reset, p.BestYielderPct))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  %sTop Gainer%s %s%s%s %s",
			dimWhite, reset, cyan, p.TopGainer, reset, colorCompact(p.TopGainerAmt)))
		b.WriteString(fmt.Sprintf("    %sTop Loser%s  %s%s%s %s",
			dimWhite, reset, cyan, p.TopLoser, reset, colorCompact(p.TopLoserAmt)))
		if p.AnnualGoal > 0 {
			coverage := (p.TotalAnnualDiv / p.AnnualGoal) * 100
			b.WriteString(fmt.Sprintf("    %sGoal Coverage%s  %s%.1f%%%s",
				dimWhite, reset, yellow, coverage, reset))
		}
		b.WriteString("\n")
	}

	// ── Goal Progress ──
	if cfg.ShowGoal && p.AnnualGoal > 0 {
		barWidth := 40
		if tier == LayoutNarrow {
			barWidth = w - 40
			if barWidth < 10 {
				barWidth = 10
			}
		}
		filled := int(math.Min(goalPct/100*float64(barWidth), float64(barWidth)))
		b.WriteString(fmt.Sprintf("  %sAnnual Goal%s  %s", dimWhite, reset, fmtMoney(p.AnnualGoal)))
		b.WriteString("  [")
		if filled > 0 {
			b.WriteString(green + strings.Repeat("█", filled) + reset)
		}
		b.WriteString(dim + strings.Repeat("░", barWidth-filled) + reset)
		b.WriteString(fmt.Sprintf("]  %s%.1f%%%s\n", yellow, goalPct, reset))
	}

	renderLine(&b, w)

	// ── Holdings Table ──
	if cfg.ShowHoldings {
		renderHoldings(&b, state, w, tier)
		renderLine(&b, w)
	}

	// ── Sector Allocation ──
	if cfg.ShowSectors && tier != LayoutNarrow {
		renderSectors(&b, p, w)
		renderLine(&b, w)
	}

	// ── Top Payers ──
	if cfg.ShowTopPayers {
		renderTopPayers(&b, p, w, tier)
		renderLine(&b, w)
	}

	// ── Help Overlay ──
	if state.ShowHelp {
		renderHelp(&b, w)
		renderLine(&b, w)
	}

	// ── Status Bar ──
	renderStatusBar(&b, state, w)

	flushBuffer(&b, w)
}

func renderHoldings(b *strings.Builder, state *AppState, w int, tier LayoutTier) {
	_, termH := getTermSize()
	p := state.Portfolio
	cfg := state.Config

	sorted := make([]Holding, len(p.Holdings))
	copy(sorted, p.Holdings)
	sortHoldings(sorted, cfg.SortBy, cfg.SortDesc)

	// Calculate how many lines other sections consume so holdings gets the remainder.
	// Header/summary: ~10, goal bar: 2, dividers: ~6, status: 1, holdings header: 3
	overhead := 22
	if cfg.ShowSectors && tier != LayoutNarrow {
		sectorCount := 0
		seen := map[string]bool{}
		for _, h := range p.Holdings {
			if !seen[h.Sector] {
				seen[h.Sector] = true
				sectorCount++
			}
		}
		overhead += sectorCount + 2 // sector rows + header + divider
	}
	if cfg.ShowTopPayers {
		topN := len(p.Holdings)
		if topN > 10 {
			topN = 10
		}
		overhead += topN + 2 // top payer rows + header + divider
	}
	if state.ShowHelp {
		overhead += 17 // help keys + header + divider
	}
	maxRows := termH - overhead
	if maxRows < 3 {
		maxRows = 3
	}
	showCount := len(sorted)
	if showCount > maxRows {
		showCount = maxRows
	}

	sortDir := "▲"
	if cfg.SortDesc {
		sortDir = "▼"
	}

	b.WriteString(boldWhite + "  HOLDINGS" + reset)
	b.WriteString(dimWhite + fmt.Sprintf("  (%d positions", len(p.Holdings)))
	if showCount < len(sorted) {
		b.WriteString(fmt.Sprintf(", showing %d", showCount))
	}
	b.WriteString(")" + reset)
	b.WriteString(dimWhite + fmt.Sprintf("  sort: %s %s", state.CurrentSortLabel(), sortDir) + reset)
	if state.EditMode {
		b.WriteString(yellow + "  [EDIT MODE]" + reset)
	}
	b.WriteString("\n")

	switch tier {
	case LayoutNarrow:
		b.WriteString(fmt.Sprintf("  %s%-6s %6s %8s %9s %8s %7s%s\n",
			dimWhite, "SYM", "SHRS", "PRICE", "MKT VAL", "ANN DIV", "GAIN", reset))
		b.WriteString("  " + dim + strings.Repeat("─", w-4) + reset + "\n")
		for i := 0; i < showCount; i++ {
			h := sorted[i]
			prefix := "  "
			if state.EditMode && i == state.EditRow {
				prefix = bgBlue + "► " + reset
			}
			b.WriteString(fmt.Sprintf("%s%s %s %s %s %s %s\n",
				prefix,
				cyan+fmt.Sprintf("%-6s", h.Symbol)+reset,
				white+fmt.Sprintf("%6.0f", h.Shares)+reset,
				white+fmt.Sprintf("%8s", fmtCompact(h.CurrentPrice))+reset,
				white+fmt.Sprintf("%9s", fmtCompact(h.MarketValue))+reset,
				green+fmt.Sprintf("%8s", fmtCompact(h.AnnualDivTotal))+reset,
				colorCompact(h.TotalGain),
			))
		}

	case LayoutMedium:
		b.WriteString(fmt.Sprintf("  %s%-6s %-20s %6s %8s %9s %7s %6s %6s %8s %7s%s\n",
			dimWhite, "SYM", "NAME",
			"SHRS", "PRICE", "MKT VAL",
			"LAST$", "PAY/Y", "YLD%",
			"ANN DIV", "GAIN", reset))
		b.WriteString("  " + dim + strings.Repeat("─", w-4) + reset + "\n")
		for i := 0; i < showCount; i++ {
			h := sorted[i]
			prefix := "  "
			if state.EditMode && i == state.EditRow {
				prefix = bgBlue + "► " + reset
			}
			b.WriteString(fmt.Sprintf("%s%s %s %s %s %s %s %s %s %s %s\n",
				prefix,
				cyan+fmt.Sprintf("%-6s", h.Symbol)+reset,
				dimWhite+fmt.Sprintf("%-20s", truncate(h.Name, 20))+reset,
				white+fmt.Sprintf("%6.0f", h.Shares)+reset,
				white+fmt.Sprintf("%8s", fmtCompact(h.CurrentPrice))+reset,
				white+fmt.Sprintf("%9s", fmtCompact(h.MarketValue))+reset,
				green+fmt.Sprintf("%7s", fmtDiv(h.LastDividend))+reset,
				dimWhite+fmt.Sprintf("%6.0f", h.PaymentsPerYear)+reset,
				yieldColor(h.YieldOnCost*100, fmt.Sprintf("%5.2f%%", h.YieldOnCost*100)),
				green+fmt.Sprintf("%8s", fmtCompact(h.AnnualDivTotal))+reset,
				colorCompact(h.TotalGain),
			))
		}

	case LayoutWide:
		b.WriteString(fmt.Sprintf("  %s%-6s %-20s %-16s %6s %8s %9s %9s %7s %6s %6s %8s %8s %7s%s\n",
			dimWhite, "SYM", "NAME", "SECTOR",
			"SHRS", "PRICE", "MKT VAL", "COST",
			"LAST$", "PAY/Y", "YLD%",
			"ANN DIV", "DAY +/-", "GAIN", reset))
		b.WriteString("  " + dim + strings.Repeat("─", w-4) + reset + "\n")
		for i := 0; i < showCount; i++ {
			h := sorted[i]
			prefix := "  "
			if state.EditMode && i == state.EditRow {
				prefix = bgBlue + "► " + reset
			}
			b.WriteString(fmt.Sprintf("%s%s %s %s %s %s %s %s %s %s %s %s %s %s\n",
				prefix,
				cyan+fmt.Sprintf("%-6s", h.Symbol)+reset,
				dimWhite+fmt.Sprintf("%-20s", truncate(h.Name, 20))+reset,
				dimCyan+fmt.Sprintf("%-16s", truncate(h.Sector, 16))+reset,
				white+fmt.Sprintf("%6.0f", h.Shares)+reset,
				white+fmt.Sprintf("%8s", fmtCompact(h.CurrentPrice))+reset,
				white+fmt.Sprintf("%9s", fmtCompact(h.MarketValue))+reset,
				dimWhite+fmt.Sprintf("%9s", fmtCompact(h.CostBasis))+reset,
				green+fmt.Sprintf("%7s", fmtDiv(h.LastDividend))+reset,
				dimWhite+fmt.Sprintf("%6.0f", h.PaymentsPerYear)+reset,
				yieldColor(h.YieldOnCost*100, fmt.Sprintf("%5.2f%%", h.YieldOnCost*100)),
				green+fmt.Sprintf("%8s", fmtCompact(h.AnnualDivTotal))+reset,
				colorCompact(h.DayGain),
				colorCompact(h.TotalGain),
			))
		}
	}
}

func renderSectors(b *strings.Builder, p *Portfolio, w int) {
	sectorDiv := map[string]float64{}
	sectorCount := map[string]int{}
	for _, h := range p.Holdings {
		sectorDiv[h.Sector] += h.AnnualDivTotal
		sectorCount[h.Sector]++
	}

	type sectorEntry struct {
		name string
		div  float64
		cnt  int
	}
	var sectors []sectorEntry
	for name, div := range sectorDiv {
		sectors = append(sectors, sectorEntry{name, div, sectorCount[name]})
	}
	for i := 1; i < len(sectors); i++ {
		for j := i; j > 0 && sectors[j].div > sectors[j-1].div; j-- {
			sectors[j], sectors[j-1] = sectors[j-1], sectors[j]
		}
	}

	maxDiv := 0.0
	for _, s := range sectors {
		if s.div > maxDiv {
			maxDiv = s.div
		}
	}

	b.WriteString(boldWhite + "  DIVIDEND BY SECTOR" + reset + "\n")
	for _, s := range sectors {
		pct := 0.0
		if p.TotalAnnualDiv > 0 {
			pct = (s.div / p.TotalAnnualDiv) * 100
		}
		barLen := 30
		filled := 0
		if maxDiv > 0 {
			filled = int((s.div / maxDiv) * float64(barLen))
		}
		bar := green + strings.Repeat("█", filled) + reset + dim + strings.Repeat("░", barLen-filled) + reset
		b.WriteString(fmt.Sprintf("  %-20s %s  %s%6.1f%%%s  %s  (%d stocks)\n",
			dimWhite+truncate(s.name, 20)+reset,
			bar,
			yellow, pct, reset,
			green+fmtCompact(s.div)+reset,
			s.cnt,
		))
	}
}

func renderTopPayers(b *strings.Builder, p *Portfolio, w int, tier LayoutTier) {
	sorted := make([]Holding, len(p.Holdings))
	copy(sorted, p.Holdings)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].AnnualDivTotal > sorted[j-1].AnnualDivTotal; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	topN := 10
	if len(sorted) < topN {
		topN = len(sorted)
	}
	if topN == 0 {
		return
	}

	maxTopDiv := sorted[0].AnnualDivTotal
	b.WriteString(boldWhite + "  TOP DIVIDEND PAYERS" + reset + dimWhite + "  (by annual income)" + reset + "\n")

	barLen := 25
	if tier == LayoutNarrow {
		barLen = 15
	}

	for i := 0; i < topN; i++ {
		h := sorted[i]
		filled := 0
		if maxTopDiv > 0 {
			filled = int((h.AnnualDivTotal / maxTopDiv) * float64(barLen))
		}
		if filled < 1 {
			filled = 1
		}
		bar := green + strings.Repeat("█", filled) + reset + dim + strings.Repeat("░", barLen-filled) + reset
		b.WriteString(fmt.Sprintf("  %s%-5s%s %s %s%8s%s/yr  %sYOC %5.2f%%%s\n",
			cyan, h.Symbol, reset,
			bar,
			boldGreen, fmtCompact(h.AnnualDivTotal), reset,
			dimWhite, h.YieldOnCost*100, reset,
		))
	}
}

func renderHelp(b *strings.Builder, w int) {
	b.WriteString(boldWhite + "  KEYBINDINGS" + reset + "\n")
	keys := []struct{ key, desc string }{
		{"1-4", "Select / toggle bucket"},
		{"Tab", "Cycle buckets"},
		{"Enter", "Drill into bucket"},
		{"d", "Dividend calendar"},
		{"b", "Rebalance view"},
		{"a", "Watchlist view"},
		{"x", "Move holding to bucket (detail view)"},
		{"+", "Add to watchlist"},
		{"-", "Remove from watchlist"},
		{"i", "Import file"},
		{"t", "Add transaction"},
		{"m", "View history"},
		{"s", "Cycle sort field"},
		{"r", "Reverse sort order"},
		{"e", "Enter/exit edit mode"},
		{"↑/↓", "Navigate"},
		{"←/→", "Select field (edit mode)"},
		{"Esc", "Back / Cancel edit"},
		{"w", "Write changes to xlsx"},
		{"?", "Toggle this help"},
		{"q", "Quit"},
	}
	for _, k := range keys {
		b.WriteString(fmt.Sprintf("  %s%-8s%s %s\n", cyan, k.key, reset, dimWhite+k.desc+reset))
	}
}

func renderStatusBar(b *strings.Builder, state *AppState, w int) {
	src := state.Portfolio.SourceFile

	var hints string
	if state.EditMode {
		hints = "Up/Dn nav  Enter edit  Esc cancel  w write"
	} else {
		hints = "1-4 sort  s sort  r rev  e edit  ? help  q quit"
	}

	maxSrc := w - visibleLen(hints) - 14 // "  Source: " + padding
	if maxSrc < 10 {
		maxSrc = 10
	}
	src = truncate(src, maxSrc)

	left := "  Source: " + src
	pad := w - visibleLen(left) - visibleLen(hints)
	if pad < 1 {
		pad = 1
	}
	b.WriteString(dimWhite + left + strings.Repeat(" ", pad) + hints + reset + "\n")
}

func renderLine(b *strings.Builder, w int) {
	b.WriteString(dim + "  " + strings.Repeat("─", w-4) + reset + "\n")
}
