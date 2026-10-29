package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

const (
	timeout     = 15 * time.Second
	maxResponse = 16 << 20
	keepError   = 512
)

type Registry struct {
	http *http.Client
}

func New() *Registry {
	return &Registry{http: &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}}
}

type disclosedContract struct {
	TemplateID       string `json:"templateId"`
	ContractID       string `json:"contractId"`
	CreatedEventBlob string `json:"createdEventBlob"`
	SynchronizerID   string `json:"synchronizerId"`
}

type choiceContext struct {
	ChoiceContextData  json.RawMessage     `json:"choiceContextData"`
	DisclosedContracts []disclosedContract `json:"disclosedContracts"`
}

func (c choiceContext) data() json.RawMessage {
	if len(c.ChoiceContextData) == 0 || string(c.ChoiceContextData) == "null" {
		return emptyContext()
	}

	return c.ChoiceContextData
}

type factory struct {
	FactoryID     string        `json:"factoryId"`
	ChoiceContext choiceContext `json:"choiceContext"`
}

type httpError struct {
	url    string
	status int
	body   string
}

func (e *httpError) Error() string {
	return fmt.Sprintf("registry %s: HTTP %d: %s", e.url, e.status, e.body)
}

func (r *Registry) factory(ctx context.Context, url string, choiceArguments any) (factory, error) {
	in := struct {
		ChoiceArguments    any  `json:"choiceArguments"`
		ExcludeDebugFields bool `json:"excludeDebugFields"`
	}{ChoiceArguments: choiceArguments, ExcludeDebugFields: true}

	var out factory

	err := r.post(ctx, url, in, &out)
	if err != nil {
		return factory{}, err
	}

	if out.FactoryID == "" {
		return factory{}, fmt.Errorf("%w: registry %s answered without a factory", domain.ErrLedger, url)
	}

	return out, nil
}

func (r *Registry) choiceContext(ctx context.Context, url string) (choiceContext, error) {
	in := struct {
		Meta               map[string]string `json:"meta"`
		ExcludeDebugFields bool              `json:"excludeDebugFields"`
	}{Meta: map[string]string{}, ExcludeDebugFields: true}

	var out choiceContext

	err := r.post(ctx, url, in, &out)

	var he *httpError
	if errors.As(err, &he) && he.status == http.StatusNotFound {
		return choiceContext{}, fmt.Errorf("%w: %w", domain.ErrContractGone, err)
	}

	return out, err
}

func (r *Registry) post(ctx context.Context, url string, in, out any) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	data, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("json.Marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("http.NewRequestWithContext: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := r.http.Do(req)
	if err != nil {
		return unavailable(fmt.Errorf("registry %s: %w", url, err))
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return unavailable(fmt.Errorf("registry %s: %w", url, err))
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		he := &httpError{url: url, status: resp.StatusCode, body: string(body[:min(len(body), keepError)])}
		if resp.StatusCode >= http.StatusInternalServerError || resp.StatusCode == http.StatusTooManyRequests {
			return unavailable(he)
		}

		return fmt.Errorf("%w: %w", domain.ErrLedger, he)
	}

	err = json.Unmarshal(body, out)
	if err != nil {
		return fmt.Errorf("%w: registry %s: %w", domain.ErrLedger, url, err)
	}

	return nil
}

func unavailable(err error) error {
	return fmt.Errorf("%w: %w: %w", domain.ErrLedger, domain.ErrLedgerUnavailable, err)
}
