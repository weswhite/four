package main

import "time"

// WatchlistEntry is a single symbol on the watchlist.
type WatchlistEntry struct {
	Symbol    string    `json:"symbol"`
	Notes     string    `json:"notes,omitempty"`
	AddedDate time.Time `json:"added_date"`
}

// Watchlist holds watched symbols with persistence.
type Watchlist struct {
	Entries []WatchlistEntry `json:"entries"`
}

const watchlistFile = "watchlist.json"

// LoadWatchlist reads the watchlist from disk. Returns an empty list on error.
func LoadWatchlist() *Watchlist {
	wl := &Watchlist{}
	_ = loadJSON(watchlistFile, wl)
	return wl
}

// Save persists the watchlist to disk.
func (wl *Watchlist) Save() error {
	return saveJSON(watchlistFile, wl)
}

// Add appends a symbol if not already present.
func (wl *Watchlist) Add(symbol, notes string) {
	if wl.Has(symbol) {
		return
	}
	wl.Entries = append(wl.Entries, WatchlistEntry{
		Symbol:    symbol,
		Notes:     notes,
		AddedDate: time.Now(),
	})
}

// Remove deletes a symbol from the watchlist.
func (wl *Watchlist) Remove(symbol string) {
	for i, e := range wl.Entries {
		if e.Symbol == symbol {
			wl.Entries = append(wl.Entries[:i], wl.Entries[i+1:]...)
			return
		}
	}
}

// Has returns true if the symbol is on the watchlist.
func (wl *Watchlist) Has(symbol string) bool {
	for _, e := range wl.Entries {
		if e.Symbol == symbol {
			return true
		}
	}
	return false
}

// Symbols returns all watched symbols.
func (wl *Watchlist) Symbols() []string {
	out := make([]string, len(wl.Entries))
	for i, e := range wl.Entries {
		out[i] = e.Symbol
	}
	return out
}
