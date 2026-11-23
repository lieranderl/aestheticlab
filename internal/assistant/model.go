package assistant

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"google.golang.org/genai"
)

// Gemini is a Gemini model on Vertex AI, reached with Application Default
// Credentials (the Cloud Run service account; gcloud locally).
type Gemini struct {
	client *genai.Client
	model  string
}

// NewGemini connects to model on Vertex AI in project at location ("eu"
// keeps requests and processing in the EU).
func NewGemini(ctx context.Context, project, location, model string) (*Gemini, error) {
	client, err := genai.NewClient(ctx, geminiConfig(project, location))
	if err != nil {
		return nil, fmt.Errorf("gemini client: %w", err)
	}
	return &Gemini{client: client, model: model}, nil
}

// geminiConfig lets the token preflight recover once from a transient error.
// Billable generation overrides this retry policy.
func geminiConfig(project, location string) *genai.ClientConfig {
	return &genai.ClientConfig{
		Backend: genai.BackendVertexAI, Project: project, Location: location,
		HTTPOptions: genai.HTTPOptions{RetryOptions: &genai.HTTPRetryOptions{
			Attempts:        genai.Ptr[int32](2),
			InitialDelay:    genai.Ptr(1.0),
			MaxDelay:        genai.Ptr(2.0),
			HTTPStatusCodes: []int32{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout},
		}},
	}
}

// checkInput counts each complete Vertex request, including the tool schema
// and the conversation accumulated during earlier rounds.
func (g *Gemini) checkInput(ctx context.Context, contents []*genai.Content, cfg *genai.GenerateContentConfig) error {
	count, err := g.client.Models.CountTokens(ctx, g.model, contents, &genai.CountTokensConfig{
		SystemInstruction: cfg.SystemInstruction,
		Tools:             cfg.Tools,
	})
	if err != nil {
		return fmt.Errorf("chat token count failed: %w", err)
	}
	// Leave room for provider-added formatting and token-accounting differences.
	const inputHeadroom = 1024
	if count == nil || count.TotalTokens <= 0 || count.TotalTokens > maxInputTokens-inputHeadroom {
		return fmt.Errorf("chat input exceeds the reserved token allowance or could not be counted")
	}
	return nil
}

// Generate implements Model.
func (g *Gemini) Generate(ctx context.Context, contents []*genai.Content, cfg *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	// A dropped response may hide a billable success. One reservation covers
	// one attempt, so do not allow the SDK to replay a generation.
	request := genai.GenerateContentConfig{}
	if cfg != nil {
		request = *cfg
	}
	options := genai.HTTPOptions{}
	if request.HTTPOptions != nil {
		options = *request.HTTPOptions
	}
	options.RetryOptions = &genai.HTTPRetryOptions{Attempts: genai.Ptr[int32](1)}
	request.HTTPOptions = &options
	return g.client.Models.GenerateContent(ctx, g.model, contents, &request)
}

// Fake is a scripted stand-in for local development and the browser tests:
// no network and no cost. A message about times calls find_free_slots with
// only_times, so the times found end the reply; one about booking calls
// offer_booking, for the first bookable treatment; anything else gets a
// fixed reply.
type Fake struct{}

var (
	bookable   = regexp.MustCompile(`(?m)^- service_id=(\S+) \| [^|]*\| [^|]*\| [^|]*\| ([^|]+) \|`)
	timesWords = regexp.MustCompile(`(?i)free|time|when|available|vrij|wanneer|libre|quand|свобод|когда|вільн|коли`)
	bookWords  = regexp.MustCompile(`(?i)book|boek|réserv|reserv|запис`)
)

// Generate implements Model.
func (Fake) Generate(_ context.Context, contents []*genai.Content, cfg *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	last := contents[len(contents)-1]
	if fr := last.Parts[0].FunctionResponse; fr != nil {
		return reply(fakeSummary(fr)), nil
	}
	msg := last.Parts[0].Text
	if i := strings.LastIndex(msg, "Visitor's new message:\n"); i >= 0 {
		msg = msg[i:]
	}
	service := ""
	for _, m := range bookable.FindAllStringSubmatch(cfg.SystemInstruction.Parts[0].Text, -1) {
		if strings.TrimSpace(m[2]) != "not bookable online" {
			service = m[1]
			break
		}
	}
	switch {
	case service != "" && timesWords.MatchString(msg):
		return fakeCall(toolFindFreeSlots, map[string]any{"service_id": service, "only_times": true}), nil
	case service != "" && bookWords.MatchString(msg):
		return fakeCall(toolOfferBooking, map[string]any{"service_id": service}), nil
	}
	return reply("I'm Aesthetic Lab's test assistant, without AI. Ask me for free times or to book, to try the tools."), nil
}

// noTokens is the Fake's usage: known, and nothing billed.
var noTokens = &genai.GenerateContentResponseUsageMetadata{}

func reply(text string) *genai.GenerateContentResponse {
	return &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
		Content: genai.NewContentFromText(text, genai.RoleModel), FinishReason: genai.FinishReasonStop,
	}}, UsageMetadata: noTokens}
}

func fakeCall(name string, args map[string]any) *genai.GenerateContentResponse {
	return &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
		Content: genai.NewContentFromParts([]*genai.Part{{FunctionCall: &genai.FunctionCall{
			ID: "fake-1", Name: name, Args: args,
		}}}, genai.RoleModel),
		FinishReason: genai.FinishReasonStop,
	}}, UsageMetadata: noTokens}
}

func fakeSummary(fr *genai.FunctionResponse) string {
	if msg, ok := fr.Response["error"].(string); ok {
		return "Sorry, I can't do that: " + msg + "."
	}
	if fr.Name == toolOfferBooking {
		return "Tap the button below to book."
	}
	// The times found are shown under the reply; it never lists them.
	if slots, _ := fr.Response["slots"].([]map[string]any); len(slots) > 0 {
		return fmt.Sprintf("Here are the free times I found for %s.", fr.Response["service"])
	}
	return "I found no free times in that period. Tap Book to see the booking page."
}
