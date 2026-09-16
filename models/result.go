package models

import "time"

type LookupResult struct {
	VIN                string    `json:"vin"`
	SuggestedPriceLow  int       `json:"suggested_price_low"`
	SuggestedPriceHigh int       `json:"suggested_price_high"`
	RecommendedPrice   int       `json:"recommended_price"`
	Confidence         int       `json:"confidence"`
	Summary            string    `json:"summary"`
	Flags              []string  `json:"flags"`
	Source             string    `json:"source"`
	Cached             bool      `json:"cached"`
	CreatedAt          time.Time `json:"created_at"`
	InputHash          string    `json:"input_hash"`
}
