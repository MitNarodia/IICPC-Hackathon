// Package validation implements Track 3's correctness engine. It reconstructs a
// REFERENCE order book from the contestant engine's own accepted orders, then
// checks every fill the engine reports against the book the rules of a fair
// exchange demand. Any deviation is a recorded Finding that lowers the
// submission's correctness score.
//
// This file is the reference order book: a price-ordered set of FIFO queues,
// exactly the canonical limit-order-book shape (price levels, time priority
// within a level). It is intentionally simple and obviously-correct, because it
// is the ORACLE we judge contestants against — cleverness here would undermine
// trust in the verdicts.
package validation

import "sort"

// Side of a resting order.
type Side uint8

const (
	Buy Side = iota
	Sell
)

func (s Side) String() string {
	if s == Buy {
		return "buy"
	}
	return "sell"
}

// refOrder is one resting order in the reference book.
type refOrder struct {
	orderID   uint64
	botID     uint64
	side      Side
	priceTick int64   // quantized price, the map/sort key
	price     float64 // original price, for messages
	remaining int64   // remaining quantity (can hit 0 → removed)
	seq       uint64  // global arrival sequence = TIME priority
}

// level is a single price level: a FIFO queue where index 0 is the earliest
// still-resting order (highest time priority).
type level struct {
	priceTick int64
	queue     []*refOrder
}

// front returns the earliest still-resting order at this level, or nil.
func (l *level) front() *refOrder {
	for len(l.queue) > 0 && l.queue[0].remaining <= 0 {
		l.queue = l.queue[1:]
	}
	if len(l.queue) == 0 {
		return nil
	}
	return l.queue[0]
}

// totalQty is the sum of remaining quantity at this level (consistency checks).
func (l *level) totalQty() int64 {
	var t int64
	for _, o := range l.queue {
		if o.remaining > 0 {
			t += o.remaining
		}
	}
	return t
}

// Book is the reference limit order book for one (run, submission).
type Book struct {
	tickSize float64
	bids     map[int64]*level
	asks     map[int64]*level
	bidTicks []int64 // sorted DESC: bidTicks[0] is the best (highest) bid
	askTicks []int64 // sorted ASC:  askTicks[0] is the best (lowest) ask
	byID     map[uint64]*refOrder
	seq      uint64
}

// NewBook creates an empty reference book. tickSize quantizes prices so float
// noise never splits one economic price into two map keys.
func NewBook(tickSize float64) *Book {
	if tickSize <= 0 {
		tickSize = 0.01
	}
	return &Book{
		tickSize: tickSize,
		bids:     map[int64]*level{},
		asks:     map[int64]*level{},
		byID:     map[uint64]*refOrder{},
	}
}

func (b *Book) tick(price float64) int64 {
	return int64((price / b.tickSize) + 0.5)
}

// Add inserts a resting limit order, assigning it the next arrival sequence so
// later FIFO checks have a deterministic time order.
func (b *Book) Add(orderID, botID uint64, side Side, price float64, qty uint32) *refOrder {
	if _, exists := b.byID[orderID]; exists {
		return b.byID[orderID] // duplicate accept; consistency check flags it
	}
	b.seq++
	o := &refOrder{
		orderID:   orderID,
		botID:     botID,
		side:      side,
		priceTick: b.tick(price),
		price:     price,
		remaining: int64(qty),
		seq:       b.seq,
	}
	b.byID[orderID] = o
	side2levels, ticks := b.sideMaps(side)
	lv := side2levels[o.priceTick]
	if lv == nil {
		lv = &level{priceTick: o.priceTick}
		side2levels[o.priceTick] = lv
		b.insertTick(ticks, o.priceTick, side)
	}
	lv.queue = append(lv.queue, o)
	return o
}

func (b *Book) sideMaps(side Side) (map[int64]*level, *[]int64) {
	if side == Buy {
		return b.bids, &b.bidTicks
	}
	return b.asks, &b.askTicks
}

// insertTick keeps the per-side tick slice sorted (bids DESC, asks ASC) so the
// best price is always element 0.
func (b *Book) insertTick(ticks *[]int64, t int64, side Side) {
	s := *ticks
	var idx int
	if side == Buy { // descending
		idx = sort.Search(len(s), func(i int) bool { return s[i] < t })
	} else { // ascending
		idx = sort.Search(len(s), func(i int) bool { return s[i] > t })
	}
	s = append(s, 0)
	copy(s[idx+1:], s[idx:])
	s[idx] = t
	*ticks = s
}

func (b *Book) removeTick(ticks *[]int64, t int64) {
	s := *ticks
	for i, v := range s {
		if v == t {
			*ticks = append(s[:i], s[i+1:]...)
			return
		}
	}
}

// Lookup returns a resting order by id (nil if unknown/fully filled+removed).
func (b *Book) Lookup(orderID uint64) *refOrder { return b.byID[orderID] }

// levelFor returns the price level for (side, tick) without pruning, or nil.
func (b *Book) levelFor(side Side, tick int64) *level {
	levels, _ := b.sideMaps(side)
	return levels[tick]
}

// BestBid / BestAsk return the best price level on each side, or nil.
func (b *Book) BestBid() *level { return b.bestLevel(Buy) }
func (b *Book) BestAsk() *level { return b.bestLevel(Sell) }

func (b *Book) bestLevel(side Side) *level {
	levels, ticks := b.sideMaps(side)
	for len(*ticks) > 0 {
		lv := levels[(*ticks)[0]]
		if lv != nil && lv.front() != nil {
			return lv
		}
		// Top level is empty — prune it and look again.
		if lv != nil {
			delete(levels, (*ticks)[0])
		}
		*ticks = (*ticks)[1:]
	}
	return nil
}

// Reduce subtracts qty from a resting order (applying a fill), removing it from
// the book when it reaches zero. Returns the quantity actually reduced.
func (b *Book) Reduce(orderID uint64, qty int64) int64 {
	o := b.byID[orderID]
	if o == nil || o.remaining <= 0 {
		return 0
	}
	if qty > o.remaining {
		qty = o.remaining
	}
	o.remaining -= qty
	if o.remaining == 0 {
		delete(b.byID, orderID)
	}
	return qty
}

// Cancel removes a resting order entirely. Returns the quantity removed.
func (b *Book) Cancel(orderID uint64) int64 {
	o := b.byID[orderID]
	if o == nil {
		return 0
	}
	rem := o.remaining
	o.remaining = 0
	delete(b.byID, orderID)
	return rem
}

// Crossed reports whether the book is in an illegal crossed state: the best bid
// is priced at or above the best ask, which a correct engine must never leave
// resting (it would have matched them). Used by the consistency check.
func (b *Book) Crossed() bool {
	bb, ba := b.BestBid(), b.BestAsk()
	if bb == nil || ba == nil {
		return false
	}
	return bb.priceTick >= ba.priceTick
}
