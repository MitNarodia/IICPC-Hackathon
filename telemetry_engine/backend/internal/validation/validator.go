package validation

import (
	"fmt"
	"math"
	"time"

	"github.com/iicpc/track3/telemetry-engine/pkg/events"
	"github.com/iicpc/track3/telemetry-engine/pkg/models"
)

// Rule identifies a correctness rule. These strings are the keys in
// ValidationResult.ViolationsByRule and appear verbatim in the dashboard.
const (
	RulePriceTimePriority = "price_time_priority"
	RuleFIFO              = "fifo"
	RuleFillQuantity      = "fill_quantity"
	RuleBookConsistency   = "book_consistency"
	RuleTradeMatching     = "trade_matching"
)

const maxRecentFindings = 25

// pendingOrder remembers what a bot submitted so that, when the engine later
// acks or fills it, we can validate against the original intent (side, price,
// quantity).
type pendingOrder struct {
	side  Side
	kind  events.OrderKind
	price float64
	qty   uint32
}

// Validator is the correctness checker for ONE (run, submission). It is
// single-goroutine: the engine shards by submission so each Validator is owned
// by exactly one worker (no locks), mirroring the order events' partitioning.
type Validator struct {
	runID, submissionID string
	book                *Book

	pending            map[uint64]*pendingOrder // order_id -> submitted intent
	aggressorRemaining map[uint64]int64         // order_id -> qty left to trade

	ordersChecked uint64
	tradesChecked uint64
	violations    uint64
	byRule        map[string]uint64
	recent        []models.Finding
}

// NewValidator builds a validator with an empty reference book.
func NewValidator(runID, submissionID string, tickSize float64) *Validator {
	return &Validator{
		runID:              runID,
		submissionID:       submissionID,
		book:               NewBook(tickSize),
		pending:            map[uint64]*pendingOrder{},
		aggressorRemaining: map[uint64]int64{},
		byRule:             map[string]uint64{},
	}
}

func sideFromEvent(s events.Side) Side {
	if s == events.SideSell {
		return Sell
	}
	return Buy
}

// OnSubmitted records the bot's intent. Nothing is checked yet; we just remember
// it for when the engine responds.
func (v *Validator) OnSubmitted(o events.OrderSubmitted) {
	v.pending[o.OrderID] = &pendingOrder{
		side:  sideFromEvent(o.Side),
		kind:  o.Kind,
		price: o.Price,
		qty:   o.Quantity,
	}
	v.aggressorRemaining[o.OrderID] = int64(o.Quantity)
}

// OnAck adds an accepted LIMIT order to the reference book. A rejected order, or
// a market/cancel order (which never rests), is not added. Duplicate accepts of
// the same id are a consistency violation.
func (v *Validator) OnAck(a events.OrderAck) {
	if !a.Accepted {
		return
	}
	p := v.pending[a.OrderID]
	if p == nil || p.kind != events.KindLimit {
		return
	}
	if v.book.Lookup(a.OrderID) != nil {
		v.flag(RuleBookConsistency, a.OrderID, "engine accepted the same order_id twice")
		return
	}
	v.book.Add(a.OrderID, a.BotID, p.side, p.price, p.qty)
}

// OnCancelled removes a resting order. Cancelling an order the book never had
// (or already removed) is a consistency violation.
func (v *Validator) OnCancelled(c events.OrderCancelled) {
	if v.book.Lookup(c.OrderID) == nil {
		v.flag(RuleBookConsistency, c.OrderID, "cancel for an order not resting in the book")
		return
	}
	v.book.Cancel(c.OrderID)
	delete(v.aggressorRemaining, c.OrderID)
}

// OnFilled is the heart of the engine: it validates a reported trade against the
// five fairness rules, then applies the fill to the reference book.
func (v *Validator) OnFilled(f events.OrderFilled) {
	v.ordersChecked++

	maker := v.book.Lookup(f.MakerOrderID)
	if maker == nil {
		// Rule 4 (consistency): you cannot fill against an order that is not
		// resting. Without a maker we cannot run the price/FIFO/qty checks.
		v.flag(RuleBookConsistency, f.OrderID,
			fmt.Sprintf("fill names maker %d that is not resting in the book", f.MakerOrderID))
		v.applyAggressor(f)
		return
	}

	// Rule 5 — Trade Matching Correctness ---------------------------------
	// (a) Aggressor and maker must be on opposite sides.
	if ap := v.pending[f.OrderID]; ap != nil && ap.side == maker.side {
		v.flag(RuleTradeMatching, f.OrderID,
			fmt.Sprintf("aggressor and maker %d are both %s", f.MakerOrderID, maker.side))
	}
	// (b) The trade must print at the RESTING (maker) price.
	if math.Abs(f.FillPrice-maker.price) > v.book.tickSize/2 {
		v.flag(RuleTradeMatching, f.OrderID,
			fmt.Sprintf("fill price %.4f != resting maker price %.4f", f.FillPrice, maker.price))
	}
	// (c) If the aggressor is a limit order, the print must be marketable for
	//     it (a buyer never pays above its limit; a seller never sells below).
	if ap := v.pending[f.OrderID]; ap != nil && ap.kind == events.KindLimit {
		if ap.side == Buy && f.FillPrice > ap.price+v.book.tickSize/2 {
			v.flag(RuleTradeMatching, f.OrderID,
				fmt.Sprintf("buy filled at %.4f above its limit %.4f", f.FillPrice, ap.price))
		}
		if ap.side == Sell && f.FillPrice < ap.price-v.book.tickSize/2 {
			v.flag(RuleTradeMatching, f.OrderID,
				fmt.Sprintf("sell filled at %.4f below its limit %.4f", f.FillPrice, ap.price))
		}
	}

	// Rule 1 — Price Priority ---------------------------------------------
	// The maker must sit at the BEST price on its side. If a better price is
	// resting and live, that one should have been hit first.
	if best := v.book.bestLevel(maker.side); best != nil && best.priceTick != maker.priceTick {
		v.flag(RulePriceTimePriority, f.OrderID,
			fmt.Sprintf("filled maker at tick %d while a better price (tick %d) was resting",
				maker.priceTick, best.priceTick))
	}

	// Rule 2 — Time Priority (FIFO within a price level) -------------------
	// Among orders at the maker's price, the earliest-arriving live order has
	// priority. If the front of that level is someone else, FIFO was violated.
	if lv := v.book.levelFor(maker.side, maker.priceTick); lv != nil {
		if front := lv.front(); front != nil && front.orderID != maker.orderID {
			v.flag(RuleFIFO, f.OrderID,
				fmt.Sprintf("filled maker %d ahead of earlier order %d at the same price",
					maker.orderID, front.orderID))
		}
	}

	// Rule 3 — Correct Fill Quantity --------------------------------------
	fq := int64(f.FillQuantity)
	switch {
	case fq <= 0:
		v.flag(RuleFillQuantity, f.OrderID, "fill quantity must be positive")
	case fq > maker.remaining:
		v.flag(RuleFillQuantity, f.OrderID,
			fmt.Sprintf("fill qty %d exceeds maker remaining %d", fq, maker.remaining))
	}
	if ar, ok := v.aggressorRemaining[f.OrderID]; ok {
		if fq > ar {
			v.flag(RuleFillQuantity, f.OrderID,
				fmt.Sprintf("fill qty %d exceeds aggressor remaining %d", fq, ar))
		}
		// The engine's reported residual must match our arithmetic.
		if expected := ar - fq; expected >= 0 && int64(f.RemainingQuantity) != expected {
			v.flag(RuleFillQuantity, f.OrderID,
				fmt.Sprintf("reported remaining %d != expected %d", f.RemainingQuantity, expected))
		}
	}

	// Apply the fill to the reference book and aggressor bookkeeping.
	v.book.Reduce(f.MakerOrderID, fq)
	v.applyAggressorFill(f, fq)
	v.tradesChecked++

	// Post-condition consistency: a correct fill must not leave the book in a
	// persistently crossed state once the aggressor is exhausted.
	if v.aggressorRemaining[f.OrderID] == 0 && v.book.Crossed() {
		v.flag(RuleBookConsistency, f.OrderID, "book left crossed after aggressor fully traded")
	}
}

func (v *Validator) applyAggressor(f events.OrderFilled) {
	if ar, ok := v.aggressorRemaining[f.OrderID]; ok {
		na := ar - int64(f.FillQuantity)
		if na < 0 {
			na = 0
		}
		v.aggressorRemaining[f.OrderID] = na
	}
}

func (v *Validator) applyAggressorFill(f events.OrderFilled, fq int64) {
	v.applyAggressor(f)
	// Keep the aggressor's resting residual (if it is itself a limit) in sync.
	if v.book.Lookup(f.OrderID) != nil {
		v.book.Reduce(f.OrderID, fq)
	}
}

func (v *Validator) flag(rule string, orderID uint64, msg string) {
	v.violations++
	v.byRule[rule]++
	if len(v.recent) >= maxRecentFindings {
		v.recent = v.recent[1:]
	}
	v.recent = append(v.recent, models.Finding{
		Rule:    rule,
		Message: msg,
		OrderID: orderID,
		At:      time.Now().UTC(),
	})
}

// Result renders the cumulative verdict. CorrectnessScore is 1 minus the share
// of checks that found a violation, clamped to [0,1]; a clean run scores 1.0.
func (v *Validator) Result() models.ValidationResult {
	checks := v.ordersChecked
	if checks == 0 {
		checks = 1
	}
	score := 1.0 - float64(v.violations)/float64(checks)
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	byRule := make(map[string]uint64, len(v.byRule))
	for k, val := range v.byRule {
		byRule[k] = val
	}
	recent := make([]models.Finding, len(v.recent))
	copy(recent, v.recent)
	return models.ValidationResult{
		RunID:            v.runID,
		SubmissionID:     v.submissionID,
		UpdatedAt:        time.Now().UTC(),
		OrdersChecked:    v.ordersChecked,
		TradesChecked:    v.tradesChecked,
		Violations:       v.violations,
		ViolationsByRule: byRule,
		CorrectnessScore: score,
		RecentFindings:   recent,
	}
}
