// alogdbcli is just a reference example for demoing direct cloudflare sql usage.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

type cloudflareD1 struct {
	url, auth string
}

func newD1() (*cloudflareD1, error) {
	path := filepath.Join(os.Getenv("HOME"), ".config/.iio")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("alogdbcli.ReadD1Config: %v", err)
	}
	parts := strings.Fields(string(data))
	if len(parts) != 4 {
		return nil, fmt.Errorf("alogdbcli.BadD1Config parts=%d want=%d", len(parts), 4)
	}
	return &cloudflareD1{
		url:  fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/d1/database/%s/raw", parts[1], parts[2]),
		auth: "Bearer " + parts[3],
	}, nil
}

func (db *cloudflareD1) query(ctx context.Context, sql string, params ...string) ([][]any, error) {
	w := &bytes.Buffer{}
	fmt.Fprintf(w, "{\n  %q: %q,\n  %q: [\n", "sql", sql, "params")
	for i, p := range params {
		if i == len(params)-1 {
			fmt.Fprintf(w, "    %q\n", p)
		} else {
			fmt.Fprintf(w, "    %q,\n", p)
		}
	}
	fmt.Fprintf(w, "  ]\n}\n")
	fmt.Println(w)

	request, err := http.NewRequestWithContext(ctx, "POST", db.url, w)
	if err != nil {
		return nil, fmt.Errorf("alogdbcli.NewQueryRequest: %v", err)
	}
	request.Header.Set("Authorization", db.auth)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("alogdbcli.DoQuery: %v", err)
	}
	body, err := io.ReadAll(response.Body)
	if err := response.Body.Close(); err != nil {
		return nil, fmt.Errorf("alogdbcli.CloseQueryBody: %v", err)
	}
	if err != nil {
		return nil, fmt.Errorf("alogdbcli.ReadQueryBody: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("alogdbcli.QueryFailed status=%q: %s", response.Status, bytes.TrimSpace(body))
	}

	type resultsType struct {
		Rows [][]any
	}
	type resultType struct {
		Results resultsType
	}
	type queryResultType struct {
		Errors []json.RawMessage
		Result []resultType
	}
	result := queryResultType{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("alogdbcli.UnmarshalResponseBody: %v\n%s", err, body)
	}
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("alogdbcli.QueryFailed:\n%s", body)
	}
	return result.Result[0].Results.Rows, nil
}

func run(ctx context.Context) error {
	db, err := newD1()
	if err != nil {
		return fmt.Errorf("alogdbcli.NewD1: %v", err)
	}
	result, err := db.query(ctx, "select * from alogdb")
	if err != nil {
		return fmt.Errorf("alogdbcli.QueryD1: %v", err)
	}
	for _, r := range result {
		fmt.Printf("%v\n", r)
	}
	return nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
