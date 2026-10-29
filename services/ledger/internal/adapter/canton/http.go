package canton

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

const (
	readTimeout   = 30 * time.Second
	submitTimeout = 120 * time.Second
	readyTimeout  = 10 * time.Second

	maxResponse = 64 << 20
)

const (
	categoryTransientServerFailure      = 1
	categoryContentionOnSharedResources = 2
	categoryDeadlineExceeded            = 3
)

type apiError struct {
	code  string
	cause string
}

func (e *apiError) Error() string {
	return e.code + ": " + e.cause
}

func ledgerError(code, cause string, category, status int) error {
	err := &apiError{code: code, cause: cause}

	switch {
	case code == "DUPLICATE_COMMAND":
		return fmt.Errorf("%w: %w: %w", domain.ErrLedger, domain.ErrDuplicateCommand, err)
	case code == "FAILED_TO_EXECUTE_TRANSACTION" && strings.Contains(cause, "valid signatures"):
		return fmt.Errorf("%w: %w", domain.ErrSignatureRejected, err)
	case code == "CONTRACT_NOT_FOUND" || code == "LOCAL_VERDICT_INACTIVE_CONTRACTS":
		return fmt.Errorf("%w: %w: %w", domain.ErrLedger, domain.ErrContractGone, err)
	case retryable(category, status):
		return unavailable(err)
	default:
		return fmt.Errorf("%w: %w", domain.ErrLedger, err)
	}
}

func retryable(category, status int) bool {
	return category == categoryTransientServerFailure ||
		category == categoryContentionOnSharedResources ||
		category == categoryDeadlineExceeded ||
		status >= http.StatusInternalServerError
}

func unavailable(err error) error {
	return fmt.Errorf("%w: %w: %w", domain.ErrLedger, domain.ErrLedgerUnavailable, err)
}

func (c *Canton) call(ctx context.Context, method, path string, in, out any, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var body io.Reader

	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("json.Marshal: %w", err)
		}

		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.url+path, body)
	if err != nil {
		return fmt.Errorf("http.NewRequestWithContext: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	token, err := c.token()
	if err != nil {
		return err
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return unavailable(err)
	}

	defer func() { _ = resp.Body.Close() }()

	return decodeResponse(resp, path, out)
}

func decodeResponse(resp *http.Response, path string, out any) error {
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return unavailable(fmt.Errorf("read %s: %w", path, err))
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return responseError(resp.StatusCode, data)
	}

	if out == nil || len(data) == 0 {
		return nil
	}

	err = json.Unmarshal(data, out)
	if err != nil {
		return fmt.Errorf("%w: decode %s: %w", domain.ErrLedger, path, err)
	}

	return nil
}

func responseError(status int, data []byte) error {
	var e struct {
		Code          string `json:"code"`
		Cause         string `json:"cause"`
		ErrorCategory int    `json:"errorCategory"`
	}

	if json.Unmarshal(data, &e) != nil || e.Code == "" {
		const keep = 512

		return ledgerError(fmt.Sprintf("HTTP_%d", status), string(data[:min(len(data), keep)]), 0, status)
	}

	return ledgerError(e.Code, e.Cause, e.ErrorCategory, status)
}
