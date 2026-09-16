package lookup

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"dealer-lookup/config"
	"dealer-lookup/models"
)

type Client struct {
	Cfg  config.Config
	HTTP *http.Client
}

func NewClient(cfg config.Config) Client {
	return Client{Cfg: cfg, HTTP: &http.Client{Timeout: cfg.OpenClawTimeout}}
}

type chatRequest struct {
	Model          string        `json:"model,omitempty"`
	Messages       []chatMessage `json:"messages"`
	Temperature    float64       `json:"temperature"`
	MaxTokens      int           `json:"max_tokens,omitempty"`
	Stream         bool          `json:"stream"`
	Format         any           `json:"format,omitempty"`
	ResponseFormat any           `json:"response_format,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Message *chatMessage `json:"message,omitempty"` // Ollama native sometimes uses this shape
	Content string       `json:"content,omitempty"`
}

type cliResponse struct {
	OK      bool `json:"ok"`
	Outputs []struct {
		Text string `json:"text"`
	} `json:"outputs"`
	Error string `json:"error,omitempty"`
}

type openClawResponse struct {
	Results []models.LookupResult `json:"results"`
}

type compactOpenClawResponse struct {
	Results []compactLookupRow `json:"r"`
}

type compactLookupRow struct {
	Key                string
	SuggestedPriceLow  int
	SuggestedPriceHigh int
	RecommendedPrice   int
	Confidence         int
}

type TokenEstimate struct {
	PromptTokens       int
	CompletionTokens   int
	TotalTokens        int
	EstimatedCost      float64
	EstimatedCost1M    float64
	RowsEstimated      int
	PricePerMillionTok float64
}

func (r *compactLookupRow) UnmarshalJSON(data []byte) error {
	var row []json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil {
		return err
	}
	if len(row) < 5 {
		return fmt.Errorf("compact result row has %d fields, want 5", len(row))
	}
	r.Key = compactKey(row[0])
	if r.Key == "" {
		return fmt.Errorf("compact result row has empty lookup key")
	}
	fields := []*int{&r.SuggestedPriceLow, &r.SuggestedPriceHigh, &r.RecommendedPrice, &r.Confidence}
	for i, field := range fields {
		if err := json.Unmarshal(row[i+1], field); err != nil {
			return err
		}
	}
	return nil
}

func (r compactOpenClawResponse) expand() openClawResponse {
	out := openClawResponse{Results: make([]models.LookupResult, 0, len(r.Results))}
	for _, item := range r.Results {
		out.Results = append(out.Results, models.LookupResult{
			VIN:                item.Key,
			SuggestedPriceLow:  item.SuggestedPriceLow,
			SuggestedPriceHigh: item.SuggestedPriceHigh,
			RecommendedPrice:   item.RecommendedPrice,
			Confidence:         item.Confidence,
		})
	}
	return out
}

func compactKey(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return strconv.Itoa(n)
	}
	return ""
}

func (c Client) BatchLookup(ctx context.Context, vehicles []models.Vehicle) (map[string]models.LookupResult, error) {
	out := map[string]models.LookupResult{}
	if len(vehicles) == 0 {
		return out, nil
	}
	if !c.Cfg.OpenClawEnabled || c.Cfg.OpenClawURL == "" {
		for _, v := range vehicles {
			out[v.VIN] = cheapMockResult(v)
		}
		return out, nil
	}

	batchSize := c.Cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 100
	}
	batches := buildBatches(vehicles, batchSize)
	concurrency := c.Cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > len(batches) {
		concurrency = len(batches)
	}

	if concurrency <= 1 {
		return c.lookupBatchesSerial(ctx, batches)
	}
	return c.lookupBatchesConcurrent(ctx, batches, concurrency)
}

type lookupBatch struct {
	Number int
	Start  int
	End    int
	Total  int
	Items  []models.Vehicle
}

func buildBatches(vehicles []models.Vehicle, batchSize int) []lookupBatch {
	batches := []lookupBatch{}
	for start := 0; start < len(vehicles); start += batchSize {
		end := start + batchSize
		if end > len(vehicles) {
			end = len(vehicles)
		}
		batches = append(batches, lookupBatch{
			Number: len(batches) + 1,
			Start:  start,
			End:    end,
			Total:  len(vehicles),
			Items:  vehicles[start:end],
		})
	}
	return batches
}

func (c Client) lookupBatchesSerial(ctx context.Context, batches []lookupBatch) (map[string]models.LookupResult, error) {
	out := map[string]models.LookupResult{}
	for _, batch := range batches {
		res, err := c.lookupOneBatch(ctx, batch, len(batches))
		if err != nil {
			return out, err
		}
		for vin, result := range res {
			out[vin] = result
		}
	}
	return out, nil
}

func (c Client) lookupBatchesConcurrent(ctx context.Context, batches []lookupBatch, concurrency int) (map[string]models.LookupResult, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan lookupBatch)
	errs := make(chan error, 1)
	out := map[string]models.LookupResult{}
	var outMu sync.Mutex
	var wg sync.WaitGroup

	for worker := 1; worker <= concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for batch := range jobs {
				res, err := c.lookupOneBatch(ctx, batch, len(batches))
				if err != nil {
					select {
					case errs <- err:
						cancel()
					default:
					}
					return
				}
				outMu.Lock()
				for vin, result := range res {
					out[vin] = result
				}
				outMu.Unlock()
			}
		}()
	}

	for _, batch := range batches {
		select {
		case <-ctx.Done():
			break
		case jobs <- batch:
		}
	}
	close(jobs)
	wg.Wait()

	select {
	case err := <-errs:
		return out, err
	default:
		return out, nil
	}
}

func (c Client) lookupOneBatch(ctx context.Context, batch lookupBatch, totalBatches int) (map[string]models.LookupResult, error) {
	out := map[string]models.LookupResult{}
	log.Printf("openclaw batch %d/%d: looking up rows %d-%d of %d", batch.Number, totalBatches, batch.Start+1, batch.End, batch.Total)
	batchStart := time.Now()
	res, err := c.callOpenClaw(ctx, batch.Items)
	if err != nil {
		if len(batch.Items) > 1 {
			log.Printf("openclaw batch %d/%d failed after %s; retrying as smaller batches: %v", batch.Number, totalBatches, time.Since(batchStart).Round(time.Second), err)
			return c.lookupSplitBatch(ctx, batch, totalBatches)
		}
		if c.Cfg.UseMockIfDown {
			log.Printf("openclaw batch %d/%d failed after %s; using local fallback: %v", batch.Number, totalBatches, time.Since(batchStart).Round(time.Second), err)
			for _, v := range batch.Items {
				out[v.VIN] = cheapMockResult(v)
			}
			return out, nil
		}
		return out, err
	}

	seen := map[string]bool{}
	vehicleByKey := map[string]models.Vehicle{}
	for i, v := range batch.Items {
		vehicleByKey[v.VIN] = v
		vehicleByKey[rowID(i)] = v
	}
	for _, r := range res.Results {
		key := strings.TrimSpace(r.VIN)
		if key == "" {
			continue
		}
		v, ok := vehicleByKey[key]
		if !ok {
			continue
		}
		r.VIN = v.VIN
		r.Source = "local-openclaw"
		if r.CreatedAt.IsZero() {
			r.CreatedAt = time.Now().UTC()
		}
		out[v.VIN] = normalizeResult(r, v)
		seen[v.VIN] = true
	}
	if c.Cfg.UseMockIfDown {
		for _, v := range batch.Items {
			if !seen[v.VIN] {
				out[v.VIN] = cheapMockResult(v)
			}
		}
	}
	log.Printf("openclaw batch %d/%d done in %s: received %d/%d results", batch.Number, totalBatches, time.Since(batchStart).Round(time.Second), len(seen), len(batch.Items))
	return out, nil
}

func (c Client) lookupSplitBatch(ctx context.Context, batch lookupBatch, totalBatches int) (map[string]models.LookupResult, error) {
	mid := len(batch.Items) / 2
	left := lookupBatch{
		Number: batch.Number,
		Start:  batch.Start,
		End:    batch.Start + mid,
		Total:  batch.Total,
		Items:  batch.Items[:mid],
	}
	right := lookupBatch{
		Number: batch.Number,
		Start:  batch.Start + mid,
		End:    batch.End,
		Total:  batch.Total,
		Items:  batch.Items[mid:],
	}

	out, err := c.lookupOneBatch(ctx, left, totalBatches)
	if err != nil {
		return out, err
	}
	rightOut, err := c.lookupOneBatch(ctx, right, totalBatches)
	if err != nil {
		return out, err
	}
	for vin, result := range rightOut {
		out[vin] = result
	}
	return out, nil
}

func (c Client) callOpenClaw(ctx context.Context, vehicles []models.Vehicle) (openClawResponse, error) {
	if c.usesOpenClawCLI() {
		return c.callOpenClawCLI(ctx, vehicles)
	}
	return c.callOpenClawHTTP(ctx, vehicles)
}

func (c Client) usesOpenClawCLI() bool {
	mode := strings.ToLower(strings.TrimSpace(c.Cfg.OpenClawURL))
	return mode == "" || mode == "cli" || mode == "openclaw"
}

func (c Client) callOpenClawCLI(ctx context.Context, vehicles []models.Vehicle) (openClawResponse, error) {
	systemPrompt, userPrompt := buildOpenClawPrompt(vehicles)
	prompt := systemPrompt + "\n" + userPrompt

	cmdCtx := ctx
	cancel := func() {}
	if c.Cfg.OpenClawTimeout > 0 {
		cmdCtx, cancel = context.WithTimeout(ctx, c.Cfg.OpenClawTimeout)
	}
	defer cancel()

	args := []string{"infer", "model", "run", "--json", "--prompt", prompt}
	if strings.TrimSpace(c.Cfg.OpenClawModel) != "" {
		args = append(args, "--model", strings.TrimSpace(c.Cfg.OpenClawModel))
	}
	cmd := exec.CommandContext(cmdCtx, "openclaw", args...)
	body, err := cmd.CombinedOutput()
	if cmdCtx.Err() == context.DeadlineExceeded {
		return openClawResponse{}, fmt.Errorf("openclaw CLI timed out after %s", c.Cfg.OpenClawTimeout)
	}
	if err != nil {
		return openClawResponse{}, fmt.Errorf("openclaw CLI failed: %w: %s", err, strings.TrimSpace(string(body)))
	}

	var parsed cliResponse
	if err := json.Unmarshal(body, &parsed); err == nil && len(parsed.Outputs) > 0 {
		if !parsed.OK {
			return openClawResponse{}, fmt.Errorf("openclaw CLI returned ok=false: %s", parsed.Error)
		}
		return parseJSONContent(parsed.Outputs[0].Text)
	}
	return parseOpenClawResponse(body)
}

func (c Client) callOpenClawHTTP(ctx context.Context, vehicles []models.Vehicle) (openClawResponse, error) {
	systemPrompt, userPrompt := buildOpenClawPrompt(vehicles)

	reqBody := chatRequest{
		Model:          c.Cfg.OpenClawModel,
		Messages:       []chatMessage{{Role: "system", Content: systemPrompt}, {Role: "user", Content: userPrompt}},
		Temperature:    c.Cfg.Temperature,
		MaxTokens:      c.Cfg.MaxTokens,
		Stream:         false,
		ResponseFormat: map[string]string{"type": "json_object"},
		Format: map[string]string{
			"type": "json_object",
		},
	}
	b, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Cfg.OpenClawURL, bytes.NewReader(b))
	if err != nil {
		return openClawResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return openClawResponse{}, fmt.Errorf("openclaw HTTP request failed for %s: %w (use OPENCLAW_BASE_URL=cli for the OpenClaw CLI transport)", c.Cfg.OpenClawURL, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return openClawResponse{}, fmt.Errorf("openclaw status %d: %s", resp.StatusCode, string(body))
	}

	return parseOpenClawResponse(body)
}

func buildOpenClawPrompt(vehicles []models.Vehicle) (string, string) {
	var rows strings.Builder
	w := csv.NewWriter(&rows)
	for i, v := range vehicles {
		_ = w.Write([]string{
			rowID(i),
			strconv.Itoa(v.Year),
			v.Make,
			v.Model,
			v.Trim,
			strconv.Itoa(v.Price),
			strconv.Itoa(v.Miles),
		})
	}
	w.Flush()

	systemPrompt := `JSON only.`
	userPrompt := fmt.Sprintf(`Return {"r":[[id,low,high,rec,conf]]}. low<=rec<=high. conf 55-85. Rows id,Year,Make,Model,Trim,Price,Miles:
%s`, strings.TrimSpace(rows.String()))
	return systemPrompt, userPrompt
}

func rowID(i int) string {
	return strconv.Itoa(i + 1)
}

func EstimateTokenUse(vehicles []models.Vehicle, batchSize int, pricePerMillion float64) TokenEstimate {
	if len(vehicles) == 0 {
		return TokenEstimate{PricePerMillionTok: pricePerMillion}
	}
	if batchSize <= 0 {
		batchSize = 10
	}
	estimate := TokenEstimate{RowsEstimated: len(vehicles), PricePerMillionTok: pricePerMillion}
	for start := 0; start < len(vehicles); start += batchSize {
		end := start + batchSize
		if end > len(vehicles) {
			end = len(vehicles)
		}
		systemPrompt, userPrompt := buildOpenClawPrompt(vehicles[start:end])
		estimate.PromptTokens += approxTokens(systemPrompt) + approxTokens(userPrompt)
		for i, v := range vehicles[start:end] {
			estimate.CompletionTokens += approxTokens(fmt.Sprintf(`[%s,%d,%d,%d,%d]`, rowID(i), v.Price, v.Price, v.Price, 75))
		}
	}
	estimate.TotalTokens = estimate.PromptTokens + estimate.CompletionTokens
	estimate.EstimatedCost = float64(estimate.TotalTokens) / 1_000_000 * pricePerMillion
	if len(vehicles) > 0 {
		estimate.EstimatedCost1M = estimate.EstimatedCost / float64(len(vehicles)) * 1_000_000
	}
	return estimate
}

func approxTokens(s string) int {
	if s == "" {
		return 0
	}
	return int(math.Ceil(float64(len(s)) / 4.0))
}

func parseOpenClawResponse(body []byte) (openClawResponse, error) {
	// Direct shape: {"results": [...]}
	var direct openClawResponse
	if err := json.Unmarshal(body, &direct); err == nil && len(direct.Results) > 0 {
		return direct, nil
	}

	// Chat completions shape: {"choices":[{"message":{"content":"{...}"}}]}
	var chat chatResponse
	if err := json.Unmarshal(body, &chat); err == nil {
		content := ""
		if len(chat.Choices) > 0 {
			content = chat.Choices[0].Message.Content
		} else if chat.Message != nil {
			content = chat.Message.Content
		} else if chat.Content != "" {
			content = chat.Content
		}
		if content != "" {
			return parseJSONContent(content)
		}
	}

	// Last try: extract a JSON object from a raw text response.
	return parseJSONContent(string(body))
}

func parseJSONContent(content string) (openClawResponse, error) {
	original := content
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	if start := strings.Index(content, "{"); start >= 0 {
		if end := strings.LastIndex(content, "}"); end > start {
			content = content[start : end+1]
		}
	}

	var compact compactOpenClawResponse
	if err := json.Unmarshal([]byte(content), &compact); err == nil && len(compact.Results) > 0 {
		return compact.expand(), nil
	}

	var parsed openClawResponse
	if err := json.Unmarshal([]byte(content), &parsed); err == nil && len(parsed.Results) > 0 {
		return parsed, nil
	}
	errContent := content
	if errContent == "" {
		errContent = original
	}
	return openClawResponse{}, fmt.Errorf("could not parse local OpenClaw JSON: %s", truncateForError(errContent, 1000))
}

func truncateForError(s string, limit int) string {
	if limit <= 0 || len(s) <= limit {
		return s
	}
	return s[:limit] + "...(truncated)"
}

func normalizeResult(r models.LookupResult, v models.Vehicle) models.LookupResult {
	if r.RecommendedPrice <= 0 {
		if r.SuggestedPriceLow > 0 && r.SuggestedPriceHigh > 0 {
			r.RecommendedPrice = roundPrice(float64(r.SuggestedPriceLow+r.SuggestedPriceHigh) / 2)
		}
	}
	if r.SuggestedPriceLow > r.SuggestedPriceHigh && r.SuggestedPriceHigh > 0 {
		r.SuggestedPriceLow, r.SuggestedPriceHigh = r.SuggestedPriceHigh, r.SuggestedPriceLow
	}
	if r.Confidence <= 0 {
		r.Confidence = 65
	}
	if r.Summary == "" {
		r.Summary = summaryForVehicle(v)
	}
	if len(r.Flags) == 0 {
		r.Flags = flagsForVehicle(v)
	}
	return r
}

func cheapMockResult(v models.Vehicle) models.LookupResult {
	base := v.Price
	if base <= 0 {
		base = 20000
	}

	discount := 0.02
	flags := flagsForVehicle(v)
	if v.Miles >= 100000 {
		discount = 0.04
	}

	rec := roundPrice(float64(base) * (1 - discount))
	spread := math.Max(750, float64(rec)*0.035)
	return models.LookupResult{
		VIN:                v.VIN,
		SuggestedPriceLow:  roundPrice(float64(rec) - spread),
		SuggestedPriceHigh: roundPrice(float64(rec) + spread),
		RecommendedPrice:   rec,
		Confidence:         70,
		Summary:            summaryForVehicle(v),
		Flags:              flags,
		Source:             "local-cheap-fallback",
		CreatedAt:          time.Now().UTC(),
	}
}

func summaryForVehicle(v models.Vehicle) string {
	name := v.Name()
	if name == "" {
		name = "Vehicle"
	}
	return fmt.Sprintf("%s price review.", name)
}

func flagsForVehicle(v models.Vehicle) []string {
	flags := []string{"PRICE_REVIEW"}
	if v.Miles >= 100000 {
		flags = append(flags, "HIGH_MILES")
	}
	return flags
}

func roundPrice(x float64) int {
	return int(math.Round(x/100.0) * 100)
}
