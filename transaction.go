package main

import (
	"fmt"
	"sort"
	"time"
)

type TransactionType string

const (
	TxBuy      TransactionType = "buy"
	TxSell     TransactionType = "sell"
	TxDividend TransactionType = "dividend"
)

type Transaction struct {
	ID     string          `json:"id"`
	Symbol string          `json:"symbol"`
	Type   TransactionType `json:"type"`
	Date   time.Time       `json:"date"`
	Shares float64         `json:"shares"`
	Price  float64         `json:"price"`
	Total  float64         `json:"total"`
	Source string          `json:"source,omitempty"`
}

type TaxLot struct {
	Symbol    string    `json:"symbol"`
	Date      time.Time `json:"date"`
	Shares    float64   `json:"shares"`
	CostBasis float64  `json:"cost_basis"`
}

type RealizedGain struct {
	Symbol    string    `json:"symbol"`
	SellDate  time.Time `json:"sell_date"`
	Shares    float64   `json:"shares"`
	Proceeds  float64   `json:"proceeds"`
	CostBasis float64  `json:"cost_basis"`
	Gain      float64   `json:"gain"`
	LongTerm  bool      `json:"long_term"`
}

type TransactionStore struct {
	Transactions  []Transaction  `json:"transactions"`
	OpenLots      []TaxLot       `json:"open_lots"`
	RealizedGains []RealizedGain `json:"realized_gains"`
}

func generateTxID() string {
	return fmt.Sprintf("tx_%d", time.Now().UnixNano())
}

func NewTransactionStore() *TransactionStore {
	return &TransactionStore{
		Transactions:  []Transaction{},
		OpenLots:      []TaxLot{},
		RealizedGains: []RealizedGain{},
	}
}

func LoadTransactionStore() *TransactionStore {
	ts := NewTransactionStore()

	var txs []Transaction
	if err := loadJSON("transactions.json", &txs); err == nil {
		ts.Transactions = txs
	}

	var lots []TaxLot
	if err := loadJSON("lots.json", &lots); err == nil {
		ts.OpenLots = lots
	}

	return ts
}

func (ts *TransactionStore) Save() error {
	if err := saveJSON("transactions.json", ts.Transactions); err != nil {
		return err
	}
	return saveJSON("lots.json", ts.OpenLots)
}

func (ts *TransactionStore) AddTransaction(tx Transaction) {
	if tx.ID == "" {
		tx.ID = generateTxID()
	}

	ts.Transactions = append(ts.Transactions, tx)

	switch tx.Type {
	case TxBuy:
		ts.addLot(tx)
	case TxSell:
		ts.consumeLots(tx)
	case TxDividend:
		// just recorded
	}
}

func (ts *TransactionStore) addLot(tx Transaction) {
	lot := TaxLot{
		Symbol:    tx.Symbol,
		Date:      tx.Date,
		Shares:    tx.Shares,
		CostBasis: tx.Price,
	}
	ts.OpenLots = append(ts.OpenLots, lot)
}

func (ts *TransactionStore) consumeLots(tx Transaction) {
	// Sort open lots for this symbol by date (oldest first) - FIFO
	sort.Slice(ts.OpenLots, func(i, j int) bool {
		if ts.OpenLots[i].Symbol != tx.Symbol && ts.OpenLots[j].Symbol != tx.Symbol {
			return false
		}
		if ts.OpenLots[i].Symbol == tx.Symbol && ts.OpenLots[j].Symbol != tx.Symbol {
			return true
		}
		if ts.OpenLots[i].Symbol != tx.Symbol && ts.OpenLots[j].Symbol == tx.Symbol {
			return false
		}
		return ts.OpenLots[i].Date.Before(ts.OpenLots[j].Date)
	})

	remaining := tx.Shares
	var kept []TaxLot

	for _, lot := range ts.OpenLots {
		if lot.Symbol != tx.Symbol || remaining <= 0 {
			kept = append(kept, lot)
			continue
		}

		if lot.Shares <= remaining {
			// Fully consume this lot
			sharesSold := lot.Shares
			gain := RealizedGain{
				Symbol:    tx.Symbol,
				SellDate:  tx.Date,
				Shares:    sharesSold,
				Proceeds:  tx.Price,
				CostBasis: lot.CostBasis,
				Gain:      (tx.Price - lot.CostBasis) * sharesSold,
				LongTerm:  tx.Date.Sub(lot.Date) > 365*24*time.Hour,
			}
			ts.RealizedGains = append(ts.RealizedGains, gain)
			remaining -= sharesSold
		} else {
			// Partially consume this lot
			sharesSold := remaining
			gain := RealizedGain{
				Symbol:    tx.Symbol,
				SellDate:  tx.Date,
				Shares:    sharesSold,
				Proceeds:  tx.Price,
				CostBasis: lot.CostBasis,
				Gain:      (tx.Price - lot.CostBasis) * sharesSold,
				LongTerm:  tx.Date.Sub(lot.Date) > 365*24*time.Hour,
			}
			ts.RealizedGains = append(ts.RealizedGains, gain)
			lot.Shares -= sharesSold
			kept = append(kept, lot)
			remaining = 0
		}
	}

	ts.OpenLots = kept
}

func (ts *TransactionStore) DividendIncomeYTD(symbol string) float64 {
	now := time.Now()
	yearStart := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.Local)

	var total float64
	for _, tx := range ts.Transactions {
		if tx.Type == TxDividend && tx.Symbol == symbol && !tx.Date.Before(yearStart) {
			total += tx.Total
		}
	}
	return total
}

func (ts *TransactionStore) DividendIncomeTotal(symbol string) float64 {
	var total float64
	for _, tx := range ts.Transactions {
		if tx.Type == TxDividend && tx.Symbol == symbol {
			total += tx.Total
		}
	}
	return total
}

func (ts *TransactionStore) RealizedGainYTD() float64 {
	now := time.Now()
	yearStart := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.Local)

	var total float64
	for _, g := range ts.RealizedGains {
		if !g.SellDate.Before(yearStart) {
			total += g.Gain
		}
	}
	return total
}

func (ts *TransactionStore) RealizedGainByBucket(symMap *SymbolMap) map[BucketType]float64 {
	result := make(map[BucketType]float64)
	for _, g := range ts.RealizedGains {
		bucket := symMap.Lookup(g.Symbol)
		result[bucket] += g.Gain
	}
	return result
}

func (ts *TransactionStore) IncomeByBucket(symMap *SymbolMap) map[BucketType]float64 {
	result := make(map[BucketType]float64)
	for _, tx := range ts.Transactions {
		if tx.Type == TxDividend {
			bucket := symMap.Lookup(tx.Symbol)
			result[bucket] += tx.Total
		}
	}
	return result
}
