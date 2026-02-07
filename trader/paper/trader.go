package paper

import (
	"fmt"
	"sync"
	"time"

	"nofx/trader/types"
)

// position represents a simple long/short position for paper trading
type position struct {
	side       string // long/short
	quantity   float64
	entryPrice float64
}

// Trader is a lightweight paper trader implementing types.Trader interface (subset needed by auto_trader)
type Trader struct {
	mu        sync.Mutex
	balance   float64
	positions map[string]*position // key: symbol
}

// NewPaperTrader creates a new paper trader with given starting balance
func NewPaperTrader(initialBalance float64) *Trader {
	if initialBalance <= 0 {
		initialBalance = 10000 // fallback
	}
	return &Trader{
		balance:   initialBalance,
		positions: make(map[string]*position),
	}
}

// --- Helper ---
func (t *Trader) getPrice(symbol string) float64 {
	// Simple constant price; in future can be wired to live feed
	return 100.0
}

func (t *Trader) fee(amount float64) float64 {
	return amount * 0.0004 // 4 bps default taker fee
}

// --- Interface implementation ---
func (t *Trader) GetBalance() (map[string]interface{}, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return map[string]interface{}{
		"total_equity":       t.balance,
		"totalWalletBalance": t.balance,
		"wallet_balance":     t.balance,
		"balance":            t.balance,
	}, nil
}

func (t *Trader) GetPositions() ([]map[string]interface{}, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var res []map[string]interface{}
	for sym, p := range t.positions {
		if p.quantity == 0 {
			continue
		}
		cur := t.getPrice(sym)
		unrealized := (cur - p.entryPrice)
		if p.side == "short" {
			unrealized = -unrealized
		}
		unrealized *= p.quantity
		res = append(res, map[string]interface{}{
			"symbol":     sym,
			"side":       p.side,
			"quantity":   p.quantity,
			"entryPrice": p.entryPrice,
			"markPrice":  cur,
			"unrealized": unrealized,
		})
	}
	return res, nil
}

func (t *Trader) OpenLong(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	return t.openPosition(symbol, "long", quantity)
}

func (t *Trader) OpenShort(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	return t.openPosition(symbol, "short", quantity)
}

func (t *Trader) openPosition(symbol, side string, quantity float64) (map[string]interface{}, error) {
	if quantity <= 0 {
		return nil, fmt.Errorf("quantity must be > 0")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	price := t.getPrice(symbol)
	cost := price * quantity
	fee := t.fee(cost)
	if t.balance < cost+fee {
		return nil, fmt.Errorf("insufficient balance")
	}
	t.balance -= cost + fee
	p, ok := t.positions[symbol]
	if !ok {
		p = &position{side: side, quantity: 0, entryPrice: price}
		t.positions[symbol] = p
	}
	// If switching side, flatten first
	if p.side != side && p.quantity > 0 {
		// close existing then reopen
		t.mu.Unlock()
		if p.side == "long" {
			if _, err := t.CloseLong(symbol, p.quantity); err != nil {
				return nil, err
			}
		} else {
			if _, err := t.CloseShort(symbol, p.quantity); err != nil {
				return nil, err
			}
		}
		t.mu.Lock()
		p = &position{side: side, quantity: 0, entryPrice: price}
		t.positions[symbol] = p
	}
	newQty := p.quantity + quantity
	p.entryPrice = (p.entryPrice*p.quantity + price*quantity) / newQty
	p.quantity = newQty
	return map[string]interface{}{"price": price, "executedQty": quantity, "status": "FILLED"}, nil
}

func (t *Trader) CloseLong(symbol string, quantity float64) (map[string]interface{}, error) {
	return t.closePosition(symbol, "long", quantity)
}

func (t *Trader) CloseShort(symbol string, quantity float64) (map[string]interface{}, error) {
	return t.closePosition(symbol, "short", quantity)
}

func (t *Trader) closePosition(symbol, side string, quantity float64) (map[string]interface{}, error) {
	if quantity < 0 {
		return nil, fmt.Errorf("quantity must be >= 0")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	p, ok := t.positions[symbol]
	if !ok || p.quantity == 0 || p.side != side {
		return nil, fmt.Errorf("no %s position for %s", side, symbol)
	}
	if quantity == 0 || quantity > p.quantity {
		quantity = p.quantity
	}
	price := t.getPrice(symbol)
	// PnL
	pnl := (price - p.entryPrice)
	if side == "short" {
		pnl = -pnl
	}
	pnl *= quantity
	fee := t.fee(price * quantity)
	t.balance += price*quantity + pnl - fee
	p.quantity -= quantity
	if p.quantity == 0 {
		delete(t.positions, symbol)
	}
	return map[string]interface{}{"price": price, "executedQty": quantity, "realizedPnl": pnl}, nil
}

func (t *Trader) SetLeverage(symbol string, leverage int) error         { return nil }
func (t *Trader) SetMarginMode(symbol string, isCrossMargin bool) error { return nil }

func (t *Trader) GetMarketPrice(symbol string) (float64, error) {
	return t.getPrice(symbol), nil
}

func (t *Trader) SetStopLoss(symbol string, positionSide string, quantity, stopPrice float64) error {
	// No-op in paper trader
	return nil
}

func (t *Trader) SetTakeProfit(symbol string, positionSide string, quantity, takeProfitPrice float64) error {
	return nil
}

func (t *Trader) CancelStopLossOrders(symbol string) error   { return nil }
func (t *Trader) CancelTakeProfitOrders(symbol string) error { return nil }
func (t *Trader) CancelAllOrders(symbol string) error        { return nil }
func (t *Trader) CancelStopOrders(symbol string) error       { return nil }

func (t *Trader) FormatQuantity(symbol string, quantity float64) (string, error) {
	return fmt.Sprintf("%.6f", quantity), nil
}

func (t *Trader) GetOrderStatus(symbol string, orderID string) (map[string]interface{}, error) {
	return map[string]interface{}{"status": "FILLED", "avgPrice": t.getPrice(symbol), "executedQty": 0.0, "commission": 0.0}, nil
}

func (t *Trader) GetClosedPnL(startTime time.Time, limit int) ([]types.ClosedPnLRecord, error) {
	return []types.ClosedPnLRecord{}, nil
}

func (t *Trader) GetOpenOrders(symbol string) ([]types.OpenOrder, error) {
	return []types.OpenOrder{}, nil
}
