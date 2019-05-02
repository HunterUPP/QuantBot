package api

///https://github.com/Biboxcom/API_Docs/wiki
import (
	"crypto/hmac"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/HunterUPP/QuantBot/constant"
	"github.com/HunterUPP/QuantBot/model"
	"github.com/bitly/go-simplejson"
	"github.com/miaolz123/conver"
)

// BIBOX the exchange struct of bibox.io
type BIBOX struct {
	stockTypeMap     map[string]string
	stocksTypeMap    map[string][2]string
	tradeTypeMap     map[string]string
	orderSideMap     map[int64]string
	recordsPeriodMap map[string]string
	minAmountMap     map[string]float64
	records          map[string][]Record
	host             string
	logger           model.Logger
	option           Option

	limit     float64
	lastSleep int64
	lastTimes int64
}

// NewBibox create an exchange struct of bibox.io
func NewBibox(opt Option) Exchange {
	return &BIBOX{
		stockTypeMap: map[string]string{
			"BTC/USDT":  "BTC_USDT",
			"ETH/USDT":  "ETH_USDT",
			"EOS/USDT":  "EOS_USDT",
			"ONT/USDT":  "ONT_USDT",
			"QTUM/USDT": "QTUM_USDT",
		},
		stocksTypeMap: map[string][2]string{
			"BTC/USDT":  [2]string{"BTC", "USDT"},
			"ETH/USDT":  [2]string{"ETH", "USDT"},
			"EOS/USDT":  [2]string{"EOS", "USDT"},
			"ONT/USDT":  [2]string{"ONT", "USDT"},
			"QTUM/USDT": [2]string{"QTUM", "USDT"},
		},
		tradeTypeMap: map[string]string{
			"buy":         constant.TradeTypeBuy,
			"sell":        constant.TradeTypeSell,
			"buy_market":  constant.TradeTypeBuy,
			"sell_market": constant.TradeTypeSell,
		},
		orderSideMap: map[int64]string{
			1: constant.TradeTypeBuy,
			2: constant.TradeTypeSell,
		},
		recordsPeriodMap: map[string]string{
			"M":   "1min",
			"M5":  "5min",
			"M15": "15min",
			"M30": "30min",
			"H":   "1hour",
			"D":   "1day",
			"W":   "1week",
		},
		minAmountMap: map[string]float64{
			"BTC/USDT":  0.001,
			"ETH/USDT":  0.001,
			"EOS/USDT":  0.001,
			"ONT/USDT":  0.001,
			"QTUM/USDT": 0.001,
		},
		records: make(map[string][]Record),
		host:    "https://api.bibox365.com/v1/",
		logger:  model.Logger{TraderID: opt.TraderID, ExchangeType: opt.Type},
		option:  opt,

		limit:     10.0,
		lastSleep: time.Now().UnixNano(),
	}
}

// Log print something to console
func (e *BIBOX) Log(msgs ...interface{}) {
	e.logger.Log(constant.INFO, "", 0.0, 0.0, msgs...)
}

// GetType get the type of this exchange
func (e *BIBOX) GetType() string {
	return e.option.Type
}

// GetName get the name of this exchange
func (e *BIBOX) GetName() string {
	return e.option.Name
}

// SetLimit set the limit calls amount per second of this exchange
func (e *BIBOX) SetLimit(times interface{}) float64 {
	e.limit = conver.Float64Must(times)
	return e.limit
}

// AutoSleep auto sleep to achieve the limit calls amount per second of this exchange
func (e *BIBOX) AutoSleep() {
	now := time.Now().UnixNano()
	interval := 1e+9/e.limit*conver.Float64Must(e.lastTimes) - conver.Float64Must(now-e.lastSleep)
	if interval > 0.0 {
		time.Sleep(time.Duration(conver.Int64Must(interval)))
	}
	e.lastTimes = 0
	e.lastSleep = now
}

// GetMinAmount get the min trade amonut of this exchange
func (e *BIBOX) GetMinAmount(stock string) float64 {
	return e.minAmountMap[stock]
}

func (e *BIBOX) getAuthJSON(url string, params []string) (json *simplejson.Json, err error) {
	e.lastTimes++
	resp, err := post_gateio(url, params, e.option.AccessKey, signSha512(params, e.option.SecretKey))
	if err != nil {
		return
	}
	return simplejson.NewJson(resp)
}

func (e *BIBOX) getSign(params string) string {
	e.lastTimes++
	key := []byte(e.option.SecretKey)
	mac := hmac.New(md5.New, key)
	mac.Write([]byte(params))
	return hex.EncodeToString(mac.Sum(nil))
	// return fmt.Sprintf("%x", mac.Sum(nil))
}

type UserAsset struct {
	Cmd  string         `json:"cmd"`
	Body UserAssetsBody `json:"body"`
}

type UserAssetsBody struct {
	Select int `json:"select"`
}

// GetAccount get the account detail of this exchange
func (e *BIBOX) GetAccount() interface{} {

	param := UserAsset{
		Cmd: "transfer/assets",
		Body: UserAssetsBody{
			Select: 1,
		},
	}
	params := []UserAsset{}
	params = append(params, param)
	cmds, _ := json.Marshal(&params)

	forms := []string{}
	cmdsItem := "cmds=" + string(cmds)
	keyItem := "apikey=" + e.option.AccessKey
	signItem := "sign=" + e.getSign(string(cmds))
	forms = append(forms, cmdsItem)
	forms = append(forms, keyItem)
	forms = append(forms, signItem)

	resp, err := post(e.host+"transfer", forms)
	if err != nil {
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, "GetAccount() error, ", err)
		return false
	}

	jsonResp, err := simplejson.NewJson(resp)
	if err != nil {
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, "GetAccount() error, ", err)
		return false
	}

	jsons := jsonResp.Get("result").GetIndex(0)
	balancesArray, err := jsons.Get("result").Get("assets_list").Array()
	if err != nil {
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, "GetAccount() error, ", err)
		return false
	}

	result := map[string]float64{
		"USDT":       0,
		"FrozenUSDT": 0,
		"BTC":        0,
		"FrozenBTC":  0,
		"ETH":        0,
		"FrozenETH":  0,
		"EOS":        0,
		"FrozenEOS":  0,
		"ONT":        0,
		"FrozenONT":  0,
		"QTUM":       0,
		"FrozenQTUM": 0,
	}
	for i, _ := range balancesArray {
		balance := jsons.Get("result").Get("assets_list").GetIndex(i)
		symbol := strings.ToUpper(balance.Get("coin_symbol").MustString())
		avail := balance.Get("balance").MustString()
		freeze := balance.Get("freeze").MustString()

		result[symbol] = conver.Float64Must(avail)
		result["Frozen"+symbol] = conver.Float64Must(freeze)
	}

	return result
}

// Trade place an order
func (e *BIBOX) Trade(tradeType string, stockType string, _price, _amount interface{}, msgs ...interface{}) interface{} {
	stockType = strings.ToUpper(stockType)
	tradeType = strings.ToUpper(tradeType)
	price := conver.Float64Must(_price)
	amount := conver.Float64Must(_amount)
	if _, ok := e.stockTypeMap[stockType]; !ok {
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, "Trade() error, unrecognized stockType: ", stockType)
		return false
	}
	switch tradeType {
	case constant.TradeTypeBuy:
		return e.buy(stockType, price, amount, msgs...)
	case constant.TradeTypeSell:
		return e.sell(stockType, price, amount, msgs...)
	default:
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, "Trade() error, unrecognized tradeType: ", tradeType)
		return false
	}
}

type OrderTrade struct {
	Cmd   string      `json:"cmd"`
	Index int         `json:"index"`
	Body  OrderDetail `json:"body"`
}

type OrderDetail struct {
	Pair         string  `json:"pair"`
	Account_type int     `json:"account_type"`
	Order_type   int     `json:"order_type"`
	Order_side   int     `json:"order_side"`
	Price        float64 `json:"price"`
	Amount       float64 `json:"amount"`
}

func (e *BIBOX) buy(stockType string, price, amount float64, msgs ...interface{}) interface{} {
	param := OrderTrade{
		Cmd:   "orderpending/trade",
		Index: 1,
		Body: OrderDetail{
			Pair:         e.stockTypeMap[stockType],
			Account_type: 0,
			Order_type:   2,
			Order_side:   1,
			Price:        price,
			Amount:       amount,
		},
	}
	params := []OrderTrade{}
	params = append(params, param)
	cmds, _ := json.Marshal(&params)

	forms := []string{}
	cmdsItem := "cmds=" + string(cmds)
	keyItem := "apikey=" + e.option.AccessKey
	signItem := "sign=" + e.getSign(string(cmds))
	forms = append(forms, cmdsItem)
	forms = append(forms, keyItem)
	forms = append(forms, signItem)

	resp, err := post(e.host+"orderpending", forms)
	if err != nil {
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, "buy() error, ", err)
		return false
	}

	fmt.Println("buy: ", string(resp))
	/// get result:
	jsonResp, err := simplejson.NewJson(resp)
	if err != nil {
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, "buy() error, ", err)
		return false
	}

	if result := jsonResp.Get("error").Interface(); result != nil {
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, "buy() error, the error message => ", jsonResp.Get("error").Get("msg").MustString())
		return false
	}

	jsons := jsonResp.Get("result").GetIndex(0)
	orderID := jsons.Get("result").MustInt64()
	// index := jsons.Get("index").MustInt64()
	// cmd := jsons.Get("cmd").MustString()

	e.logger.Log(constant.BUY, stockType, price, amount, msgs...)
	return fmt.Sprint(orderID)
}

func (e *BIBOX) sell(stockType string, price, amount float64, msgs ...interface{}) interface{} {
	param := OrderTrade{
		Cmd:   "orderpending/trade",
		Index: 1,
		Body: OrderDetail{
			Pair:         e.stockTypeMap[stockType],
			Account_type: 0,
			Order_type:   2,
			Order_side:   2,
			Price:        price,
			Amount:       amount,
		},
	}
	params := []OrderTrade{}
	params = append(params, param)
	cmds, _ := json.Marshal(&params)

	forms := []string{}
	cmdsItem := "cmds=" + string(cmds)
	keyItem := "apikey=" + e.option.AccessKey
	signItem := "sign=" + e.getSign(string(cmds))
	forms = append(forms, cmdsItem)
	forms = append(forms, keyItem)
	forms = append(forms, signItem)

	resp, err := post(e.host+"orderpending", forms)
	if err != nil {
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, "sell() error, ", err)
		return false
	}

	fmt.Println("sell: ", string(resp))
	/// get result:
	jsonResp, err := simplejson.NewJson(resp)
	if err != nil {
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, "sell() error, ", err)
		return false
	}

	if result := jsonResp.Get("error").Interface(); result != nil {
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, "sell() error, the error message => ", jsonResp.Get("error").Get("msg").MustString())
		return false
	}

	jsons := jsonResp.Get("result").GetIndex(0)
	orderID := jsons.Get("result").MustInt64()
	// index := jsons.Get("index").MustInt64()
	// cmd := jsons.Get("cmd").MustString()

	e.logger.Log(constant.SELL, stockType, price, amount, msgs...)
	return fmt.Sprint(orderID)
}

type OrderRequest struct {
	Cmd  string           `json:"cmd"`
	Body OrderRequestBody `json:"body"`
}

type OrderRequestBody struct {
	ID int64 `json:"id"`
}

// GetOrder get details of an order
func (e *BIBOX) GetOrder(stockType, id string) interface{} {
	idInt, err := strconv.ParseInt(id, 10, 64)
	param := OrderRequest{
		Cmd: "orderpending/order",
		Body: OrderRequestBody{
			ID: idInt,
		},
	}
	params := []OrderRequest{}
	params = append(params, param)
	cmds, _ := json.Marshal(&params)

	forms := []string{}
	cmdsItem := "cmds=" + string(cmds)
	keyItem := "apikey=" + e.option.AccessKey
	signItem := "sign=" + e.getSign(string(cmds))
	forms = append(forms, cmdsItem)
	forms = append(forms, keyItem)
	forms = append(forms, signItem)

	resp, err := post(e.host+"orderpending", forms)
	if err != nil {
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, "GetOrder() error, ", err)
		return false
	}

	fmt.Println("GetOrder resp: ", string(resp))
	/// get result:
	jsonResp, err := simplejson.NewJson(resp)
	if err != nil {
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, "GetOrder() error, ", err)
		return false
	}

	if result := jsonResp.Get("error").Interface(); result != nil {
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, "GetOrder() error, the error message => ", jsonResp.Get("error").Get("msg").MustString())
		return false
	}

	jsons := jsonResp.Get("result").GetIndex(0)
	orderJSON := jsons.Get("result")
	orderID := orderJSON.Get("id").MustInt64()
	price := orderJSON.Get("price").MustFloat64()
	amount := orderJSON.Get("amount").MustFloat64()
	dealAmount := orderJSON.Get("deal_amount").MustFloat64()
	tradeType := e.orderSideMap[orderJSON.Get("order_side").MustInt64()]
	symbol := orderJSON.Get("pair").MustString()

	return Order{
		ID:         fmt.Sprint(orderID),
		Price:      price,
		Amount:     amount,
		DealAmount: dealAmount,
		TradeType:  tradeType,
		StockType:  symbol,
	}
}

type OrderPendingRequest struct {
	Cmd  string           `json:"cmd"`
	Body OrderPendingBody `json:"body"`
}

type OrderPendingBody struct {
	Account_type    int    `json:"account_type"`
	Order_side      int    `json:"order_side"`
	Page            int    `json:"page"`
	Size            int    `json:"size"`
	Coin_symbol     string `json:"coin_symbol"`
	Currency_symbol string `json:"currency_symbol"`
}

// GetOrders get all unfilled orders
func (e *BIBOX) GetOrders(stockType string) interface{} {
	ordersBid := e.getOrders(1, stockType).([]Order)
	ordersAsk := e.getOrders(2, stockType).([]Order)
	orders := append(ordersBid, ordersAsk...)

	fmt.Println("GetOrders orders: ", orders)

	return orders
}

func (e *BIBOX) getOrders(tradeType int, stockType string) interface{} {
	coins, ok := e.stocksTypeMap[stockType]
	if !ok {
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, "GetOrders() error, unrecognized stockType: ", stockType)
		return false
	}
	param := OrderPendingRequest{
		Cmd: "orderpending/orderPendingList",
		Body: OrderPendingBody{
			Account_type:    0,
			Order_side:      tradeType,
			Page:            1,
			Size:            1000,
			Coin_symbol:     coins[0],
			Currency_symbol: coins[1],
		},
	}
	params := []OrderPendingRequest{}
	params = append(params, param)
	cmds, _ := json.Marshal(&params)

	forms := []string{}
	cmdsItem := "cmds=" + string(cmds)
	keyItem := "apikey=" + e.option.AccessKey
	signItem := "sign=" + e.getSign(string(cmds))
	forms = append(forms, cmdsItem)
	forms = append(forms, keyItem)
	forms = append(forms, signItem)

	resp, err := post(e.host+"orderpending", forms)
	if err != nil {
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, "GetOrders() error, ", err)
		return false
	}

	fmt.Println("GetOrders resp: ", string(resp))
	/// get result:
	jsonResp, err := simplejson.NewJson(resp)
	if err != nil {
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, "GetOrders() error, ", err)
		return false
	}

	if result := jsonResp.Get("error").Interface(); result != nil {
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, "GetOrders() error, the error message => ", jsonResp.Get("error").Get("msg").MustString())
		return false
	}

	orders := []Order{}
	jsons := jsonResp.Get("result").GetIndex(0)
	ordersJSON := jsons.Get("result").Get("items")
	count := len(ordersJSON.MustArray())
	for i := 0; i < count; i++ {
		orderJSON := ordersJSON.GetIndex(i)
		orders = append(orders, Order{

			ID:         fmt.Sprint(orderJSON.Get("id").MustInt64()),
			Price:      orderJSON.Get("price").MustFloat64(),
			Amount:     orderJSON.Get("amount").MustFloat64(),
			DealAmount: orderJSON.Get("deal_amount").MustFloat64(),
			TradeType:  e.orderSideMap[orderJSON.Get("order_side").MustInt64()],
			StockType:  orderJSON.Get("coin_symbol").MustString() + orderJSON.Get("currency_symbol").MustString(),
		})
	}

	return orders
}

// GetTrades get all filled orders recently
func (e *BIBOX) GetTrades(stockType string) interface{} {

	ordersBid := e.getOrders(1, stockType).([]Order)
	ordersAsk := e.getOrders(2, stockType).([]Order)
	orders := append(ordersBid, ordersAsk...)

	fmt.Println("GetTrades orders: ", orders)

	return orders
}

func (e *BIBOX) getTrades(tradeType int, stockType string) interface{} {
	coins, ok := e.stocksTypeMap[stockType]
	if !ok {
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, "getTrades() error, unrecognized stockType: ", stockType)
		return false
	}
	param := OrderPendingRequest{
		Cmd: "orderpending/pendingHistoryList",
		Body: OrderPendingBody{
			Account_type:    0,
			Order_side:      tradeType,
			Page:            1,
			Size:            1000,
			Coin_symbol:     coins[0],
			Currency_symbol: coins[1],
		},
	}
	params := []OrderPendingRequest{}
	params = append(params, param)
	cmds, _ := json.Marshal(&params)

	forms := []string{}
	cmdsItem := "cmds=" + string(cmds)
	keyItem := "apikey=" + e.option.AccessKey
	signItem := "sign=" + e.getSign(string(cmds))
	forms = append(forms, cmdsItem)
	forms = append(forms, keyItem)
	forms = append(forms, signItem)

	resp, err := post(e.host+"orderpending", forms)
	if err != nil {
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, "getTrades() error, ", err)
		return false
	}

	fmt.Println("getTrades resp: ", string(resp))
	/// get result:
	jsonResp, err := simplejson.NewJson(resp)
	if err != nil {
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, "getTrades() error, ", err)
		return false
	}

	if result := jsonResp.Get("error").Interface(); result != nil {
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, "getTrades() error, the error message => ", jsonResp.Get("error").Get("msg").MustString())
		return false
	}

	orders := []Order{}
	jsons := jsonResp.Get("result").GetIndex(0)
	ordersJSON := jsons.Get("result").Get("items")
	count := len(ordersJSON.MustArray())
	for i := 0; i < count; i++ {
		orderJSON := ordersJSON.GetIndex(i)
		orders = append(orders, Order{

			ID:         fmt.Sprint(orderJSON.Get("id").MustInt64()),
			Price:      orderJSON.Get("price").MustFloat64(),
			Amount:     orderJSON.Get("amount").MustFloat64(),
			DealAmount: orderJSON.Get("deal_amount").MustFloat64(),
			TradeType:  e.orderSideMap[orderJSON.Get("order_side").MustInt64()],
			StockType:  orderJSON.Get("coin_symbol").MustString() + orderJSON.Get("currency_symbol").MustString(),
		})
	}

	return orders
}

type OrderCancelRequest struct {
	Cmd   string          `json:"cmd"`
	Index int             `json:"index"`
	Body  OrderCancelBody `json:"body"`
}

type OrderCancelBody struct {
	ID int64 `json:"id"`
}

// CancelOrder cancel an order
func (e *BIBOX) CancelOrder(order Order) bool {
	idInt, err := strconv.ParseInt(order.ID, 10, 64)
	param := OrderCancelRequest{
		Cmd:   "orderpending/cancelTrade",
		Index: 1,
		Body: OrderCancelBody{
			ID: idInt,
		},
	}
	params := []OrderCancelRequest{}
	params = append(params, param)
	cmds, _ := json.Marshal(&params)

	forms := []string{}
	cmdsItem := "cmds=" + string(cmds)
	keyItem := "apikey=" + e.option.AccessKey
	signItem := "sign=" + e.getSign(string(cmds))
	forms = append(forms, cmdsItem)
	forms = append(forms, keyItem)
	forms = append(forms, signItem)

	resp, err := post(e.host+"orderpending", forms)
	if err != nil {
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, "CancelOrder() error, ", err)
		return false
	}

	fmt.Println("CancelOrder resp: ", string(resp))
	/// get result:
	jsonResp, err := simplejson.NewJson(resp)
	if err != nil {
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, "CancelOrder() error, ", err)
		return false
	}

	if result := jsonResp.Get("error").Interface(); result != nil {
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, "CancelOrder() error, the error message => ", jsonResp.Get("error").Get("msg").MustString())
		return false
	}

	jsons := jsonResp.Get("result").GetIndex(0)
	msg := jsons.Get("result").MustString()

	e.logger.Log(constant.CANCEL, order.StockType, order.Price, order.Amount-order.DealAmount, msg)
	return true
}

// GetTicker get market ticker & depth
func (e *BIBOX) GetTicker(stockType string, sizes ...interface{}) interface{} {
	ticker, err := e.getTicker(stockType, sizes...)
	if err != nil {
		e.logger.Log(constant.ERROR, "", 0.0, 0.0, err)
		return false
	}
	return ticker
}

// getTicker get market ticker & depth
func (e *BIBOX) getTicker(stockType string, sizes ...interface{}) (ticker Ticker, err error) {
	stockType = strings.ToUpper(stockType)
	if _, ok := e.stockTypeMap[stockType]; !ok {
		err = fmt.Errorf("GetTicker() error, unrecognized stockType: %+v", stockType)
		return
	}
	resp, err := get(fmt.Sprintf("http://data.bibox.io/api2/1/orderBook/%v_usdt", e.stockTypeMap[stockType]))
	if err != nil {
		err = fmt.Errorf("GetTicker() error, %+v", err)
		return
	}
	json, err := simplejson.NewJson(resp)
	if err != nil {
		err = fmt.Errorf("GetTicker() error, %+v", err)
		return
	}
	depthsJSON := json.Get("bids")
	for i := 0; i < len(depthsJSON.MustArray()); i++ {
		depthJSON := depthsJSON.GetIndex(i)
		ticker.Bids = append(ticker.Bids, OrderBook{
			Price:  depthJSON.GetIndex(0).MustFloat64(),
			Amount: depthJSON.GetIndex(1).MustFloat64(),
		})
	}
	depthsJSON = json.Get("asks")
	for i := len(depthsJSON.MustArray()); i > 0; i-- {
		depthJSON := depthsJSON.GetIndex(i - 1)
		ticker.Asks = append(ticker.Asks, OrderBook{
			Price:  depthJSON.GetIndex(0).MustFloat64(),
			Amount: depthJSON.GetIndex(1).MustFloat64(),
		})
	}
	if len(ticker.Bids) < 1 || len(ticker.Asks) < 1 {
		err = fmt.Errorf("GetTicker() error, can not get enough Bids or Asks")
		return
	}
	ticker.Buy = ticker.Bids[0].Price
	ticker.Sell = ticker.Asks[0].Price
	ticker.Mid = (ticker.Buy + ticker.Sell) / 2
	return
}

// GetRecords get candlestick data
func (e *BIBOX) GetRecords(stockType, period string, sizes ...interface{}) interface{} {
	return nil
}
