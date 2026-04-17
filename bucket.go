package main

import (
	"fmt"
	"math"
)

type BucketType string

const (
	BucketCash            BucketType = "cash"
	BucketBonds           BucketType = "bonds"
	BucketEquityIncome    BucketType = "equity_income"
	BucketLongTermEquity  BucketType = "long_term_equity"
)

var AllBuckets = []BucketType{
	BucketCash,
	BucketBonds,
	BucketEquityIncome,
	BucketLongTermEquity,
}

func (b BucketType) Label() string {
	switch b {
	case BucketCash:
		return "CASH"
	case BucketBonds:
		return "BONDS"
	case BucketEquityIncome:
		return "EQUITY INCOME"
	case BucketLongTermEquity:
		return "LONG-TERM EQUITY"
	default:
		return string(b)
	}
}

func (b BucketType) Short() string {
	switch b {
	case BucketCash:
		return "Cash"
	case BucketBonds:
		return "Bonds"
	case BucketEquityIncome:
		return "Eq Inc"
	case BucketLongTermEquity:
		return "LT Eq"
	default:
		return string(b)
	}
}

func (b BucketType) Color() string {
	switch b {
	case BucketCash:
		return cyan
	case BucketBonds:
		return yellow
	case BucketEquityIncome:
		return green
	case BucketLongTermEquity:
		return magenta
	default:
		return white
	}
}

func BucketFromIndex(i int) BucketType {
	if i >= 0 && i < len(AllBuckets) {
		return AllBuckets[i]
	}
	return BucketCash
}

type BucketGoal struct {
	TargetPct    float64 `json:"target_pct"`              // e.g. 10.0 for 10%
	TargetIncome float64 `json:"target_income,omitempty"` // annual income goal for this bucket
}

// RebalanceAction describes a single rebalance suggestion.
type RebalanceAction struct {
	Bucket      BucketType
	TargetPct   float64
	ActualPct   float64
	DeltaPct    float64
	DeltaDollar float64
}

type Bucket struct {
	Type           BucketType
	Holdings       []Holding
	MarketValue    float64
	CostBasis      float64
	UnrealizedGain float64
	RealizedGain   float64
	AnnualIncome   float64
	AllocationPct  float64
	MonthChange    float64
	MonthChangePct float64
	IncomeYield    float64
}

func AggregateBuckets(holdings []Holding, symMap *SymbolMap, realizedByBucket map[BucketType]float64, prevMonthValues map[BucketType]float64) (map[BucketType]*Bucket, float64) {
	buckets := map[BucketType]*Bucket{}
	for _, bt := range AllBuckets {
		buckets[bt] = &Bucket{Type: bt}
	}

	totalValue := 0.0
	for _, h := range holdings {
		bt := symMap.Lookup(h.Symbol)
		b := buckets[bt]
		b.Holdings = append(b.Holdings, h)
		b.MarketValue += h.MarketValue
		b.CostBasis += h.CostBasis
		b.UnrealizedGain += h.TotalGain
		b.AnnualIncome += h.AnnualDivTotal
		totalValue += h.MarketValue
	}

	// Add per-brokerage cash balances to the Cash bucket
	cs := LoadCashStore()
	cashTotal := cs.Total()
	if cashTotal > 0 {
		buckets[BucketCash].MarketValue += cashTotal
		totalValue += cashTotal
	}

	for bt, b := range buckets {
		if totalValue > 0 {
			b.AllocationPct = (b.MarketValue / totalValue) * 100
		}
		if b.MarketValue > 0 {
			b.IncomeYield = (b.AnnualIncome / b.MarketValue) * 100
		}
		if rg, ok := realizedByBucket[bt]; ok {
			b.RealizedGain = rg
		}
		if prev, ok := prevMonthValues[bt]; ok && prev > 0 {
			b.MonthChange = b.MarketValue - prev
			b.MonthChangePct = (b.MonthChange / prev) * 100
		}
	}

	return buckets, totalValue
}

func BucketPromptText(sym string) string {
	return fmt.Sprintf("Assign %s to bucket: [1] Cash  [2] Bonds  [3] Equity Income  [4] Long-Term Equity", sym)
}

// ComputeRebalance returns rebalance actions for each bucket that has a target.
func ComputeRebalance(buckets map[BucketType]*Bucket, goals map[BucketType]BucketGoal, totalValue float64) []RebalanceAction {
	var actions []RebalanceAction
	for _, bt := range AllBuckets {
		goal, ok := goals[bt]
		if !ok || goal.TargetPct == 0 {
			continue
		}
		bkt := buckets[bt]
		actual := 0.0
		if bkt != nil && totalValue > 0 {
			actual = (bkt.MarketValue / totalValue) * 100
		}
		delta := actual - goal.TargetPct
		deltaDollar := (delta / 100) * totalValue
		actions = append(actions, RebalanceAction{
			Bucket:      bt,
			TargetPct:   goal.TargetPct,
			ActualPct:   actual,
			DeltaPct:    delta,
			DeltaDollar: math.Abs(deltaDollar),
		})
	}
	return actions
}
