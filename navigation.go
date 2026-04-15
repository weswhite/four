package main

import (
	"fmt"
	"time"
)

// ViewMode represents the current view in the TUI.
type ViewMode int

const (
	ViewDashboard ViewMode = iota
	ViewBucketDetail
	ViewImport
	ViewTransaction
	ViewHistory
	ViewDividendCal
	ViewRebalance
	ViewWatchlist
)

// NavState tracks the current navigation state of the TUI.
type NavState struct {
	View           ViewMode
	SelectedBucket int // 0-3 index into AllBuckets
	PreviousView   ViewMode
	ImportPath     string
	StatusMsg      string
	StatusExpiry   time.Time
}

// NewNavState returns a NavState initialized to the dashboard view.
func NewNavState() *NavState {
	return &NavState{View: ViewDashboard}
}

// GoTo navigates to the specified view, saving the current view as previous.
func (n *NavState) GoTo(view ViewMode) {
	n.PreviousView = n.View
	n.View = view
}

// Back returns to the previous view. If the previous view is the same as the
// current view, it defaults to the dashboard.
func (n *NavState) Back() {
	if n.PreviousView == n.View {
		n.View = ViewDashboard
	} else {
		n.View = n.PreviousView
	}
}

// SelectBucket sets the selected bucket index (clamped 0-3) and navigates to
// the bucket detail view.
func (n *NavState) SelectBucket(index int) {
	if index < 0 {
		index = 0
	}
	if index > 3 {
		index = 3
	}
	n.SelectedBucket = index
	n.GoTo(ViewBucketDetail)
}

// NextBucket cycles the selected bucket forward (mod 4).
func (n *NavState) NextBucket() {
	n.SelectedBucket = (n.SelectedBucket + 1) % 4
}

// PrevBucket cycles the selected bucket backward (mod 4).
func (n *NavState) PrevBucket() {
	n.SelectedBucket = (n.SelectedBucket + 3) % 4
}

// CurrentBucket returns the BucketType for the currently selected bucket.
func (n *NavState) CurrentBucket() BucketType {
	return AllBuckets[n.SelectedBucket]
}

// SetStatus sets a temporary status message with the given duration.
func (n *NavState) SetStatus(msg string, duration time.Duration) {
	n.StatusMsg = msg
	n.StatusExpiry = time.Now().Add(duration)
}

// Status returns the current status message if it has not expired, otherwise "".
func (n *NavState) Status() string {
	if time.Now().Before(n.StatusExpiry) {
		return n.StatusMsg
	}
	return ""
}

// ViewLabel returns a human-readable label for the current view.
func (n *NavState) ViewLabel() string {
	switch n.View {
	case ViewDashboard:
		return "Dashboard"
	case ViewBucketDetail:
		return fmt.Sprintf("Bucket: %s", n.CurrentBucket().Label())
	case ViewImport:
		return "Import"
	case ViewTransaction:
		return "Transaction"
	case ViewHistory:
		return "History"
	case ViewDividendCal:
		return "Dividend Calendar"
	case ViewRebalance:
		return "Rebalance"
	case ViewWatchlist:
		return "Watchlist"
	default:
		return "Dashboard"
	}
}
