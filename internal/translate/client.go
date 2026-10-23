package translate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Translator translates a slice of texts from source language to target language.
type Translator interface {
	Translate(ctx context.Context, texts []string, sourceLang, targetLang string) ([]string, error)
}

// MyMemoryTranslator uses the public MyMemory Translation API (no key required).
type MyMemoryTranslator struct {
	client *http.Client
}

func NewMyMemoryTranslator() *MyMemoryTranslator {
	return &MyMemoryTranslator{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

type myMemoryResponse struct {
	ResponseData struct {
		TranslatedText string `json:"translatedText"`
	} `json:"responseData"`
	ResponseStatus int `json:"responseStatus"`
}

func (m *MyMemoryTranslator) Translate(ctx context.Context, texts []string, sourceLang, targetLang string) ([]string, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	src := myMemoryLang(sourceLang)
	tgt := myMemoryLang(targetLang)
	pair := src + "|" + tgt

	results := make([]string, len(texts))
	// Translate with small concurrency pool to avoid hammering the endpoint
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup
	var errOnce sync.Once
	var firstErr error

	for i, text := range texts {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if strings.TrimSpace(text) == "" {
			results[i] = ""
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, t string) {
			defer wg.Done()
			defer func() { <-sem }()

			u := fmt.Sprintf("https://api.mymemory.translated.net/get?q=%s&langpair=%s",
				url.QueryEscape(t), url.QueryEscape(pair))
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
			if err != nil {
				errOnce.Do(func() { firstErr = err })
				return
			}
			resp, err := m.client.Do(req)
			if err != nil {
				errOnce.Do(func() { firstErr = err })
				return
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				errOnce.Do(func() { firstErr = err })
				return
			}
			var res myMemoryResponse
			if err := json.Unmarshal(body, &res); err != nil {
				errOnce.Do(func() { firstErr = fmt.Errorf("mymemory response: %w", err) })
				return
			}
			results[idx] = res.ResponseData.TranslatedText
		}(i, text)
	}

	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return results, nil
}

func myMemoryLang(code string) string {
	switch strings.ToLower(code) {
	case "zh", "zh-cn", "chinese":
		return "zh-CN"
	case "en", "en-us", "english":
		return "en-GB"
	case "ja", "japanese":
		return "ja-JP"
	default:
		return code
	}
}

// OpenAICompatibleTranslator translates subtitles via any OpenAI-compatible completions endpoint.
type OpenAICompatibleTranslator struct {
	client  *http.Client
	baseURL string
	apiKey  string
	model   string
}

func NewOpenAICompatibleTranslator(baseURL, apiKey, model string) *OpenAICompatibleTranslator {
	baseURL = strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(baseURL, "/v1") && !strings.Contains(baseURL, "/v1/") {
		baseURL += "/v1"
	}
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &OpenAICompatibleTranslator{
		client:  &http.Client{Timeout: 60 * time.Second},
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
	}
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (o *OpenAICompatibleTranslator) Translate(ctx context.Context, texts []string, sourceLang, targetLang string) ([]string, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	prompt := fmt.Sprintf("Translate the following numbered subtitle lines from %s to %s. "+
		"Preserve the line numbers. Return ONLY the translated lines in the format: 1. Translated text\n\n",
		sourceLang, targetLang)

	for i, t := range texts {
		prompt += fmt.Sprintf("%d. %s\n", i+1, t)
	}

	reqBody, _ := json.Marshal(chatRequest{
		Model: o.model,
		Messages: []chatMessage{
			{Role: "system", Content: "You are a professional subtitle translator. Output only the numbered translations."},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.2,
	})

	url := o.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var chatRes chatResponse
	if err := json.Unmarshal(body, &chatRes); err != nil {
		return nil, fmt.Errorf("decode llm response: %w", err)
	}
	if chatRes.Error != nil && chatRes.Error.Message != "" {
		return nil, errors.New(chatRes.Error.Message)
	}
	if len(chatRes.Choices) == 0 {
		return nil, errors.New("empty response from translation model")
	}

	raw := chatRes.Choices[0].Message.Content
	return parseNumberedTranslations(raw, len(texts)), nil
}

func parseNumberedTranslations(output string, count int) []string {
	results := make([]string, count)
	lines := strings.Split(output, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		var num int
		var text string
		if n, _ := fmt.Sscanf(l, "%d. %s", &num, &text); n >= 1 {
			prefix := fmt.Sprintf("%d.", num)
			content := strings.TrimSpace(strings.TrimPrefix(l, prefix))
			if num >= 1 && num <= count {
				results[num-1] = content
			}
		}
	}
	return results
}
