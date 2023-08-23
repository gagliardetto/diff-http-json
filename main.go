package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/goware/urlx"
	"k8s.io/klog/v2"
)

func main() {
	// {"jsonrpc": "2.0", "id": "99", "method": "getBlock", "params": [100955115]}
	body := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "99",
		"method":  "getBlock",
		"params":  []any{210384016, map[string]interface{}{"encoding": "base64", "maxSupportedTransactionVersion": 0}},
	}

	servers := []string{
		"http://127.0.0.1:8899",
		"https://api.mainnet-beta.solana.com",
	}

	// unique the servers:
	{
		seen := map[string]struct{}{}
		for _, server := range servers {
			if _, ok := seen[server]; ok {
				panic(fmt.Errorf("duplicate server: %s", server))
			}
			seen[server] = struct{}{}
		}
	}

	runID := time.Now().Unix()
	err := os.MkdirAll("bodies", 0o755)
	if err != nil {
		panic(err)
	}
	klog.Infof("runID: %d", runID)
	klog.Infof("Will save response bodies to %s/body_%d_*.json", mustAbsOsPath("bodies"), runID)

	responses := make([]any, len(servers))
	bodies := make([][]byte, len(servers))
	// send the request to the server
	for i, serverURL := range servers {
		klog.Infof("Sending request to %s", serverURL)
		startedAt := time.Now()
		parsedResult, originalBody, err := sendToServer(serverURL, body)
		if err != nil {
			panic(err)
		}
		klog.Infof("Got response from %s in %s", serverURL, time.Since(startedAt))
		responses[i] = parsedResult
		bodies[i] = originalBody

		// check that the result is equal to all the previous results
		if i == 0 {
			continue
		}
		klog.Infof("Comparing responses from %s and %s (this might take a while)", servers[i-1], serverURL)
		if !cmp.Equal(parsedResult, responses[i-1]) {
			if diff := cmp.Diff(responses[i-1], parsedResult); diff != "" {
				fmt.Printf(
					"mismatch : \n- = have in %s and not in %s\n+ = have in %s and not in %s\n",
					servers[i-1],
					serverURL,
					serverURL,
					servers[i-1],
				)
				fmt.Println(diff)
				// save the bodies to disk
				{
					err = saveBody(servers[i-1], bodies[i-1], runID)
					if err != nil {
						panic(err)
					}
					err = saveBody(serverURL, originalBody, runID)
					if err != nil {
						panic(err)
					}
				}
				panic("mismatch (-want +got):")
			}
		}
	}
}

func mustAbsOsPath(path string) string {
	// get absolute path to the file
	abs, err := filepath.Abs(path)
	if err != nil {
		panic(err)
	}
	return abs
}

func saveBody(u string, body []byte, runID int64) error {
	parsedURL, err := urlx.Parse(u)
	if err != nil {
		return fmt.Errorf("failed to parse url %s: %w", u, err)
	}
	fname := fmt.Sprintf("bodies/body_%d_%s.json", runID, parsedURL.Hostname())
	body = jsonPrettyPrintJsonBytes(body)
	err = os.WriteFile(fname, body, 0o644)
	if err != nil {
		return err
	}
	return nil
}

func jsonPrettyPrintJsonBytes(body []byte) []byte {
	var prettyJSON bytes.Buffer
	err := json.Indent(&prettyJSON, body, "", "\t")
	if err != nil {
		panic(err)
	}
	return prettyJSON.Bytes()
}

func sendToServer(serverURL string, bodyValue any) (any, []byte, error) {
	body, err := json.Marshal(bodyValue)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal body: %w", err)
	}
	// send the request to the server
	resp, err := http.Post(serverURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, nil, err
	}

	originalBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	bodyReader := bytes.NewReader(originalBody)
	// read the response
	var result map[string]any
	err = json.NewDecoder(bodyReader).Decode(&result)
	if err != nil {
		return nil, nil, err
	}

	return result, originalBody, nil
}
