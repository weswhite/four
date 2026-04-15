package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"
)

// DividendEvent represents a single dividend payment from Yahoo Finance.
type DividendEvent struct {
	Date   time.Time
	Amount float64
}

type YahooQuote struct {
	Symbol          string
	Price           float64
	LastDividend    float64
	DividendFreq    int
	DividendHistory []DividendEvent
	LastUpdated     time.Time
}

// DivGrowthInfo holds year-over-year dividend growth for a symbol.
type DivGrowthInfo struct {
	Symbol       string
	CurrentAnnual float64
	PriorAnnual   float64
	GrowthPct     float64
	IsCut         bool
}

type YahooAgent struct {
	quotes          map[string]YahooQuote
	mu              sync.RWMutex
	updateCh        chan map[string]YahooQuote
	symbols         []string
	refreshInterval time.Duration
	client          *http.Client
	running         bool
	lastErrors      int // count of errors from last fetch cycle
}

type yahooChartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				RegularMarketPrice float64 `json:"regularMarketPrice"`
			} `json:"meta"`
			Events struct {
				Dividends map[string]struct {
					Amount float64 `json:"amount"`
					Date   int64   `json:"date"`
				} `json:"dividends"`
			} `json:"events"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

func NewYahooAgent(symbols []string, refreshSecs int) *YahooAgent {
	if refreshSecs <= 0 {
		refreshSecs = 60
	}
	return &YahooAgent{
		quotes:          make(map[string]YahooQuote),
		updateCh:        make(chan map[string]YahooQuote, 1),
		symbols:         symbols,
		refreshInterval: time.Duration(refreshSecs) * time.Second,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		running: false,
	}
}

func (a *YahooAgent) Start() {
	a.mu.Lock()
	a.running = true
	a.mu.Unlock()

	go func() {
		a.fetchAll()

		ticker := time.NewTicker(a.refreshInterval)
		defer ticker.Stop()

		for {
			a.mu.RLock()
			running := a.running
			a.mu.RUnlock()
			if !running {
				return
			}

			<-ticker.C

			a.mu.RLock()
			running = a.running
			a.mu.RUnlock()
			if !running {
				return
			}

			a.fetchAll()
		}
	}()
}

func (a *YahooAgent) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.running = false
}

func (a *YahooAgent) Updates() <-chan map[string]YahooQuote {
	return a.updateCh
}

func (a *YahooAgent) GetQuote(symbol string) (YahooQuote, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	q, ok := a.quotes[symbol]
	return q, ok
}

func (a *YahooAgent) SetSymbols(symbols []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.symbols = symbols
}

// LastErrors returns the number of fetch errors from the last cycle.
func (a *YahooAgent) LastErrors() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.lastErrors
}

func (a *YahooAgent) fetchAll() {
	a.mu.RLock()
	syms := make([]string, len(a.symbols))
	copy(syms, a.symbols)
	a.mu.RUnlock()

	results := make(map[string]YahooQuote)
	errCount := 0
	for _, sym := range syms {
		q, err := FetchQuote(a.client, sym)
		if err != nil {
			errCount++
			continue
		}
		results[sym] = q
	}

	a.mu.Lock()
	for sym, q := range results {
		a.quotes[sym] = q
	}
	a.lastErrors = errCount
	a.mu.Unlock()

	// Non-blocking send
	select {
	case a.updateCh <- results:
	default:
	}
}

func FetchQuote(client *http.Client, symbol string) (YahooQuote, error) {
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=1d&range=2y", symbol)

	resp, err := client.Get(url)
	if err != nil {
		return YahooQuote{}, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return YahooQuote{}, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return YahooQuote{}, fmt.Errorf("http status %d: %s", resp.StatusCode, string(body))
	}

	var chart yahooChartResponse
	if err := json.Unmarshal(body, &chart); err != nil {
		return YahooQuote{}, fmt.Errorf("json decode: %w", err)
	}

	if chart.Chart.Error != nil {
		return YahooQuote{}, fmt.Errorf("api error: %s - %s", chart.Chart.Error.Code, chart.Chart.Error.Description)
	}

	if len(chart.Chart.Result) == 0 {
		return YahooQuote{}, fmt.Errorf("no results for %s", symbol)
	}

	result := chart.Chart.Result[0]
	price := result.Meta.RegularMarketPrice

	now := time.Now()
	oneYearAgo := now.AddDate(-1, 0, 0)

	var lastDividend float64
	var lastDividendDate int64
	divCount := 0

	// Collect all dividend events
	var divHistory []DividendEvent
	for _, div := range result.Events.Dividends {
		divTime := time.Unix(div.Date, 0)
		divHistory = append(divHistory, DividendEvent{Date: divTime, Amount: div.Amount})
		if divTime.After(oneYearAgo) {
			divCount++
			if div.Date > lastDividendDate {
				lastDividendDate = div.Date
				lastDividend = div.Amount
			}
		}
	}
	// Sort by date ascending
	sort.Slice(divHistory, func(i, j int) bool {
		return divHistory[i].Date.Before(divHistory[j].Date)
	})

	var freq int
	switch {
	case divCount >= 11:
		freq = 12
	case divCount >= 3:
		freq = 4
	case divCount >= 1:
		freq = divCount
	default:
		freq = 0
	}

	return YahooQuote{
		Symbol:          symbol,
		Price:           price,
		LastDividend:    lastDividend,
		DividendFreq:    freq,
		DividendHistory: divHistory,
		LastUpdated:     now,
	}, nil
}

// ComputeDivGrowth calculates year-over-year dividend growth for each symbol.
func ComputeDivGrowth(quotes map[string]YahooQuote) []DivGrowthInfo {
	now := time.Now()
	oneYearAgo := now.AddDate(-1, 0, 0)
	twoYearsAgo := now.AddDate(-2, 0, 0)

	var results []DivGrowthInfo
	for _, q := range quotes {
		if len(q.DividendHistory) == 0 {
			continue
		}
		var current, prior float64
		for _, d := range q.DividendHistory {
			if d.Date.After(oneYearAgo) {
				current += d.Amount
			} else if d.Date.After(twoYearsAgo) {
				prior += d.Amount
			}
		}
		if prior == 0 && current == 0 {
			continue
		}
		growthPct := 0.0
		if prior > 0 {
			growthPct = ((current - prior) / prior) * 100
		}
		results = append(results, DivGrowthInfo{
			Symbol:        q.Symbol,
			CurrentAnnual: current,
			PriorAnnual:   prior,
			GrowthPct:     growthPct,
			IsCut:         current < prior && prior > 0,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		return math.Abs(results[i].GrowthPct) > math.Abs(results[j].GrowthPct)
	})
	return results
}
