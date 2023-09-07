package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/go-cmp/cmp"
	"github.com/goware/urlx"
	"k8s.io/klog/v2"
)

type StringSliceVar struct {
	Value []string
}

func (s *StringSliceVar) Set(v string) error {
	s.Value = append(s.Value, v)
	return nil
}

func (s *StringSliceVar) String() string {
	return strings.Join(s.Value, ",")
}

func main() {
	// {"jsonrpc": "2.0", "id": "99", "method": "getBlock", "params": [100955115]}
	// body := map[string]any{
	// 	"jsonrpc": "2.0",
	// 	"id":      "99",
	// 	"method":  "getTransaction",
	// 	"params": []any{
	// 		"3ZoKehx5haKmM1r74Ni9Ezxc6Sa2vLqRQgKisETGUjLK3KX45yRTAbN4xZ4LXt9jXBBozvjQ4qTz5eJtq3PD6j2P",
	// 		map[string]any{
	// 			"encoding":                       "json",
	// 			"maxSupportedTransactionVersion": 0,
	// 		},
	// 	},
	// }
	var noSaveBody bool
	var fieldsToIgnore StringSliceVar
	var rpcList StringSliceVar
	flag.BoolVar(&noSaveBody, "no-save-body", false, "Don't save the response bodies to disk.")
	flag.Var(&fieldsToIgnore, "ignore-field", "Ignore the given field in the diff. Can be specified multiple times.")
	flag.Var(&rpcList, "rpc", "The RPC server to send the request to. Can be specified multiple times.")
	flag.Parse()
	// re request body is the args:
	bodyString := flag.Arg(0)
	if bodyString == "" {
		panic("no body provided")
	}
	// try to parse the body as json:
	var body map[string]any
	err := json.Unmarshal([]byte(bodyString), &body)
	if err != nil {
		panic(err)
	}
	fmt.Println("body:")
	spew.Dump(body)

	servers := rpcList.Value

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
	err = os.MkdirAll("bodies", 0o755)
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
		options := []cmp.Option{
			IgnoreFields(fieldsToIgnore.Value...),
		}
		if !cmp.Equal(parsedResult, responses[i-1], options...) {
			if diff := cmp.Diff(responses[i-1], parsedResult, options...); diff != "" {
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
		} else {
			klog.Infof(green("Responses from %s and %s are EQUAL"), servers[i-1], serverURL)
		}
	}
}

func green(s string) string {
	return fmt.Sprintf("\033[32m%s\033[0m", s)
}

// return a f cmd.Diff option that ignores the given fields (by name, at any depth).
func IgnoreFields(fields ...string) cmp.Option {
	return cmp.FilterPath(func(p cmp.Path) bool {
		for _, field := range fields {
			for _, path := range p {
				stringified := path.String()
				if strings.Contains(stringified, fmt.Sprintf("%q", field)) {
					return true
				}
			}
		}
		return false
	}, cmp.Ignore())
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
