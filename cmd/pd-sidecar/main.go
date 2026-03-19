/*
Copyright 2025 The llm-d Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/llm-d/llm-d-inference-scheduler/pkg/sidecar/proxy"
	"github.com/llm-d/llm-d-inference-scheduler/pkg/sidecar/version"
	"github.com/llm-d/llm-d-inference-scheduler/pkg/telemetry"
	"github.com/stretchr/testify/assert/yaml"
)

var (
	// supportedConnectors defines all valid P/D connector types
	supportedConnectors = []string{
		proxy.ConnectorNIXLV2,
		proxy.ConnectorSharedStorage,
		proxy.ConnectorSGLang,
	}
)

type configMap map[string]any

const (
	defaultPort             = 8000
	defaultvLLMPort         = 8001
	defaultDataParallelSize = 1
)

type SidecarConfig struct {
	Port                        int    `yaml:"port"`
	VLLMPort                    int    `yaml:"vllm-port"`
	VLLMDataParallelSize        int    `yaml:"data-parallel-size"`
	Connector                   string `yaml:"connector"`
	PrefillerUseTLS             bool   `yaml:"prefiller-use-tls"`
	DecoderUseTLS               bool   `yaml:"decoder-use-tls"`
	PrefillerInsecureSkipVerify bool   `yaml:"prefiller-tls-insecure-skip-verify"`
	DecoderInsecureSkipVerify   bool   `yaml:"decoder-tls-insecure-skip-verify"`
	SecureProxy                 bool   `yaml:"secure-proxy"`
	CertPath                    string `yaml:"cert-path"`
	EnableSSRFProtection        bool   `yaml:"enable-ssrf-protection"`
	InferencePoolNamespace      string `yaml:"inference-pool-namespace"`
	InferencePoolName           string `yaml:"inference-pool-name"`
	EnablePrefillerSampling     bool   `yaml:"enable-prefiller-sampling"`
	PoolGroup                   string `yaml:"pool-group"`
}

func NewSidecarConfig() *SidecarConfig {
	return &SidecarConfig{
		Port:                        defaultPort,
		VLLMPort:                    defaultvLLMPort,
		VLLMDataParallelSize:        defaultDataParallelSize,
		Connector:                   proxy.ConnectorNIXLV2,
		PrefillerUseTLS:             false,
		DecoderUseTLS:               false,
		PrefillerInsecureSkipVerify: false,
		DecoderInsecureSkipVerify:   false,
		SecureProxy:                 true,
		CertPath:                    "",
		EnableSSRFProtection:        false,
		InferencePoolNamespace:      os.Getenv("INFERENCE_POOL_NAMESPACE"),
		InferencePoolName:           os.Getenv("INFERENCE_POOL_NAMESPACE"),
		EnablePrefillerSampling:     func() bool { b, _ := strconv.ParseBool(os.Getenv("ENABLE_PREFILLER_SAMPLING")); return b }(),
		PoolGroup:                   proxy.DefaultPoolGroup,
	}
}

func main() {
	sidecarConfig := NewSidecarConfig()
	flag.IntVar(&sidecarConfig.Port, "port", sidecarConfig.Port, "the port the sidecar is listening on")
	flag.IntVar(&sidecarConfig.VLLMPort, "vllm-port", sidecarConfig.VLLMPort, "the port vLLM is listening on")
	flag.IntVar(&sidecarConfig.VLLMDataParallelSize, "data-parallel-size", sidecarConfig.VLLMDataParallelSize, "the vLLM DATA-PARALLEL-SIZE value")
	flag.StringVar(&sidecarConfig.Connector, "connector", sidecarConfig.Connector, "the P/D connector being used. Supported: "+strings.Join(supportedConnectors, ", "))
	flag.BoolVar(&sidecarConfig.PrefillerUseTLS, "prefiller-use-tls", sidecarConfig.PrefillerUseTLS, "whether to use TLS when sending requests to prefillers")
	flag.BoolVar(&sidecarConfig.DecoderUseTLS, "decoder-use-tls", sidecarConfig.DecoderUseTLS, "whether to use TLS when sending requests to the decoder")
	flag.BoolVar(&sidecarConfig.PrefillerInsecureSkipVerify, "prefiller-tls-insecure-skip-verify", sidecarConfig.PrefillerInsecureSkipVerify, "configures the proxy to skip TLS verification for requests to prefiller")
	flag.BoolVar(&sidecarConfig.DecoderInsecureSkipVerify, "decoder-tls-insecure-skip-verify", sidecarConfig.DecoderInsecureSkipVerify, "configures the proxy to skip TLS verification for requests to decoder")
	flag.BoolVar(&sidecarConfig.SecureProxy, "secure-proxy", sidecarConfig.SecureProxy, "Enables secure proxy. Defaults to true.")
	flag.StringVar(&sidecarConfig.CertPath,
		"cert-path", "", "The path to the certificate for secure proxy. The certificate and private key files "+
			"are assumed to be named tls.crt and tls.key, respectively. If not set, and secureProxy is enabled, "+
			"then a self-signed certificate is used (for testing).")
	flag.BoolVar(&sidecarConfig.EnableSSRFProtection, "enable-ssrf-protection", sidecarConfig.EnableSSRFProtection, "enable SSRF protection using InferencePool allowlisting")
	flag.StringVar(&sidecarConfig.InferencePoolNamespace, "inference-pool-namespace", sidecarConfig.InferencePoolNamespace, "the Kubernetes namespace to watch for InferencePool resources (defaults to INFERENCE_POOL_NAMESPACE env var)")
	flag.StringVar(&sidecarConfig.InferencePoolName, "inference-pool-name", sidecarConfig.InferencePoolName, "the specific InferencePool name to watch (defaults to INFERENCE_POOL_NAME env var)")
	flag.BoolVar(&sidecarConfig.EnablePrefillerSampling, "enable-prefiller-sampling", sidecarConfig.EnablePrefillerSampling, "if true, the target prefill instance will be selected randomly from among the provided prefill host values")
	flag.StringVar(&sidecarConfig.PoolGroup, "pool-group", sidecarConfig.PoolGroup, "group of the InferencePool this Endpoint Picker is associated with.")
	config := flag.String("config", "", "sidecar configuration in YAML. Example `--config={port: 8085, vllm-port: 8203}`")
	configFile := flag.String("config-file", "", "The path to sidecar configuration file. Example `--config-file=/etc/config/sidecar-config.yaml`")

	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine) // optional to allow zap logging control via CLI
	flag.Parse()

	logger := zap.New(zap.UseFlagOptions(&opts))
	log.SetLogger(logger)

	ctx := ctrl.SetupSignalHandler()
	log.IntoContext(ctx, logger)

	// Initialize tracing before creating any spans
	shutdownTracing, err := telemetry.InitTracing(ctx)
	if err != nil {
		// Log error but don't fail - tracing is optional
		logger.Error(err, "Failed to initialize tracing")
	}
	if shutdownTracing != nil {
		defer func() {
			if err := shutdownTracing(ctx); err != nil {
				logger.Error(err, "Failed to shutdown tracing")
			}
		}()
	}
	logger.Info("Proxy starting", "Built on", version.BuildRef, "From Git SHA", version.CommitSHA)

	var configMap map[string]any
	if *config != "" {
		configMap = extractConfigFromCLI(*config)
		if *configFile != "" {
			sidecarConfig.updateSidecarConfig(mergeYAMLConfigs(extractConfigFromFile(*configFile), configMap))
		}
		sidecarConfig.updateSidecarConfig(configMap)
	}
	if *configFile != "" {
		sidecarConfig.updateSidecarConfig(extractConfigFromFile(*configFile))
	}

	// Validate connector
	isValidConnector := false
	for _, validConnector := range supportedConnectors {
		if sidecarConfig.Connector == validConnector {
			isValidConnector = true
			break
		}
	}
	if !isValidConnector {
		logger.Info("Error: --connector must be one of: " + strings.Join(supportedConnectors, ", "))
		return
	}
	logger.Info("p/d connector validated", "connector", sidecarConfig.Connector)

	// Determine namespace and pool name for SSRF protection
	if sidecarConfig.EnableSSRFProtection {
		if sidecarConfig.InferencePoolNamespace == "" {
			logger.Info("Error: --inference-pool-namespace or INFERENCE_POOL_NAMESPACE environment variable is required when --enable-ssrf-protection is true")
			return
		}
		if sidecarConfig.InferencePoolName == "" {
			logger.Info("Error: --inference-pool-name or INFERENCE_POOL_NAME environment variable is required when --enable-ssrf-protection is true")
			return
		}

		logger.Info("SSRF protection enabled", "namespace", sidecarConfig.InferencePoolNamespace, "poolName", sidecarConfig.InferencePoolName)
	}

	// start reverse proxy HTTP server
	scheme := "http"
	if sidecarConfig.DecoderUseTLS {
		scheme = "https"
	}
	targetURL, err := url.Parse(scheme + "://localhost:" + strconv.Itoa(sidecarConfig.VLLMPort))
	if err != nil {
		logger.Error(err, "failed to create targetURL")
		return
	}

	proxyConfig := proxy.Config{
		Connector:                   sidecarConfig.Connector,
		PrefillerUseTLS:             sidecarConfig.PrefillerUseTLS,
		PrefillerInsecureSkipVerify: sidecarConfig.PrefillerInsecureSkipVerify,
		DecoderInsecureSkipVerify:   sidecarConfig.DecoderInsecureSkipVerify,
		DataParallelSize:            sidecarConfig.VLLMDataParallelSize,
		EnablePrefillerSampling:     sidecarConfig.EnablePrefillerSampling,
		SecureServing:               sidecarConfig.SecureProxy,
		CertPath:                    sidecarConfig.CertPath,
	}

	// Create SSRF protection validator
	validator, err := proxy.NewAllowlistValidator(sidecarConfig.EnableSSRFProtection, sidecarConfig.PoolGroup, sidecarConfig.InferencePoolNamespace, sidecarConfig.InferencePoolName)
	if err != nil {
		logger.Error(err, "failed to create SSRF protection validator")
		return
	}

	proxyServer := proxy.NewProxy(strconv.Itoa(sidecarConfig.Port), targetURL, proxyConfig)

	if err := proxyServer.Start(ctx, validator); err != nil {
		logger.Error(err, "failed to start proxy server")
	}
}

// isDefault checks whether flag was provided by user or is a default input
func isDefault(parameter string) bool {
	result := true
	flag.Visit(func(f *flag.Flag) {
		if f.Name == parameter {
			result = false
		}
	})
	return result
}

// extractConfigFromCLI extracts config provided directly as flag parameter
// "--config={port: 8085, vllm-port: 8203}"
func extractConfigFromCLI(config string) map[string]any {
	var temp map[string]any
	if err := yaml.Unmarshal([]byte(config), &temp); err != nil {
		// logger.Error(err, "Failed to unmarshal sidecar configuration")
		fmt.Printf("Failed to unmarshal sidecar configuration\n")
	}
	return temp

}

// extractConfigFromCLI extracts config from file path
// "--config-file=/etc/config/sidecar-config.yaml"
func extractConfigFromFile(configFile string) map[string]any {
	var temp map[string]any
	rawFile, err := os.ReadFile(configFile)
	if err != nil {
		// logger.Error(err, "Failed to read sidecar configuration file")
		fmt.Printf("Failed to read sidecar configuration file\n")
	}
	if err := yaml.Unmarshal(rawFile, &temp); err != nil {
		// logger.Error(err, "Failed to unmarshal sidecar configuration")
		fmt.Printf("Failed to unmarshal sidecar configuration\n")

	}
	return temp
}

// mergeYAMLConfigs merges YAML obtained from config file ("--config-file")  into YAML as CLI parameter ("--config"),
// gives higher priority to YAML as CLI parameter
func mergeYAMLConfigs(fileYAML, parameterYAML map[string]any) map[string]any {
	for k, v := range parameterYAML {
		if val, ok := fileYAML[k]; ok {
			dstMap, dstOk := val.(map[string]any)
			srcMap, srcOk := v.(map[string]any)
			if dstOk && srcOk {
				fileYAML[k] = mergeYAMLConfigs(dstMap, srcMap)
				continue
			}
		}
		fileYAML[k] = v
	}
	return fileYAML
}

// Update values from YAML only when:
// 1. YAML config contains non-zero value
// 2. sidecar config contains value not explicitely set by flag
func (s *SidecarConfig) updateSidecarConfig(configMap configMap) {
	if configMap["port"] != nil {
		if v, ok := configMap["port"].(int); ok {
			if s.Port == defaultPort {
				s.Port = v
			}
		} else {
			fmt.Println("Type assertion failed")
		}
	}
	if configMap["vllm-port"] != nil {
		if v, ok := configMap["vllm-port"].(int); ok {
			if s.VLLMPort == defaultvLLMPort {
				s.VLLMPort = v
			}
		} else {
			fmt.Println("Type assertion failed")
		}
	}
	if configMap["connector"] != nil {
		if v, ok := configMap["connector"].(string); ok {
			if isDefault("connector") {
				s.Connector = v
			}
		} else {
			fmt.Println("Type assertion failed")
		}
	}
	if configMap["data-parallel-size"] != nil {
		if v, ok := configMap["data-parallel-size"].(int); ok {
			if isDefault("data-parallel-size") {
				s.VLLMDataParallelSize = v
			}
		} else {
			fmt.Println("Type assertion failed")
		}
	}
	if configMap["prefiller-use-tls"] != nil {
		if v, ok := configMap["prefiller-use-tls"].(bool); ok {
			if isDefault("prefiller-use-tls") {
				s.PrefillerUseTLS = v
			}
		} else {
			fmt.Println("Type assertion failed")
		}
	}
	if configMap["decoder-use-tls"] != nil {
		if v, ok := configMap["decoder-use-tls"].(bool); ok {
			if isDefault("decoder-use-tls") {
				s.DecoderUseTLS = v
			}
		} else {
			fmt.Println("Type assertion failed")
		}
	}
	if configMap["prefiller-tls-insecure-skip-verify"] != nil {
		if v, ok := configMap["prefiller-tls-insecure-skip-verify"].(bool); ok {
			if isDefault("prefiller-tls-insecure-skip-verify") {
				s.PrefillerInsecureSkipVerify = v
			}
		} else {
			fmt.Println("Type assertion failed")
		}
	}
	if configMap["decoder-tls-insecure-skip-verify"] != nil {
		if v, ok := configMap["decoder-tls-insecure-skip-verify"].(bool); ok {
			if isDefault("decoder-tls-insecure-skip-verify") {
				s.DecoderInsecureSkipVerify = v
			}
		} else {
			fmt.Println("Type assertion failed")
		}
	}
	if configMap["secure-proxy"] != nil {
		if v, ok := configMap["secure-proxy"].(bool); ok {
			if isDefault("secure-proxy") {
				s.SecureProxy = v
			}
		} else {
			fmt.Println("Type assertion failed")
		}
	}
	if configMap["cert-path"] != nil {
		if v, ok := configMap["cert-path"].(string); ok {
			if isDefault("cert-path") {
				s.CertPath = v
			}
		} else {
			fmt.Println("Type assertion failed")
		}
	}
	if configMap["enable-ssrf-protection"] != nil {
		if v, ok := configMap["enable-ssrf-protection"].(string); ok {
			if isDefault("enable-ssrf-protection") {
				s.CertPath = v
			}
		} else {
			fmt.Println("Type assertion failed")
		}
	}
	if configMap["inference-pool-namespace"] != nil {
		if v, ok := configMap["inference-pool-namespace"].(string); ok {
			if isDefault("inference-pool-namespace") {
				s.InferencePoolNamespace = v
			}
		} else {
			fmt.Println("Type assertion failed")
		}
	}
	if configMap["inference-pool-name"] != nil {
		if v, ok := configMap["inference-pool-name"].(string); ok {
			if isDefault("inference-pool-name") {
				s.InferencePoolName = v
			}
		} else {
			fmt.Println("Type assertion failed")
		}
	}
	if configMap["enable-prefiller-sampling"] != nil {
		if v, ok := configMap["enable-prefiller-sampling"].(bool); ok {
			if isDefault("enable-prefiller-sampling") {
				s.EnablePrefillerSampling = v
			}
		} else {
			fmt.Println("Type assertion failed")
		}
	}
	if configMap["pool-group"] != nil {
		if v, ok := configMap["pool-group"].(string); ok {
			if isDefault("pool-group") {
				s.PoolGroup = v
			}
		} else {
			fmt.Println("Type assertion failed")
		}
	}
}
