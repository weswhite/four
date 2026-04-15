package main

import "strings"

type SymbolEntry struct {
	Bucket BucketType `json:"bucket"`
	Source string     `json:"source,omitempty"`
}

type SymbolMap struct {
	Entries map[string]SymbolEntry `json:"entries"`
}

func NewSymbolMap() *SymbolMap {
	return &SymbolMap{Entries: map[string]SymbolEntry{}}
}

func LoadSymbolMap() *SymbolMap {
	sm := NewSymbolMap()
	_ = loadJSON("symbol_map.json", &sm.Entries)
	return sm
}

func (sm *SymbolMap) Save() error {
	return saveJSON("symbol_map.json", sm.Entries)
}

func (sm *SymbolMap) Lookup(symbol string) BucketType {
	sym := strings.ToUpper(strings.TrimSpace(symbol))
	if e, ok := sm.Entries[sym]; ok {
		return e.Bucket
	}
	return BucketEquityIncome // default bucket
}

func (sm *SymbolMap) Assign(symbol string, bucket BucketType, source string) {
	sym := strings.ToUpper(strings.TrimSpace(symbol))
	sm.Entries[sym] = SymbolEntry{Bucket: bucket, Source: source}
}

func (sm *SymbolMap) IsKnown(symbol string) bool {
	sym := strings.ToUpper(strings.TrimSpace(symbol))
	_, ok := sm.Entries[sym]
	return ok
}

func (sm *SymbolMap) UnknownSymbols(symbols []string) []string {
	var unknown []string
	for _, s := range symbols {
		if !sm.IsKnown(s) {
			unknown = append(unknown, strings.ToUpper(strings.TrimSpace(s)))
		}
	}
	return unknown
}
