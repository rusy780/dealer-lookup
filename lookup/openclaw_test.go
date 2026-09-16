package lookup

import (
	"strings"
	"testing"

	"dealer-lookup/models"
)

func TestParseOpenClawResponseAcceptsCompactTokenSavingShape(t *testing.T) {
	body := []byte(`{"r":[["1C4PJMDX0PD000001",29000,31000,30000,75]]}`)

	got, err := parseOpenClawResponse(body)
	if err != nil {
		t.Fatalf("parseOpenClawResponse() error = %v", err)
	}
	if len(got.Results) != 1 {
		t.Fatalf("results len = %d, want 1", len(got.Results))
	}
	res := got.Results[0]
	if res.VIN != "1C4PJMDX0PD000001" {
		t.Fatalf("VIN = %q", res.VIN)
	}
	if res.SuggestedPriceLow != 29000 || res.SuggestedPriceHigh != 31000 || res.RecommendedPrice != 30000 {
		t.Fatalf("prices = low %d high %d recommended %d", res.SuggestedPriceLow, res.SuggestedPriceHigh, res.RecommendedPrice)
	}
	if res.Confidence != 75 {
		t.Fatalf("expanded result = %+v", res)
	}
}

func TestEstimateTokenUseProjectsMillionRowCost(t *testing.T) {
	vehicles := []models.Vehicle{
		{VIN: "1C4PJMDX0PD000001", Year: 2024, Make: "Jeep", Model: "Grand Cherokee", Trim: "Limited", Price: 32000, Miles: 41234},
	}

	got := EstimateTokenUse(vehicles, 100, 0.05)
	if got.TotalTokens <= 0 {
		t.Fatal("TotalTokens was not estimated")
	}
	if got.EstimatedCost1M <= 0 {
		t.Fatal("EstimatedCost1M was not projected")
	}
}

func TestBuildOpenClawPromptUsesCompactRowIDs(t *testing.T) {
	vehicles := []models.Vehicle{
		{VIN: "1C4PJMDX0PD000001", Year: 2024, Make: "Jeep", Model: "Grand Cherokee", Trim: "Limited", Price: 32000, Miles: 41234},
	}

	_, userPrompt := buildOpenClawPrompt(vehicles)
	if want := "Rows id,Year,Make,Model,Trim,Price,Miles:"; !contains(userPrompt, want) {
		t.Fatalf("prompt missing %q:\n%s", want, userPrompt)
	}
	if contains(userPrompt, vehicles[0].VIN) {
		t.Fatalf("prompt includes VIN despite compact row ID mode:\n%s", userPrompt)
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
