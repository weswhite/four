package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BucketSnapshot holds a point-in-time snapshot of a single bucket's metrics.
type BucketSnapshot struct {
	MarketValue    float64 `json:"market_value"`
	CostBasis      float64 `json:"cost_basis"`
	UnrealizedGain float64 `json:"unrealized_gain"`
	RealizedGain   float64 `json:"realized_gain"`
	Income         float64 `json:"income"`
	AllocationPct  float64 `json:"allocation_pct"`
}

// MonthlySnapshot captures portfolio state for a given month.
type MonthlySnapshot struct {
	Month      string                        `json:"month"` // "2026-04"
	Date       time.Time                     `json:"date"`
	TotalValue float64                       `json:"total_value"`
	TotalIncome float64                      `json:"total_income"`
	Buckets    map[BucketType]BucketSnapshot `json:"buckets"`
}

// TakeSnapshot creates a MonthlySnapshot from the current bucket data.
func TakeSnapshot(buckets map[BucketType]*Bucket, totalValue float64) *MonthlySnapshot {
	now := time.Now()
	snap := &MonthlySnapshot{
		Month:      now.Format("2006-01"),
		Date:       now,
		TotalValue: totalValue,
		Buckets:    make(map[BucketType]BucketSnapshot),
	}

	var totalIncome float64
	for bt, b := range buckets {
		snap.Buckets[bt] = BucketSnapshot{
			MarketValue:    b.MarketValue,
			CostBasis:      b.CostBasis,
			UnrealizedGain: b.UnrealizedGain,
			RealizedGain:   b.RealizedGain,
			Income:         b.AnnualIncome,
			AllocationPct:  b.AllocationPct,
		}
		totalIncome += b.AnnualIncome
	}
	snap.TotalIncome = totalIncome

	return snap
}

// SaveSnapshot persists a MonthlySnapshot to ~/.config/four/history/{month}.json.
func SaveSnapshot(snap *MonthlySnapshot) error {
	if err := ensureHistoryDir(); err != nil {
		return err
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(historyDir(), snap.Month+".json")
	return os.WriteFile(path, data, 0644)
}

// LoadSnapshot reads a MonthlySnapshot from history/{month}.json.
func LoadSnapshot(month string) (*MonthlySnapshot, error) {
	path := filepath.Join(historyDir(), month+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var snap MonthlySnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

// ListSnapshots reads all snapshot files in the history directory and returns
// them sorted by month ascending.
func ListSnapshots() ([]MonthlySnapshot, error) {
	dir := historyDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var snapshots []MonthlySnapshot
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		month := strings.TrimSuffix(entry.Name(), ".json")
		snap, err := LoadSnapshot(month)
		if err != nil {
			continue
		}
		snapshots = append(snapshots, *snap)
	}

	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Month < snapshots[j].Month
	})

	return snapshots, nil
}

// AutoSnapshot checks if the current month's snapshot exists. If not, it takes
// and saves one automatically.
func AutoSnapshot(buckets map[BucketType]*Bucket, totalValue float64) error {
	currentMonth := time.Now().Format("2006-01")
	path := filepath.Join(historyDir(), currentMonth+".json")

	if _, err := os.Stat(path); err == nil {
		// Snapshot already exists for this month.
		return nil
	}

	snap := TakeSnapshot(buckets, totalValue)
	return SaveSnapshot(snap)
}

// PreviousMonthValues loads the previous month's snapshot and returns a map of
// bucket type to market value. Returns an empty map if no previous snapshot exists.
func PreviousMonthValues() map[BucketType]float64 {
	prevMonth := time.Now().AddDate(0, -1, 0).Format("2006-01")
	snap, err := LoadSnapshot(prevMonth)
	if err != nil {
		return make(map[BucketType]float64)
	}

	values := make(map[BucketType]float64)
	for bt, bs := range snap.Buckets {
		values[bt] = bs.MarketValue
	}
	return values
}
