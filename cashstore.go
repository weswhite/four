package main

// CashStore tracks per-brokerage cash balances so imports from different
// sources can be summed without double-counting.
// Persisted to ~/.config/four/cash_balances.json
type CashStore struct {
	Balances map[string]float64 `json:"balances"`
}

func NewCashStore() *CashStore {
	return &CashStore{Balances: map[string]float64{}}
}

func LoadCashStore() *CashStore {
	cs := NewCashStore()
	_ = loadJSON("cash_balances.json", &cs.Balances)
	return cs
}

func (cs *CashStore) Save() error {
	return saveJSON("cash_balances.json", cs.Balances)
}

func (cs *CashStore) Set(source string, amount float64) {
	cs.Balances[source] = amount
}

func (cs *CashStore) Total() float64 {
	total := 0.0
	for _, v := range cs.Balances {
		total += v
	}
	return total
}
