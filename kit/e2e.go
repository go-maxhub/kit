package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/fatih/color"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type e2eHeader struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

type e2eTests []e2eTest

type e2eTest struct {
	URL        string        `yaml:"url"`
	AssertCode int           `yaml:"assert_code"`
	Method     string        `yaml:"method"`
	Timeout    time.Duration `yaml:"timeout,omitempty"`
	Headers    []e2eHeader   `yaml:"headers,omitempty"`
	Body       string        `yaml:"body,omitempty"`
}

func loadTestConfig(configPath string) (*e2eTests, error) {
	yamlFile, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read test configuration file: %v", err)
	}
	var et e2eTests
	if err = yaml.Unmarshal(yamlFile, &et); err != nil {
		return nil, fmt.Errorf("unmarshall test configuration: %v", err)
	}
	return &et, nil
}

func (s *Server) runTest(t e2eTest) {
	var (
		to  time.Duration
		buf bytes.Buffer
	)
	if t.Timeout != 0 {
		to = t.Timeout
	} else {
		to = 10 * time.Second
	}

	ctx, cncl := context.WithTimeout(context.Background(), to)
	defer cncl()

	if &t.Body != nil {
		err := json.NewEncoder(&buf).Encode(t.Body)
		if err != nil {
			log.Fatal(err)
		}
	}
	client := &http.Client{}
	req, err := http.NewRequestWithContext(ctx, t.Method, t.URL, &buf)
	if err != nil {
		s.DefaultLogger.Error("create http request", zap.Error(err))
	}
	resp, err := client.Do(req)
	if err != nil {
		s.DefaultLogger.Error("send http request", zap.Error(err))
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			s.DefaultLogger.Error("close http body", zap.Error(err))
		}
	}(resp.Body)

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	bodyString := string(bodyBytes)

	switch {
	case resp.StatusCode == t.AssertCode:
		color.Green("Test passed")
		s.DefaultLogger.Info("Test passed", zap.String("url", t.URL), zap.String("method", t.Method), zap.String("body", buf.String()))
	default:
		color.Red("Test failed")
		s.DefaultLogger.Error("Test failed", zap.String("url", t.URL), zap.String("method", t.Method), zap.String("body", buf.String()), zap.Int("status_code", resp.StatusCode), zap.Error(err), zap.String("response_body", bodyString))
		os.Exit(1)
	}
}
