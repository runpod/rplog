package rplog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"gitlab.com/efronlicht/enve"
)

const (
	// The maximum content size per request is 5MB
	maxContentSize = 5 * 1024 * 1024

	// The maximum size for a single log is 256kB
	maxLogSize = 256 * 1024

	// The maximum amount of logs that can be sent in a single request is 1000
	maxLogsPerBatch = 1000

	maxRetries = 5
)

var client = &http.Client{Timeout: 20 * time.Second}

// send a batch of logs to datadog, retrying up to 5 times
func send(buf *bytes.Buffer, apiKey, url string, batch []json.RawMessage) error {
	buf.Reset()
	// write the batch to the buffer as a JSON array
	if err := json.NewEncoder(buf).Encode(batch); err != nil {
		return fmt.Errorf("failed to encode batch: %s", err)
	}
	req, err := http.NewRequest("POST", url, buf)
	if err != nil {
		return fmt.Errorf("failed to create request: %s", err)
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", apiKey)
	var errs []error
	for i := 0; i < maxRetries; i++ {
		time.Sleep(10 * time.Millisecond * time.Duration(i)) // 0 on first iteration, 10ms on second, 20ms on third, etc.
		resp, err := client.Do(req)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			errs = append(errs, fmt.Errorf("failed to send logs: %s", resp.Status))
			continue
		}
		return nil // success! no need to retry

	}
	return fmt.Errorf("failed to send logs after %d retries: %v", maxRetries, errs)
}

// InitDatadog initializes the datadog logger with the given API key. It should be called once at the start of the program.
func InitDatadog(ctx context.Context, apiKey string) {
	once.Do(func() {
		logEvents := make(datadogBatchWriter, 1000)
		Init(os.Stderr, logEvents)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		go collectAndSendBatches(ctx, apiKey, logEvents, ticker.C)
	})
}

// collect log entries from the `in` channel and send them to datadog in batches.
func collectAndSendBatches(ctx context.Context, apiKey string, in <-chan json.RawMessage, tick <-chan time.Time) {
	batches := make([]json.RawMessage, 0)
	batchSize := 0
	buf := &bytes.Buffer{}
	url := enve.StringOr("RUNPOD_DATADOG_LOGS_URL", "https://in.logs.betterstack.com")
	for {
		select {
		case <-tick: // Flush the batch every tick
			if len(batches) > 0 {
				send(buf, apiKey, url, batches)
			}
			batches = batches[:0]
		case <-ctx.Done(): // Flush the batch and return
			if len(batches) > 0 {
				send(buf, apiKey, url, batches)
			}
			return
		case entry := <-in: // Collect entries
			// is this entry too large to send / will it make the batch too large?
			if len(batches) >= maxLogsPerBatch || len(entry)+batchSize >= maxContentSize {
				send(buf, apiKey, url, batches)
				batches = batches[:0]
				batchSize = 0
			}
			batches = append(batches, entry)
			batchSize += len(entry)
		}
	}
}

// "Write" a log entry to datadog by sending it to the channel to be read by `collectAndSendBatches`
func (w datadogBatchWriter) Write(b []byte) (int, error) {
	if len(b) > maxLogSize {
		return 0, fmt.Errorf("log entry too large: %d bytes > %d bytes", len(b), maxLogSize)
	}
	select {
	case w <- json.RawMessage(b):
		return len(b), nil
	default:
		return 0, fmt.Errorf("failed to write log, channel is full")
	}
}

type datadogBatchWriter chan json.RawMessage
