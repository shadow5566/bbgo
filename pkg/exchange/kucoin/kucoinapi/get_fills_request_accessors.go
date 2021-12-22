// Code generated by "requestgen -type GetFillsRequest"; DO NOT EDIT.

package kucoinapi

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

func (r *GetFillsRequest) OrderID(orderID string) *GetFillsRequest {
	r.orderID = &orderID
	return r
}

func (r *GetFillsRequest) Symbol(symbol string) *GetFillsRequest {
	r.symbol = &symbol
	return r
}

func (r *GetFillsRequest) Side(side string) *GetFillsRequest {
	r.side = &side
	return r
}

func (r *GetFillsRequest) OrderType(orderType string) *GetFillsRequest {
	r.orderType = &orderType
	return r
}

func (r *GetFillsRequest) StartAt(startAt time.Time) *GetFillsRequest {
	r.startAt = &startAt
	return r
}

func (r *GetFillsRequest) EndAt(endAt time.Time) *GetFillsRequest {
	r.endAt = &endAt
	return r
}

func (r *GetFillsRequest) GetParameters() (map[string]interface{}, error) {
	var params = map[string]interface{}{}

	// check orderID field -> json key orderId
	if r.orderID != nil {
		orderID := *r.orderID

		// assign parameter of orderID
		params["orderId"] = orderID
	}

	// check symbol field -> json key symbol
	if r.symbol != nil {
		symbol := *r.symbol

		// assign parameter of symbol
		params["symbol"] = symbol
	}

	// check side field -> json key side
	if r.side != nil {
		side := *r.side

		switch side {
		case "buy", "sell":
			params["side"] = side

		default:
			return params, fmt.Errorf("side value %v is invalid", side)

		}

		// assign parameter of side
		params["side"] = side
	}

	// check orderType field -> json key type
	if r.orderType != nil {
		orderType := *r.orderType

		switch orderType {
		case "limit", "market", "limit_stop", "market_stop":
			params["type"] = orderType

		default:
			return params, fmt.Errorf("type value %v is invalid", orderType)

		}

		// assign parameter of orderType
		params["type"] = orderType
	}

	// check startAt field -> json key startAt
	if r.startAt != nil {
		startAt := *r.startAt

		// assign parameter of startAt
		// convert time.Time to milliseconds time
		params["startAt"] = strconv.FormatInt(startAt.UnixNano()/int64(time.Millisecond), 10)
	}

	// check endAt field -> json key endAt
	if r.endAt != nil {
		endAt := *r.endAt

		// assign parameter of endAt
		// convert time.Time to milliseconds time
		params["endAt"] = strconv.FormatInt(endAt.UnixNano()/int64(time.Millisecond), 10)
	}

	return params, nil
}

func (r *GetFillsRequest) GetParametersQuery() (url.Values, error) {
	query := url.Values{}

	params, err := r.GetParameters()
	if err != nil {
		return query, err
	}

	for k, v := range params {
		query.Add(k, fmt.Sprintf("%v", v))
	}

	return query, nil
}

func (r *GetFillsRequest) GetParametersJSON() ([]byte, error) {
	params, err := r.GetParameters()
	if err != nil {
		return nil, err
	}

	return json.Marshal(params)
}