package assistant

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/lieranderl/aestheticlab/internal/content"
	"google.golang.org/genai"
)

// A busy or failing Vertex endpoint (429, 5xx) is retried once, so one
// overloaded moment doesn't cost a visitor their answer.
func TestGeminiRetriesABusyEndpointOnce(t *testing.T) {
	for name, tc := range map[string]struct {
		statuses []int
		wantErr  bool
		wantHits int32
	}{
		"busy, then fine":  {[]int{http.StatusTooManyRequests, http.StatusOK}, false, 2},
		"timeout, then ok": {[]int{http.StatusGatewayTimeout, http.StatusOK}, false, 2},
		"busy twice":       {[]int{http.StatusServiceUnavailable, http.StatusServiceUnavailable, http.StatusOK}, true, 2},
		"bad request":      {[]int{http.StatusBadRequest, http.StatusOK}, true, 1},
	} {
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			status := tc.statuses[min(int(hits.Add(1))-1, len(tc.statuses)-1)]
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			if status == http.StatusOK {
				_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]}}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"error":{"message":"busy"}}`))
		}))
		cfg := geminiConfig("project", "eu")
		cfg.Backend, cfg.APIKey, cfg.Project, cfg.Location = genai.BackendGeminiAPI, "test", "", ""
		cfg.HTTPOptions.BaseURL = srv.URL
		*cfg.HTTPOptions.RetryOptions.InitialDelay = 0.01
		client, err := genai.NewClient(context.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		_, err = (&Gemini{client: client, model: "m"}).Generate(context.Background(), genai.Text("hi"), nil)
		if (err != nil) != tc.wantErr || hits.Load() != tc.wantHits {
			t.Errorf("%s: err = %v after %d requests; want error %v after %d", name, err, hits.Load(), tc.wantErr, tc.wantHits)
		}
		srv.Close()
	}
}

// A cut reply keeps only whole sentences: a decimal point in a price or a
// duration doesn't end one.
func TestWholeSentencesIgnoresDecimalPoints(t *testing.T) {
	for in, want := range map[string]string{
		"The treatment takes 1.5 hours and inclu":                  "",
		"It costs €12.50. The treatment takes 1.5 hours and inclu": "It costs €12.50.",
		"Lamination lasts up to 3 months. It costs 1.":             "Lamination lasts up to 3 months.",
		"Free times:\n• 10:00\n• 10:15 with Za":                    "Free times:\n• 10:00",
		"Is that all? Yes! It is…":                                 "Is that all? Yes! It is…",
		"Ламинирование держится до 3 месяцев. Процедура фикс":      "Ламинирование держится до 3 месяцев.",
	} {
		if got := wholeSentences(in); got != want {
			t.Errorf("wholeSentences(%q) = %q, want %q", in, got, want)
		}
	}
}

type tokenTestTransport func(*http.Request) (*http.Response, error)

func (f tokenTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestVertexChecksTheCompleteInputBeforeEachGeneration(t *testing.T) {
	for _, followup := range []bool{false, true} {
		name := "oversized initial prompt"
		if followup {
			name = "oversized follow-up with tool results"
		}
		t.Run(name, func(t *testing.T) {
			var calls []string
			transport := tokenTestTransport(func(req *http.Request) (*http.Response, error) {
				var body struct {
					SystemInstruction *genai.Content
					Tools             []*genai.Tool
					Contents          []*genai.Content
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					return nil, err
				}
				var payload string
				if strings.HasSuffix(req.URL.Path, ":countTokens") {
					calls = append(calls, "count")
					if body.SystemInstruction == nil || len(body.Tools) == 0 {
						t.Error("token count omitted the instructions or tool declarations")
					}
					payload = `{"totalTokens":31000}`
					if followup && len(calls) == 1 {
						payload = `{"totalTokens":2000}`
					}
					if followup && len(calls) == 3 && (len(body.Contents) != 3 || body.Contents[2].Parts[0].FunctionResponse == nil) {
						t.Error("follow-up count omitted the tool response")
					}
				} else {
					calls = append(calls, "generate")
					payload = `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"offer_booking","args":{}}}]}}],"usageMetadata":{"promptTokenCount":2000}}`
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(payload))}, nil
			})
			ctx := context.Background()
			cfg := &genai.ClientConfig{Backend: genai.BackendVertexAI, APIKey: "test",
				HTTPClient: &http.Client{Transport: transport}, HTTPOptions: genai.HTTPOptions{BaseURL: "https://vertex.test"}}
			client, err := genai.NewClient(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			a := New(Options{Model: &Gemini{client: client, model: "gemini-test"}})
			got, err := a.Reply(ctx, Knowledge{Content: &content.Localized{}}, nil, "Book")
			wantCalls := "count"
			var wantPrompt int64
			if followup {
				wantCalls, wantPrompt = "count,generate,count", 2000
			}
			if err == nil || !strings.Contains(err.Error(), "reserved token allowance") ||
				strings.Join(calls, ",") != wantCalls || got.Tokens.Prompt != wantPrompt || got.Unmetered != 0 {
				t.Fatalf("calls %v, answer %+v, err %v; want %s and %d billed input tokens", calls, got, err, wantCalls, wantPrompt)
			}
		})
	}
}
