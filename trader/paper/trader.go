package paper

import (
	"fmt"
	"nofx/logger"
	"nofx/market"
	"sync"
	"time"

	"nofx/trader/types"
)

// position represents a simple long/short position for paper trading
type position struct {
	side       string // long/short
	quantity   float64
	entryPrice float64
	margin     float64 // total margin locked for this position
	leverage   int
}

// Trader is a lightweight paper trader implementing types.Trader interface (subset needed by auto_trader)
type Trader struct {
	mu        sync.Mutex
	balance   float64
	positions map[string]*position // key: symbol
	lastPrice map[string]float64   // external price hints

	feeRate     float64 // default 4 bps
	slippageBps float64 // default 2 bps
	priceSource string  // binance/mock
}

// NewPaperTrader creates a new paper trader with given starting balance
func NewPaperTrader(initialBalance float64) *Trader {
	if initialBalance <= 0 {
		initialBalance = 10000 // fallback
	}
	return &Trader{
		balance:     initialBalance,
		positions:   make(map[string]*position),
		lastPrice:   make(map[string]float64),
		feeRate:     0.0004,
		slippageBps: 2,
		priceSource: "binance",
	}
}

// SetFeeRate overrides the default trading fee rate (e.g., 0.0004 == 4 bps)
func (t *Trader) SetFeeRate(rate float64) {
	if rate <= 0 {
		return
	}
	t.mu.Lock()
	t.feeRate = rate
	t.mu.Unlock()
}

// SetSlippageBps overrides the default slippage in basis points
func (t *Trader) SetSlippageBps(bps float64) {
	if bps < 0 {
		return
	}
	t.mu.Lock()
	t.slippageBps = bps
	t.mu.Unlock()
}

// SetPriceSource overrides price source (binance/mock)
func (t *Trader) SetPriceSource(src string) {
	if src == "" {
		return
	}
	t.mu.Lock()
	t.priceSource = src
	t.mu.Unlock()
}

// --- Helper ---
// refreshPrices pulls latest price for held symbols from Binance futures ticker (public endpoint)
func (t *Trader) refreshPrices(symbols []string) {
	if len(symbols) == 0 {
		return
	}
	if t.priceSource != "binance" {
		// mock source: keep existing cached/placeholder prices
		if t.priceSource == "okx" {
			client := market.NewAPIClient()
			for _, sym := range symbols {
				if sym == "" {
					continue
				}
				if price, err := client.GetOKXSwapPrice(sym); err == nil && price > 0 {
					t.mu.Lock()
					t.lastPrice[sym] = price
					t.mu.Unlock()
				} else if err != nil {
					logger.Infof("[paper] failed to refresh OKX price for %s: %v", sym, err)
				}
			}
		} else if t.priceSource == "bybit" {
			client := market.NewAPIClient()
			for _, sym := range symbols {
				if sym == "" {
					continue
				}
				if price, err := client.GetBybitLinearPrice(sym); err == nil && price > 0 {
					t.mu.Lock()
					t.lastPrice[sym] = price
					t.mu.Unlock()
				}
			}
		} else if t.priceSource == "bitget" {
			client := market.NewAPIClient()
			for _, sym := range symbols {
				if sym == "" {
					continue
				}
				if price, err := client.GetBitgetSwapPrice(sym); err == nil && price > 0 {
					t.mu.Lock()
					t.lastPrice[sym] = price
					t.mu.Unlock()
				}
			}
		} else if t.priceSource == "gate" {
			client := market.NewAPIClient()
			for _, sym := range symbols {
				if sym == "" {
					continue
				}
				if price, err := client.GetGateSwapPrice(sym); err == nil && price > 0 {
					t.mu.Lock()
					t.lastPrice[sym] = price
					t.mu.Unlock()
				}
			}
		} else if t.priceSource == "kucoin" {
			client := market.NewAPIClient()
			for _, sym := range symbols {
				if sym == "" {
					continue
				}
				if price, err := client.GetKucoinSwapPrice(sym); err == nil && price > 0 {
					t.mu.Lock()
					t.lastPrice[sym] = price
					t.mu.Unlock()
				}
			}
		}
		return
	}
	client := market.NewAPIClient()
	for _, sym := range symbols {
		if sym == "" {
			continue
		}
		if price, err := client.GetCurrentPrice(sym); err == nil && price > 0 {
			t.mu.Lock()
			t.lastPrice[sym] = price
			t.mu.Unlock()
		} else if err != nil {
			logger.Infof("[paper] failed to refresh price for %s: %v", sym, err)
		}
	}
}

func (t *Trader) getPrice(symbol string) float64 {
	if p, ok := t.lastPrice[symbol]; ok && p > 0 {
		return p
	}
	// Simple constant price; in future can be wired to live feed
	return 100.0
}

func (t *Trader) fee(amount float64) float64 {
	return amount * t.feeRate
}

// SetLastPrice allows external caller to update latest price for a symbol (used for executions/marking)
func (t *Trader) SetLastPrice(symbol string, price float64) {
	if price <= 0 {
		return
	}
	t.mu.Lock()
	t.lastPrice[symbol] = price
	t.mu.Unlock()
}

// --- Interface implementation ---
func (t *Trader) GetBalance() (map[string]interface{}, error) {
	// Refresh prices for current positions before computing equity
	t.mu.Lock()
	symbols := make([]string, 0, len(t.positions))
	for sym := range t.positions {
		symbols = append(symbols, sym)
	}
	t.mu.Unlock()
	t.refreshPrices(symbols)

	t.mu.Lock()
	defer t.mu.Unlock()
	unrealized := 0.0
	totalMargin := 0.0
	for sym, p := range t.positions {
		if p.quantity == 0 {
			continue
		}
		cur := t.getPrice(sym)
		if cur <= 0 {
			cur = p.entryPrice
		}
		u := (cur - p.entryPrice)
		if p.side == "short" {
			u = -u
		}
		unrealized += u * p.quantity
		totalMargin += p.margin
	}
	equity := t.balance + totalMargin + unrealized
	return map[string]interface{}{
		"total_equity":          equity,
		"totalWalletBalance":    equity,
		"wallet_balance":        t.balance + totalMargin,
		"balance":               t.balance,
		"availableBalance":      t.balance,
		"available_balance":     t.balance,
		"totalUnrealizedProfit": unrealized,
		"total_unrealized":      unrealized,
		"totalEquity":           equity,
		"total_pnl":             0.0,
		"total_pnl_pct":         0.0,
	}, nil
}

// RestorePosition rebuilds an open position (used to reload paper trader state)
func (t *Trader) RestorePosition(symbol, side string, quantity, entryPrice float64, leverage int) {
	if quantity <= 0 || entryPrice <= 0 {
		return
	}
	if leverage < 1 {
		leverage = 1
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	price := entryPrice
	marginRequired := (price * quantity) / float64(leverage)
	if t.balance < marginRequired {
		// If not enough balance (should not happen), cap by balance
		marginRequired = t.balance
	}
	t.balance -= marginRequired

	p := &position{
		side:       side,
		quantity:   quantity,
		entryPrice: price,
		margin:     marginRequired,
		leverage:   leverage,
	}
	t.positions[symbol] = p
}

func (t *Trader) GetPositions() ([]map[string]interface{}, error) {
	// Refresh prices for positions first
	t.mu.Lock()
	symbols := make([]string, 0, len(t.positions))
	for sym := range t.positions {
		symbols = append(symbols, sym)
	}
	t.mu.Unlock()
	t.refreshPrices(symbols)

	t.mu.Lock()
	defer t.mu.Unlock()
	var res []map[string]interface{}
	for sym, p := range t.positions {
		if p.quantity == 0 {
			continue
		}
		cur := t.getPrice(sym)
		if cur <= 0 {
			cur = p.entryPrice
		}
		unrealized := (cur - p.entryPrice)
		if p.side == "short" {
			unrealized = -unrealized
		}
		unrealized *= p.quantity

		positionAmt := p.quantity
		if p.side == "short" {
			positionAmt = -positionAmt
		}

		leverage := p.leverage
		if leverage < 1 {
			leverage = 1
		}

		// Simple placeholder liquidation price (not enforced in paper mode)
		liqPrice := 0.0
		if leverage > 1 {
			if p.side == "long" {
				liqPrice = p.entryPrice * (1 - 1/float64(leverage))
			} else {
				liqPrice = p.entryPrice * (1 + 1/float64(leverage))
			}
		}

		res = append(res, map[string]interface{}{
			"symbol":           sym,
			"side":             p.side,
			"quantity":         p.quantity,
			"positionAmt":      positionAmt,
			"entryPrice":       p.entryPrice,
			"markPrice":        cur,
			"unrealized":       unrealized,
			"unRealizedProfit": unrealized,
			"liquidationPrice": liqPrice,
			"leverage":         float64(leverage),
			"margin":           p.margin,
		})
	}
	return res, nil
}

func (t *Trader) OpenLong(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	return t.openPosition(symbol, "long", quantity, leverage)
}

func (t *Trader) OpenShort(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	return t.openPosition(symbol, "short", quantity, leverage)
}

func (t *Trader) openPosition(symbol, side string, quantity float64, leverage int) (map[string]interface{}, error) {
	if quantity <= 0 {
		return nil, fmt.Errorf("quantity must be > 0")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	price := t.getPrice(symbol)
	// apply slippage
	if side == "long" {
		price *= 1 + t.slippageBps/10000.0
	} else {
		price *= 1 - t.slippageBps/10000.0
	}
	// leverage-aware margin (futures style)
	if leverage < 1 {
		leverage = 1
	}
	marginRequired := (price * quantity) / float64(leverage)
	fee := t.fee(price * quantity)
	if t.balance < marginRequired+fee {
		// auto-scale position to max affordable size instead of failing
		reqPerQty := (price / float64(leverage)) + price*t.feeRate
		maxQty := t.balance / reqPerQty
		if maxQty <= 0 {
			logger.Infof("[paper] insufficient balance: bal=%.4f price=%.4f qty=%.6f lev=%d margin=%.4f fee=%.4f reqPerQty=%.6f source=%s", t.balance, price, quantity, leverage, marginRequired, fee, reqPerQty, t.priceSource)
			return nil, fmt.Errorf("insufficient balance")
		}
		logger.Infof("[paper] auto-resize qty from %.6f to %.6f due to balance %.4f (price=%.4f lev=%d)", quantity, maxQty, t.balance, price, leverage)
		quantity = maxQty
		marginRequired = (price * quantity) / float64(leverage)
		fee = t.fee(price * quantity)
	}
	t.balance -= marginRequired + fee
	p, ok := t.positions[symbol]
	if !ok {
		p = &position{side: side, quantity: 0, entryPrice: price, margin: 0, leverage: leverage}
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
		p = &position{side: side, quantity: 0, entryPrice: price, margin: 0, leverage: leverage}
		t.positions[symbol] = p
	}
	newQty := p.quantity + quantity
	p.entryPrice = (p.entryPrice*p.quantity + price*quantity) / newQty
	p.quantity = newQty
	p.margin += marginRequired
	p.leverage = leverage
	// Cache latest trade price so unrealized PnL falls back to entry if ticker fetch fails
	t.lastPrice[symbol] = price
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
	// reverse slippage on exit
	if side == "long" {
		price *= 1 - t.slippageBps/10000.0
	} else {
		price *= 1 + t.slippageBps/10000.0
	}
	// PnL
	pnl := (price - p.entryPrice)
	if side == "short" {
		pnl = -pnl
	}
	pnl *= quantity
	fee := t.fee(price * quantity)
	marginRelease := p.margin
	if p.quantity > 0 {
		marginRelease = p.margin * (quantity / p.quantity)
	}
	t.balance += marginRelease + pnl - fee
	p.quantity -= quantity
	p.margin -= marginRelease
	if p.quantity == 0 {
		delete(t.positions, symbol)
	}
	return map[string]interface{}{"price": price, "executedQty": quantity, "realizedPnl": pnl}, nil
}

func (t *Trader) SetLeverage(symbol string, leverage int) error         { return nil }
func (t *Trader) SetMarginMode(symbol string, isCrossMargin bool) error { return nil }

func (t *Trader) GetMarketPrice(symbol string) (float64, error) {
	// Try cached
	price := t.getPrice(symbol)
	if price != 100.0 || symbol == "" {
		return price, nil
	}
	// Fetch live from public ticker if cache empty or placeholder
	switch t.priceSource {
	case "binance":
		client := market.NewAPIClient()
		if p, err := client.GetCurrentPrice(symbol); err == nil && p > 0 {
			t.SetLastPrice(symbol, p)
			return p, nil
		}
	case "okx":
		client := market.NewAPIClient()
		if p, err := client.GetOKXSwapPrice(symbol); err == nil && p > 0 {
			t.SetLastPrice(symbol, p)
			return p, nil
		}
	case "bybit":
		client := market.NewAPIClient()
		if p, err := client.GetBybitLinearPrice(symbol); err == nil && p > 0 {
			t.SetLastPrice(symbol, p)
			return p, nil
		}
	case "bitget":
		client := market.NewAPIClient()
		if p, err := client.GetBitgetSwapPrice(symbol); err == nil && p > 0 {
			t.SetLastPrice(symbol, p)
			return p, nil
		}
	case "gate":
		client := market.NewAPIClient()
		if p, err := client.GetGateSwapPrice(symbol); err == nil && p > 0 {
			t.SetLastPrice(symbol, p)
			return p, nil
		}
	case "kucoin":
		client := market.NewAPIClient()
		if p, err := client.GetKucoinSwapPrice(symbol); err == nil && p > 0 {
			t.SetLastPrice(symbol, p)
			return p, nil
		}
	}
	return price, nil
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
