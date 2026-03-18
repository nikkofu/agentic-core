package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"agentic-core/internal/gateway/wecom"
)

type requestConfig struct {
	Endpoint       string
	CorpID         string
	AgentID        int64
	Token          string
	EncodingAESKey string
	Timestamp      string
	Nonce          string
	InnerXML       string
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "wecom callback simulator error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	cfg, err := parseArgs(args)
	if err != nil {
		return err
	}

	targetURL, body, err := buildCallbackRequest(cfg)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/xml")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("callback request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if len(respBody) > 0 {
		if _, err := stdout.Write(respBody); err != nil {
			return err
		}
	}
	return nil
}

func parseArgs(args []string) (requestConfig, error) {
	var cfg requestConfig
	var innerXMLPath string

	fs := flag.NewFlagSet("wecom-callback-sim", flag.ContinueOnError)
	fs.StringVar(&cfg.Endpoint, "endpoint", "http://127.0.0.1:8081/callbacks/wecom", "wecom callback endpoint")
	fs.StringVar(&cfg.CorpID, "corp-id", "", "wecom corp id")
	fs.Int64Var(&cfg.AgentID, "agent-id", 0, "wecom agent id")
	fs.StringVar(&cfg.Token, "token", "", "wecom callback token")
	fs.StringVar(&cfg.EncodingAESKey, "encoding-aes-key", "", "wecom encoding aes key")
	fs.StringVar(&cfg.Timestamp, "timestamp", "", "callback timestamp")
	fs.StringVar(&cfg.Nonce, "nonce", "", "callback nonce")
	fs.StringVar(&innerXMLPath, "inner-xml", "", "path to plaintext callback xml payload")
	if err := fs.Parse(args); err != nil {
		return requestConfig{}, err
	}

	if innerXMLPath == "" {
		return requestConfig{}, fmt.Errorf("inner-xml is required")
	}
	raw, err := os.ReadFile(innerXMLPath)
	if err != nil {
		return requestConfig{}, err
	}
	cfg.InnerXML = string(raw)

	if cfg.Timestamp == "" {
		cfg.Timestamp = fmt.Sprintf("%d", time.Now().Unix())
	}
	if cfg.Nonce == "" {
		cfg.Nonce = "nonce-" + cfg.Timestamp
	}
	return cfg, nil
}

func buildCallbackRequest(cfg requestConfig) (string, []byte, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return "", nil, fmt.Errorf("endpoint is required")
	}
	if strings.TrimSpace(cfg.CorpID) == "" {
		return "", nil, fmt.Errorf("corp id is required")
	}
	if cfg.AgentID <= 0 {
		return "", nil, fmt.Errorf("agent id must be positive")
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return "", nil, fmt.Errorf("token is required")
	}
	if strings.TrimSpace(cfg.EncodingAESKey) == "" {
		return "", nil, fmt.Errorf("encoding aes key is required")
	}
	if strings.TrimSpace(cfg.InnerXML) == "" {
		return "", nil, fmt.Errorf("inner xml is required")
	}

	codec, err := wecom.NewCodec(wecom.Config{
		CorpID:         cfg.CorpID,
		Token:          cfg.Token,
		EncodingAESKey: cfg.EncodingAESKey,
	})
	if err != nil {
		return "", nil, err
	}
	encrypted, err := codec.Encrypt([]byte(cfg.InnerXML))
	if err != nil {
		return "", nil, err
	}

	signature := codec.Signature(cfg.Timestamp, cfg.Nonce, encrypted)
	endpointURL, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return "", nil, err
	}
	query := endpointURL.Query()
	query.Set("msg_signature", signature)
	query.Set("timestamp", cfg.Timestamp)
	query.Set("nonce", cfg.Nonce)
	endpointURL.RawQuery = query.Encode()

	body := fmt.Sprintf(`<xml><ToUserName><![CDATA[%s]]></ToUserName><AgentID><![CDATA[%d]]></AgentID><Encrypt><![CDATA[%s]]></Encrypt></xml>`, cfg.CorpID, cfg.AgentID, encrypted)
	return endpointURL.String(), []byte(body), nil
}
