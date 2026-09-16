package models

import (
	"strconv"
	"strings"
)

type Vehicle struct {
	VIN   string
	Year  int
	Make  string
	Model string
	Trim  string
	Price int
	Miles int
}

func (v Vehicle) Name() string {
	parts := []string{}
	if v.Year > 0 {
		parts = append(parts, strconv.Itoa(v.Year))
	}
	for _, p := range []string{v.Make, v.Model, v.Trim} {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " ")
}

const (
	PriceBucketSize = 1000
	MilesBucketSize = 10000
)

func (v Vehicle) GroupKey() string {
	priceLow, priceHigh := bucketRange(v.Price, PriceBucketSize)
	milesLow, milesHigh := bucketRange(v.Miles, MilesBucketSize)
	return strings.Join([]string{
		"group:" + strconv.Itoa(v.Year),
		norm(v.Make),
		norm(v.Model),
		norm(v.Trim),
		"price:" + strconv.Itoa(priceLow) + "-" + strconv.Itoa(priceHigh),
		"miles:" + strconv.Itoa(milesLow) + "-" + strconv.Itoa(milesHigh),
	}, "|")
}

func bucketRange(n, size int) (int, int) {
	if size <= 0 {
		return n, n
	}
	if n < 0 {
		n = 0
	}
	low := (n / size) * size
	return low, low + size - 1
}

func norm(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
