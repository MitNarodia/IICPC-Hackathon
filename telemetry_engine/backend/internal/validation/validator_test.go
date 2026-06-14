package validation

import (
	"testing"

	"github.com/iicpc/track3/telemetry-engine/pkg/events"
)

// helpers to drive the validator concisely.
func sub(v *Validator, id uint64, side events.Side, kind events.OrderKind, price float64, qty uint32) {
	v.OnSubmitted(events.OrderSubmitted{OrderID: id, Side: side, Kind: kind, Price: price, Quantity: qty})
	v.OnAck(events.OrderAck{OrderID: id, Accepted: true})
}

// A correct match: incoming sell hits the single resting bid at its price, for
// the full quantity. No rule should fire.
func TestCleanMatch(t *testing.T) {
	v := NewValidator("run", "sub", 0.01)
	sub(v, 1, events.SideBuy, events.KindLimit, 100.00, 10) // resting bid
	// aggressor sell, market
	v.OnSubmitted(events.OrderSubmitted{OrderID: 2, Side: events.SideSell, Kind: events.KindMarket, Quantity: 10})
	v.OnAck(events.OrderAck{OrderID: 2, Accepted: true})
	v.OnFilled(events.OrderFilled{OrderID: 2, MakerOrderID: 1, FillPrice: 100.00, FillQuantity: 10, RemainingQuantity: 0})

	r := v.Result()
	if r.Violations != 0 {
		t.Fatalf("expected clean run, got %d violations: %+v", r.Violations, r.ViolationsByRule)
	}
	if r.CorrectnessScore != 1.0 {
		t.Errorf("expected correctness 1.0, got %v", r.CorrectnessScore)
	}
}

// Price priority: two resting bids at 100 and 101; a sell that hits the 100 bid
// while the better 101 bid is resting must trip price_time_priority.
func TestPricePriorityViolation(t *testing.T) {
	v := NewValidator("run", "sub", 0.01)
	sub(v, 1, events.SideBuy, events.KindLimit, 100.00, 10)
	sub(v, 2, events.SideBuy, events.KindLimit, 101.00, 10) // better bid
	v.OnSubmitted(events.OrderSubmitted{OrderID: 3, Side: events.SideSell, Kind: events.KindMarket, Quantity: 5})
	v.OnAck(events.OrderAck{OrderID: 3, Accepted: true})
	// Engine WRONGLY fills the inferior 100 bid.
	v.OnFilled(events.OrderFilled{OrderID: 3, MakerOrderID: 1, FillPrice: 100.00, FillQuantity: 5, RemainingQuantity: 0})

	if v.Result().ViolationsByRule[RulePriceTimePriority] == 0 {
		t.Fatalf("expected price_time_priority violation, got %+v", v.Result().ViolationsByRule)
	}
}

// FIFO: two resting bids at the SAME price; the engine fills the later one
// first, which must trip fifo.
func TestFIFOViolation(t *testing.T) {
	v := NewValidator("run", "sub", 0.01)
	sub(v, 1, events.SideBuy, events.KindLimit, 100.00, 10) // earlier, seq=1
	sub(v, 2, events.SideBuy, events.KindLimit, 100.00, 10) // later,  seq=2
	v.OnSubmitted(events.OrderSubmitted{OrderID: 3, Side: events.SideSell, Kind: events.KindMarket, Quantity: 5})
	v.OnAck(events.OrderAck{OrderID: 3, Accepted: true})
	// Engine WRONGLY fills order 2 ahead of the earlier order 1.
	v.OnFilled(events.OrderFilled{OrderID: 3, MakerOrderID: 2, FillPrice: 100.00, FillQuantity: 5, RemainingQuantity: 0})

	if v.Result().ViolationsByRule[RuleFIFO] == 0 {
		t.Fatalf("expected fifo violation, got %+v", v.Result().ViolationsByRule)
	}
}

// Fill quantity: a fill larger than the resting maker quantity must trip
// fill_quantity.
func TestFillQuantityViolation(t *testing.T) {
	v := NewValidator("run", "sub", 0.01)
	sub(v, 1, events.SideBuy, events.KindLimit, 100.00, 10)
	v.OnSubmitted(events.OrderSubmitted{OrderID: 2, Side: events.SideSell, Kind: events.KindMarket, Quantity: 50})
	v.OnAck(events.OrderAck{OrderID: 2, Accepted: true})
	// Over-fill: 25 > resting 10.
	v.OnFilled(events.OrderFilled{OrderID: 2, MakerOrderID: 1, FillPrice: 100.00, FillQuantity: 25, RemainingQuantity: 25})

	if v.Result().ViolationsByRule[RuleFillQuantity] == 0 {
		t.Fatalf("expected fill_quantity violation, got %+v", v.Result().ViolationsByRule)
	}
}

// Trade matching: a print at a price different from the resting maker price must
// trip trade_matching.
func TestTradeMatchingViolation(t *testing.T) {
	v := NewValidator("run", "sub", 0.01)
	sub(v, 1, events.SideBuy, events.KindLimit, 100.00, 10)
	v.OnSubmitted(events.OrderSubmitted{OrderID: 2, Side: events.SideSell, Kind: events.KindMarket, Quantity: 10})
	v.OnAck(events.OrderAck{OrderID: 2, Accepted: true})
	// Wrong print price 99.00 != resting 100.00.
	v.OnFilled(events.OrderFilled{OrderID: 2, MakerOrderID: 1, FillPrice: 99.00, FillQuantity: 10, RemainingQuantity: 0})

	if v.Result().ViolationsByRule[RuleTradeMatching] == 0 {
		t.Fatalf("expected trade_matching violation, got %+v", v.Result().ViolationsByRule)
	}
}

// Consistency: filling a maker that was never resting must trip
// book_consistency.
func TestPhantomMakerViolation(t *testing.T) {
	v := NewValidator("run", "sub", 0.01)
	v.OnSubmitted(events.OrderSubmitted{OrderID: 2, Side: events.SideSell, Kind: events.KindMarket, Quantity: 10})
	v.OnAck(events.OrderAck{OrderID: 2, Accepted: true})
	v.OnFilled(events.OrderFilled{OrderID: 2, MakerOrderID: 999, FillPrice: 100.00, FillQuantity: 10, RemainingQuantity: 0})

	if v.Result().ViolationsByRule[RuleBookConsistency] == 0 {
		t.Fatalf("expected book_consistency violation, got %+v", v.Result().ViolationsByRule)
	}
}
