package market

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"nofx/hook"
	"strconv"
	"strings"
	"time"
)

const (
	baseURL    = "https://fapi.binance.com"
	okxBaseURL = "https://www.okx.com"
	bybitBase  = "https://api.bybit.com"
	bitgetBase = "https://api.bitget.com"
	gateBase   = "https://api.gateio.ws"
	kucoinBase = "https://api-futures.kucoin.com"
)

type APIClient struct {
	client *http.Client
}

func NewAPIClient() *APIClient {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	hookRes := hook.HookExec[hook.SetHttpClientResult](hook.SET_HTTP_CLIENT, client)
	if hookRes != nil && hookRes.Error() == nil {
		log.Printf("Using HTTP client set by Hook")
		client = hookRes.GetResult()
	}

	return &APIClient{
		client: client,
	}
}

func (c *APIClient) GetExchangeInfo() (*ExchangeInfo, error) {
	url := fmt.Sprintf("%s/fapi/v1/exchangeInfo", baseURL)
	resp, err := c.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var exchangeInfo ExchangeInfo
	err = json.Unmarshal(body, &exchangeInfo)
	if err != nil {
		return nil, err
	}

	return &exchangeInfo, nil
}

func (c *APIClient) GetKlines(symbol, interval string, limit int) ([]Kline, error) {
	url := fmt.Sprintf("%s/fapi/v1/klines", baseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("symbol", symbol)
	q.Add("interval", interval)
	q.Add("limit", strconv.Itoa(limit))
	req.URL.RawQuery = q.Encode()

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var klineResponses []KlineResponse
	err = json.Unmarshal(body, &klineResponses)
	if err != nil {
		log.Printf("Failed to get K-line data, response content: %s", string(body))
		return nil, err
	}

	var klines []Kline
	for _, kr := range klineResponses {
		kline, err := parseKline(kr)
		if err != nil {
			log.Printf("Failed to parse K-line data: %v", err)
			continue
		}
		klines = append(klines, kline)
	}

	return klines, nil
}

func parseKline(kr KlineResponse) (Kline, error) {
	var kline Kline

	if len(kr) < 11 {
		return kline, fmt.Errorf("invalid kline data")
	}

	// Parse each field
	kline.OpenTime = int64(kr[0].(float64))
	kline.Open, _ = strconv.ParseFloat(kr[1].(string), 64)
	kline.High, _ = strconv.ParseFloat(kr[2].(string), 64)
	kline.Low, _ = strconv.ParseFloat(kr[3].(string), 64)
	kline.Close, _ = strconv.ParseFloat(kr[4].(string), 64)
	kline.Volume, _ = strconv.ParseFloat(kr[5].(string), 64)
	kline.CloseTime = int64(kr[6].(float64))
	kline.QuoteVolume, _ = strconv.ParseFloat(kr[7].(string), 64)
	kline.Trades = int(kr[8].(float64))
	kline.TakerBuyBaseVolume, _ = strconv.ParseFloat(kr[9].(string), 64)
	kline.TakerBuyQuoteVolume, _ = strconv.ParseFloat(kr[10].(string), 64)

	return kline, nil
}

func (c *APIClient) GetCurrentPrice(symbol string) (float64, error) {
	url := fmt.Sprintf("%s/fapi/v1/ticker/price", baseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}

	q := req.URL.Query()
	q.Add("symbol", symbol)
	req.URL.RawQuery = q.Encode()

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var ticker PriceTicker
	err = json.Unmarshal(body, &ticker)
	if err != nil {
		return 0, err
	}

	price, err := strconv.ParseFloat(ticker.Price, 64)
	if err != nil {
		return 0, err
	}

	return price, nil
}

// GetOKXSwapPrice fetches latest price from OKX USDT-margined swap
// symbol expects Binance-style, e.g., BTCUSDT -> OKX instId BTC-USDT-SWAP
func (c *APIClient) GetOKXSwapPrice(symbol string) (float64, error) {
	if len(symbol) < 4 {
		return 0, fmt.Errorf("invalid symbol")
	}
	base := strings.TrimSuffix(strings.ToUpper(symbol), "USDT")
	if base == "" {
		return 0, fmt.Errorf("invalid symbol")
	}
	instId := fmt.Sprintf("%s-USDT-SWAP", base)
	url := fmt.Sprintf("%s/api/v5/market/ticker", okxBaseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	q := req.URL.Query()
	q.Add("instId", instId)
	req.URL.RawQuery = q.Encode()

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var data struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			Last string `json:"last"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return 0, err
	}
	if data.Code != "0" || len(data.Data) == 0 {
		return 0, fmt.Errorf("okx response error: code=%s msg=%s", data.Code, data.Msg)
	}
	return strconv.ParseFloat(data.Data[0].Last, 64)
}

// GetBybitLinearPrice fetches latest price from Bybit USDT perpetual (linear)
func (c *APIClient) GetBybitLinearPrice(symbol string) (float64, error) {
	url := fmt.Sprintf("%s/v5/market/tickers", bybitBase)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	q := req.URL.Query()
	q.Add("category", "linear")
	q.Add("symbol", strings.ToUpper(symbol))
	req.URL.RawQuery = q.Encode()
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	var data struct {
		RetCode int    `json:"retCode"`
		RetMsg  string `json:"retMsg"`
		Result  struct {
			List []struct {
				LastPrice string `json:"lastPrice"`
			} `json:"list"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return 0, err
	}
	if data.RetCode != 0 || len(data.Result.List) == 0 {
		return 0, fmt.Errorf("bybit error: %d %s", data.RetCode, data.RetMsg)
	}
	return strconv.ParseFloat(data.Result.List[0].LastPrice, 64)
}

// GetBitgetSwapPrice fetches latest price from Bitget USDT-M perp
// Bitget symbol format: BTCUSDT -> BTCUSDT_UMCBL
func (c *APIClient) GetBitgetSwapPrice(symbol string) (float64, error) {
	url := fmt.Sprintf("%s/api/mix/v1/market/ticker", bitgetBase)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	req.URL.Query().Add("symbol", fmt.Sprintf("%s_UMCBL", strings.ToUpper(symbol)))
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	var data struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Last string `json:"last"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return 0, err
	}
	if data.Code != "00000" {
		return 0, fmt.Errorf("bitget error: %s %s", data.Code, data.Msg)
	}
	return strconv.ParseFloat(data.Data.Last, 64)
}

// GetGateSwapPrice fetches latest price from Gate USDT perpetual (delivery=futures usdt)
// Gate symbol: BTC_USDT
func (c *APIClient) GetGateSwapPrice(symbol string) (float64, error) {
	url := fmt.Sprintf("%s/api/v4/futures/usdt/tickers", gateBase)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	contract := strings.ToUpper(strings.ReplaceAll(symbol, "USDT", "_USDT"))
	q := req.URL.Query()
	q.Add("contract", contract)
	req.URL.RawQuery = q.Encode()
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	var arr []struct {
		Last string `json:"last"`
	}
	if err := json.Unmarshal(body, &arr); err != nil {
		return 0, err
	}
	if len(arr) == 0 {
		return 0, fmt.Errorf("gate empty response")
	}
	return strconv.ParseFloat(arr[0].Last, 64)
}

// GetKucoinSwapPrice fetches latest price from KuCoin Futures
// KuCoin symbol: BTCUSDT -> BTCUSDTM
func (c *APIClient) GetKucoinSwapPrice(symbol string) (float64, error) {
	url := fmt.Sprintf("%s/api/v1/ticker", kucoinBase)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	q := req.URL.Query()
	q.Add("symbol", fmt.Sprintf("%sM", strings.ToUpper(symbol)))
	req.URL.RawQuery = q.Encode()
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	var data struct {
		Code string `json:"code"`
		Data struct {
			Price string `json:"price"`
		} `json:"data"`
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return 0, err
	}
	if data.Code != "200000" {
		return 0, fmt.Errorf("kucoin error: %s %s", data.Code, data.Msg)
	}
	return strconv.ParseFloat(data.Data.Price, 64)
}
