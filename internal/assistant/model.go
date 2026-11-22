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

// geminiConfig is the client's configuration. A busy or failing endpoint
// (429, 5xx; the eu endpoint answered both in testing) is retried once
// after about a second, within the chat's reply timeout. Failed requests
// aren't billed, so the spending ledger's worst case still holds.
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

// Generate implements Model.
func (g *Gemini) Generate(ctx context.Context, contents []*genai.Content, cfg *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	return g.client.Models.GenerateContent(ctx, g.model, contents, cfg)
}

// Fake is a scripted stand-in for local development and the browser tests:
// no network and no cost. A message about times calls find_free_slots, one
// about booking calls offer_booking, for the first bookable treatment;
// anything else gets a fixed reply.
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
		return fakeCall(toolFindFreeSlots, service), nil
	case service != "" && bookWords.MatchString(msg):
		return fakeCall(toolOfferBooking, service), nil
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

func fakeCall(name, service string) *genai.GenerateContentResponse {
	return &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
		Content: genai.NewContentFromParts([]*genai.Part{{FunctionCall: &genai.FunctionCall{
			ID: "fake-1", Name: name, Args: map[string]any{"service_id": service},
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
	slots, _ := fr.Response["slots"].([]map[string]any)
	if len(slots) == 0 {
		return "I found no free times in that period. Tap Book to see the booking page."
	}
	var times []string
	for _, s := range slots[:min(4, len(slots))] {
		times = append(times, fmt.Sprintf("%s %s %s with %s", s["weekday"], s["date"], s["time"], s["staff"]))
	}
	return fmt.Sprintf("%s looks free: %s.", fr.Response["service"], strings.Join(times, "; "))
}
