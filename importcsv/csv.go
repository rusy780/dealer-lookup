package importcsv

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"dealer-lookup/models"
)

var RequiredHeaders = []string{
	"vin", "year", "make", "model", "trim", "price", "miles",
}

func Read(path string) ([]models.Vehicle, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true

	headers, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read headers: %w", err)
	}
	for i := range headers {
		headers[i] = strings.ToLower(strings.TrimSpace(headers[i]))
	}
	idx := map[string]int{}
	for i, h := range headers {
		idx[h] = i
	}
	for _, h := range RequiredHeaders {
		if _, ok := idx[h]; !ok {
			return nil, fmt.Errorf("missing required header: %s", h)
		}
	}

	var out []models.Vehicle
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		row := map[string]string{}
		for i, h := range headers {
			if i < len(rec) {
				row[h] = strings.TrimSpace(rec[i])
			} else {
				row[h] = ""
			}
		}
		v := mapVehicle(row)
		out = append(out, v)
	}
	return out, nil
}

func mapVehicle(row map[string]string) models.Vehicle {
	return models.Vehicle{
		VIN:   row["vin"],
		Year:  atoi(row["year"]),
		Make:  row["make"],
		Model: row["model"],
		Trim:  row["trim"],
		Price: money(row["price"]),
		Miles: money(row["miles"]),
	}
}

func money(s string) int {
	s = strings.ReplaceAll(s, "$", "")
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if strings.Contains(s, ".") {
		f, _ := strconv.ParseFloat(s, 64)
		return int(f + 0.5)
	}
	n, _ := strconv.Atoi(s)
	return n
}

func atoi(s string) int { n, _ := strconv.Atoi(strings.TrimSpace(s)); return n }
