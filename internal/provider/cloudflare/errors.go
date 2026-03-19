package cloudflare

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	cloudflareapi "github.com/cloudflare/cloudflare-go/v6"
	"github.com/cloudflare/cloudflare-go/v6/shared"

	"dns-update/internal/httpclient"
	"dns-update/internal/retry"
)

func normalizeRequestError(ctx context.Context, action string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, httpclient.ErrRedirectNotAllowed) || context.Cause(ctx) != nil {
		return fmt.Errorf("%s: %w", action, err)
	}

	var apiErr *cloudflareapi.Error
	if errors.As(err, &apiErr) {
		wrapped := fmt.Errorf(
			"%s: Cloudflare API returned HTTP %d: %s",
			action,
			apiErr.StatusCode,
			formatAPIProblems(apiErr.Errors),
		)
		if retry.ShouldRetryHTTPStatus(apiErr.StatusCode) {
			return retry.Mark(wrapped, retryAfterFromResponse(apiErr.Response))
		}
		return wrapped
	}

	wrapped := fmt.Errorf("%s: %w", action, err)
	if isRetryableTransportError(err) {
		return retry.Mark(wrapped, 0)
	}
	return wrapped
}

func formatAPIProblems(problems []shared.ErrorData) string {
	if len(problems) == 0 {
		return "no error details returned"
	}

	messages := make([]string, 0, len(problems))
	for _, problem := range problems {
		if problem.Code == 0 {
			messages = append(messages, problem.Message)
			continue
		}
		messages = append(messages, fmt.Sprintf("%d: %s", problem.Code, problem.Message))
	}
	return strings.Join(messages, "; ")
}

func retryAfterFromResponse(response *http.Response) time.Duration {
	if response == nil {
		return 0
	}

	if retryAfterMs := strings.TrimSpace(response.Header.Get("Retry-After-Ms")); retryAfterMs != "" {
		ms, err := strconv.ParseFloat(retryAfterMs, 64)
		if err == nil && ms > 0 {
			return time.Duration(ms * float64(time.Millisecond))
		}
	}

	retryAfter, _ := retry.ParseRetryAfter(response.Header.Get("Retry-After"), time.Now())
	return retryAfter
}

func isRetryableTransportError(err error) bool {
	if err == nil {
		return false
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return !errors.Is(urlErr.Err, httpclient.ErrRedirectNotAllowed)
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}
