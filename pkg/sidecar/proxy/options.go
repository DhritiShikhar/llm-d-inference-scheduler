/*
Copyright 2026 The llm-d Authors.

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

package proxy

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/pflag"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/yaml"
)

// var defaultTemp = make([]string, 20)

var defaultTemp = []string{}
var fileTemp = make([]string, 20)
var inlineTemp = make([]string, 20)
var flagTemp = make([]string, 20)

// Options holds all configuration options for the pd-sidecar proxy.
type Options struct {
	Port             string // Port is the port the sidecar is listening on
	VLLMPort         string // VLLMPort is the port vLLM is listening on
	TargetURL        string // TargetURL is the target URL for the proxy
	DataParallelSize int    // DataParallelSize is the vLLM DATA-PARALLEL-SIZE value
	// KVConnector is the KV protocol between Prefiller and Decoder
	KVConnector string
	// ECConnector is the EC protocol between Encoder and Prefiller (for EPD mode)
	ECConnector string
	// Deprecated: Use KVConnector instead. Connector is the P/D connector being used
	Connector                      string
	EnableTLS                      []string // EnableTLS stages to enable TLS for (new StringSlice flag)
	TLSInsecureSkipVerify          []string // TLSInsecureSkipVerify stages to skip TLS verification for (new StringSlice flag)
	UseTLSForPrefiller             bool     // UseTLSForPrefiller indicates whether to use TLS when sending requests to prefillers (set from EnableTLS)
	UseTLSForEncoder               bool     // UseTLSForEncoder indicates whether to use TLS when sending requests to encoders (set from EnableTLS)
	UseTLSForDecoder               bool     // UseTLSForDecoder indicates whether to use TLS when sending requests to the decoder (set from EnableTLS)
	InsecureSkipVerifyForPrefiller bool     // InsecureSkipVerifyForPrefiller configures the proxy to skip TLS verification for requests to prefiller (set from TLSInsecureSkipVerify)
	InsecureSkipVerifyForEncoder   bool     // InsecureSkipVerifyForEncoder configures the proxy to skip TLS verification for requests to encoder (set from TLSInsecureSkipVerify)
	InsecureSkipVerifyForDecoder   bool     // InsecureSkipVerifyForDecoder configures the proxy to skip TLS verification for requests to decoder (set from TLSInsecureSkipVerify)
	Configuration                  string   // Configuration is sidecar configuration in YAML provided as inline specification. Example `--configuration={port: 8085, vllm-port: 8203}`
	ConfigurationFile              string   // ConfigurationFile is path to file which contains sidecar configuration in YAML. Example `--configuration-file=/etc/config/sidecar-config.yaml`

	// Deprecated flag fields (kept for backward compatibility)
	PrefillerUseTLS             bool   // Deprecated: Use EnableTLS instead. PrefillerUseTLS indicates whether to use TLS when sending requests to prefillers
	DecoderUseTLS               bool   // Deprecated: Use EnableTLS instead. DecoderUseTLS indicates whether to use TLS when sending requests to the decoder
	PrefillerInsecureSkipVerify bool   // Deprecated: Use TLSInsecureSkipVerify instead. PrefillerInsecureSkipVerify configures the proxy to skip TLS verification for requests to prefiller
	DecoderInsecureSkipVerify   bool   // Deprecated: Use TLSInsecureSkipVerify instead. DecoderInsecureSkipVerify configures the proxy to skip TLS verification for requests to decoder
	SecureProxy                 bool   // SecureProxy enables secure proxy
	CertPath                    string // CertPath is the path to the certificate for secure proxy
	EnableSSRFProtection        bool   // EnableSSRFProtection enables SSRF protection using InferencePool allowlisting
	InferencePool               string // InferencePool in namespace/name or name format (e.g., default/my-pool or my-pool). A single name implies the 'default' namespace.

	// Deprecated flag fields for InferencePool (kept for backward compatibility)
	InferencePoolNamespace          string      // Deprecated: Use InferencePool instead. InferencePoolNamespace is the Kubernetes namespace to watch for InferencePool resources
	InferencePoolName               string      // Deprecated: Use InferencePool instead. InferencePoolName is the specific InferencePool name to watch
	EnablePrefillerSampling         bool        // EnablePrefillerSampling enables random selection of prefill instances
	PoolGroup                       string      // PoolGroup is the group of the InferencePool this Endpoint Picker is associated with
	LoggingOptions                  zap.Options // LoggingOptions holds the zap logging configuration
	FlagSet                         *pflag.FlagSet
	ContainsDefaultValues           *[]string
	ContainsFlags                   *[]string
	ContainsYAMLInlineSpecification *[]string
	ContainsYAMLFile                *[]string
}

type configurationMap map[string]any

const (
	// TLS stages
	prefillStage            = "prefiller"
	decodeStage             = "decoder"
	encodeStage             = "encoder"
	defaultPort             = "8000"
	defaultvLLMPort         = "8001"
	defaultDataParallelSize = 1
)

var (
	// supportedKVConnectors defines all valid P/D KV connector types
	supportedKVConnectors = map[string]struct{}{
		KVConnectorNIXLV2:        {},
		KVConnectorSharedStorage: {},
		KVConnectorSGLang:        {},
	}

	// supportedECConnectors defines all valid E/P EC connector types
	supportedECConnectors = map[string]struct{}{
		ECExampleConnector: {},
	}

	// supportedTLSStages defines all valid stages for TLS configuration
	supportedTLSStages = map[string]struct{}{
		prefillStage: {},
		decodeStage:  {},
		encodeStage:  {},
	}

	supportedKVConnectorNamesStr = strings.Join([]string{KVConnectorNIXLV2, KVConnectorSharedStorage, KVConnectorSGLang}, ", ")
	supportedECConnectorNamesStr = strings.Join([]string{ECExampleConnector}, ", ")
	supportedTLSStageNamesStr    = strings.Join([]string{prefillStage, decodeStage, encodeStage}, ", ")
)

// containsStage checks if a stage is present in the slice
func containsStage(stages []string, stage string) bool {
	for _, s := range stages {
		if s == stage {
			return true
		}
	}
	return false
}

// NewOptions returns a new Options struct initialized with default values.
func NewOptions() *Options {
	// Get default value for EnablePrefillerSampling from environment
	enablePrefillerSampling := false
	if val, err := strconv.ParseBool(os.Getenv("ENABLE_PREFILLER_SAMPLING")); err == nil {
		enablePrefillerSampling = val
	}

	defaultTemp = []string{"port", "vllm-port", "data-parallel-size", "kv-connector", "ec-connector", "connector", "secure-proxy", "inference-pool", "inference-pool-namespace", "inference-pool-name", "enable-prefiller-sampling", "pool-group"}

	return &Options{
		Port:                            defaultPort,
		VLLMPort:                        defaultvLLMPort,
		DataParallelSize:                defaultDataParallelSize,
		KVConnector:                     "",
		ECConnector:                     "",
		Connector:                       KVConnectorNIXLV2,
		SecureProxy:                     true,
		InferencePool:                   os.Getenv("INFERENCE_POOL"),
		InferencePoolNamespace:          os.Getenv("INFERENCE_POOL_NAMESPACE"),
		InferencePoolName:               os.Getenv("INFERENCE_POOL_NAME"),
		EnablePrefillerSampling:         enablePrefillerSampling,
		PoolGroup:                       DefaultPoolGroup,
		ContainsDefaultValues:           &defaultTemp,
		ContainsFlags:                   &fileTemp,
		ContainsYAMLInlineSpecification: &inlineTemp,
		ContainsYAMLFile:                &fileTemp,
	}
}

// AddFlags binds the Options fields to command-line flags on the given FlagSet.
// It also sets up zap logging flags and integrates Go flags with pflag.
func (opts *Options) AddFlags(fs *pflag.FlagSet) {
	// Add logging flags to the standard flag set
	opts.LoggingOptions.BindFlags(flag.CommandLine)

	// Add Go flags to pflag (for zap options compatibility)
	fs.AddGoFlagSet(flag.CommandLine)

	fs.StringVar(&opts.Port, "port", opts.Port, "the port the sidecar is listening on")
	fs.StringVar(&opts.VLLMPort, "vllm-port", opts.VLLMPort, "the port vLLM is listening on")
	fs.IntVar(&opts.DataParallelSize, "data-parallel-size", opts.DataParallelSize, "the vLLM DATA-PARALLEL-SIZE value")

	fs.StringVar(&opts.KVConnector, "kv-connector", opts.KVConnector,
		"the KV protocol between Prefiller and Decoder. Supported: "+supportedKVConnectorNamesStr)

	fs.StringVar(&opts.ECConnector, "ec-connector", opts.ECConnector,
		"the EC protocol between Encoder and Prefiller (for EPD mode). Supported: "+supportedECConnectorNamesStr+". Leave empty to skip encoder stage.")

	fs.StringSliceVar(&opts.EnableTLS, "enable-tls", opts.EnableTLS, "stages to enable TLS for. Supported: "+supportedTLSStageNamesStr+". Can be specified multiple times or as comma-separated values.")
	fs.StringSliceVar(&opts.TLSInsecureSkipVerify, "tls-insecure-skip-verify", opts.TLSInsecureSkipVerify, "stages to skip TLS verification for. Supported: "+supportedTLSStageNamesStr+". Can be specified multiple times or as comma-separated values.")
	fs.StringVar(&opts.Configuration, "configuration", "", "Sidecar configuration in YAML provided as inline specification. Example `--configuration={port: 8085, vllm-port: 8203}`")
	fs.StringVar(&opts.ConfigurationFile, "configuration-file", "", "Path to file which contains sidecar configuration in YAML. Example `--configuration-file=/etc/config/sidecar-config.yaml`")

	// Deprecated flags - kept for backward compatibility
	fs.StringVar(&opts.Connector, "connector", opts.Connector, "Deprecated: use --kv-connector instead. The P/D connector being used. Supported: "+supportedKVConnectorNamesStr)
	_ = fs.MarkDeprecated("connector", "use --kv-connector instead")

	fs.BoolVar(&opts.PrefillerUseTLS, "prefiller-use-tls", opts.PrefillerUseTLS, "Deprecated: use --enable-tls=prefiller instead. Whether to use TLS when sending requests to prefillers.")
	_ = fs.MarkDeprecated("prefiller-use-tls", "use --enable-tls=prefiller instead")
	fs.BoolVar(&opts.DecoderUseTLS, "decoder-use-tls", opts.DecoderUseTLS, "Deprecated: use --enable-tls=decoder instead. Whether to use TLS when sending requests to the decoder.")
	_ = fs.MarkDeprecated("decoder-use-tls", "use --enable-tls=decoder instead")
	fs.BoolVar(&opts.PrefillerInsecureSkipVerify, "prefiller-tls-insecure-skip-verify", opts.PrefillerInsecureSkipVerify, "Deprecated: use --tls-insecure-skip-verify=prefiller instead. Skip TLS verification for requests to prefiller.")
	_ = fs.MarkDeprecated("prefiller-tls-insecure-skip-verify", "use --tls-insecure-skip-verify=prefiller instead")
	fs.BoolVar(&opts.DecoderInsecureSkipVerify, "decoder-tls-insecure-skip-verify", opts.DecoderInsecureSkipVerify, "Deprecated: use --tls-insecure-skip-verify=decoder instead. Skip TLS verification for requests to decoder.")
	_ = fs.MarkDeprecated("decoder-tls-insecure-skip-verify", "use --tls-insecure-skip-verify=decoder instead")
	fs.BoolVar(&opts.SecureProxy, "secure-proxy", opts.SecureProxy, "Enables secure proxy. Defaults to true.")
	fs.StringVar(&opts.CertPath, "cert-path", opts.CertPath, "The path to the certificate for secure proxy. The certificate and private key files are assumed to be named tls.crt and tls.key, respectively. If not set, and secureProxy is enabled, then a self-signed certificate is used (for testing).")
	fs.BoolVar(&opts.EnableSSRFProtection, "enable-ssrf-protection", opts.EnableSSRFProtection, "enable SSRF protection using InferencePool allowlisting")
	fs.StringVar(&opts.InferencePool, "inference-pool", opts.InferencePool, "InferencePool in namespace/name or name format (e.g., default/my-pool or my-pool). A single name implies the 'default' namespace. Can also use INFERENCE_POOL env var.")

	// Deprecated flags - kept for backward compatibility
	fs.StringVar(&opts.InferencePoolNamespace, "inference-pool-namespace", opts.InferencePoolNamespace, "Deprecated: use --inference-pool instead. The Kubernetes namespace for the InferencePool (defaults to INFERENCE_POOL_NAMESPACE env var)")
	_ = fs.MarkDeprecated("inference-pool-namespace", "use --inference-pool instead")
	fs.StringVar(&opts.InferencePoolName, "inference-pool-name", opts.InferencePoolName, "Deprecated: use --inference-pool instead. The specific InferencePool name (defaults to INFERENCE_POOL_NAME env var)")
	_ = fs.MarkDeprecated("inference-pool-name", "use --inference-pool instead")
	fs.BoolVar(&opts.EnablePrefillerSampling, "enable-prefiller-sampling", opts.EnablePrefillerSampling, "if true, the target prefill instance will be selected randomly from among the provided prefill host values")
	fs.StringVar(&opts.PoolGroup, "pool-group", opts.PoolGroup, "group of the InferencePool this Endpoint Picker is associated with.")
}

// validateStages checks if all stages in the slice are valid according to the supportedStages map
func validateStages(stages []string, supportedStages map[string]struct{}, flagName string) error {
	for _, stage := range stages {
		if _, ok := supportedStages[stage]; !ok {
			return fmt.Errorf("%s stages must be one of: %s", flagName, supportedTLSStageNamesStr)
		}
	}
	return nil
}

// Complete performs post-processing of parsed command-line arguments.
// This handles migration from deprecated boolean flags to new StringSlice flags, extracts YAML configuration,
// parses the InferencePool field, sets configuration fields from flag fields, and computes the target URL.
func (opts *Options) Complete() error {
	for i := 0; i < len(*opts.ContainsDefaultValues); i++ {
		v := (*opts.ContainsDefaultValues)[i]
		if !opts.isDefault(v) {
			fmt.Printf(">>>4>>---%#v--\n", v)
			remove(&defaultTemp, v)
			i--
			flagTemp = append(flagTemp, v)
		}
	}
	if err := opts.extractYAMLConfiguration(opts.Configuration, opts.ConfigurationFile); err != nil {
		return err
	}
	fmt.Printf(">>>7>>---%#v--\n", defaultTemp)
	fmt.Printf(">>>8>>---%#v--\n", flagTemp)
	fmt.Printf(">>>9>>---%#v--\n", inlineTemp)
	fmt.Printf(">>>10>>---%#v--\n", fileTemp)

	// Migrate deprecated Connector flag to KVConnector
	if opts.Connector != "" && opts.KVConnector == "" {
		opts.KVConnector = opts.Connector
	}

	// Parse InferencePool field (namespace/name or just name)
	if opts.InferencePool != "" {
		parts := strings.SplitN(opts.InferencePool, "/", 2)
		if len(parts) == 2 {
			// Format: namespace/name
			opts.InferencePoolNamespace = parts[0]
			opts.InferencePoolName = parts[1]
		} else {
			// Format: name (implies default namespace)
			opts.InferencePoolNamespace = "default"
			opts.InferencePoolName = parts[0]
		}
	}

	// Migrate deprecated boolean TLS flags to new StringSlice flags
	if opts.PrefillerUseTLS {
		if !containsStage(opts.EnableTLS, prefillStage) {
			opts.EnableTLS = append(opts.EnableTLS, prefillStage)
		}
	}
	if opts.DecoderUseTLS {
		if !containsStage(opts.EnableTLS, decodeStage) {
			opts.EnableTLS = append(opts.EnableTLS, decodeStage)
		}
	}
	if opts.PrefillerInsecureSkipVerify {
		if !containsStage(opts.TLSInsecureSkipVerify, prefillStage) {
			opts.TLSInsecureSkipVerify = append(opts.TLSInsecureSkipVerify, prefillStage)
		}
	}
	if opts.DecoderInsecureSkipVerify {
		if !containsStage(opts.TLSInsecureSkipVerify, decodeStage) {
			opts.TLSInsecureSkipVerify = append(opts.TLSInsecureSkipVerify, decodeStage)
		}
	}

	// Set configuration fields from flag fields
	opts.UseTLSForPrefiller = containsStage(opts.EnableTLS, prefillStage)
	opts.UseTLSForEncoder = containsStage(opts.EnableTLS, encodeStage)
	opts.UseTLSForDecoder = containsStage(opts.EnableTLS, decodeStage)
	opts.InsecureSkipVerifyForPrefiller = containsStage(opts.TLSInsecureSkipVerify, prefillStage)
	opts.InsecureSkipVerifyForEncoder = containsStage(opts.TLSInsecureSkipVerify, encodeStage)
	opts.InsecureSkipVerifyForDecoder = containsStage(opts.TLSInsecureSkipVerify, decodeStage)

	// Compute target URL based on decoder TLS settings and VLLM port
	scheme := "http"
	if opts.UseTLSForDecoder {
		scheme = schemeHTTPS
	}
	opts.TargetURL = scheme + "://localhost:" + opts.VLLMPort

	return nil
}

// Validate checks the Options for invalid or conflicting values.
func (opts *Options) Validate() error {
	// Validate KV connector
	if _, ok := supportedKVConnectors[opts.KVConnector]; !ok {
		return fmt.Errorf("--kv-connector must be one of: %s", supportedKVConnectorNamesStr)
	}

	// Validate EC connector if provided
	if opts.ECConnector != "" {
		if _, ok := supportedECConnectors[opts.ECConnector]; !ok {
			return fmt.Errorf("--ec-connector must be one of: %s", supportedECConnectorNamesStr)
		}
	}

	// Validate deprecated connector flag
	if opts.Connector != "" && opts.Connector != opts.KVConnector {
		if _, ok := supportedKVConnectors[opts.Connector]; !ok {
			return fmt.Errorf("--connector must be one of: %s", supportedKVConnectorNamesStr)
		}
	}

	// Validate TLS stages
	if err := validateStages(opts.EnableTLS, supportedTLSStages, "--enable-tls"); err != nil {
		return err
	}

	if err := validateStages(opts.TLSInsecureSkipVerify, supportedTLSStages, "--tls-insecure-skip-verify"); err != nil {
		return err
	}

	// Validate InferencePool format if provided
	if opts.InferencePool != "" {
		// Check for invalid characters (only allow alphanumeric, hyphen, and forward slash)
		if strings.Count(opts.InferencePool, "/") > 1 {
			return errors.New("--inference-pool must be in format 'namespace/name' or 'name', not multiple slashes")
		}
		// Validate that it doesn't contain invalid characters like spaces or special chars
		parts := strings.Split(opts.InferencePool, "/")
		for _, part := range parts {
			if part == "" {
				return errors.New("--inference-pool cannot have empty namespace or name")
			}
		}
	}

	// Validate SSRF protection requirements
	if opts.EnableSSRFProtection {
		if opts.InferencePoolNamespace == "" {
			return errors.New("--inference-pool, --inference-pool-namespace, INFERENCE_POOL, or INFERENCE_POOL_NAMESPACE environment variable is required when --enable-ssrf-protection is true")
		}
		if opts.InferencePoolName == "" {
			return errors.New("--inference-pool, --inference-pool-name, INFERENCE_POOL, or INFERENCE_POOL_NAME environment variable is required when --enable-ssrf-protection is true")
		}
	}

	return nil
}

func removeDuplicates(fileTemp *[]string, inlineTemp []string) {
	inlineMap := make(map[string]bool)
	for _, item := range inlineTemp {
		inlineMap[item] = true
	}
	filtered := (*fileTemp)[:0]
	for _, item := range *fileTemp {
		if !inlineMap[item] {
			filtered = append(filtered, item)
		}
	}
	*fileTemp = filtered
}

// extractYAMLConfiguration extracts sidecar configuration (if provided)
// from `--configuration` and `--configuration-file` parameters
func (opts *Options) extractYAMLConfiguration(configuration string, configurationFile string) error {
	var configurationMap1, configurationMap2 configurationMap
	var err error
	if configuration != "" {
		configurationMap1, err = YAMLConfigurationFromInlineSpecification(configuration)
		if err != nil {
			return err
		}
	}
	if configurationFile != "" {
		configurationMap2, err = YAMLConfigurationFromFile(configurationFile)
		if err != nil {
			return err
		}
	}

	switch {

	// No inline specification and no file data
	case configurationMap1 != nil && configurationMap2 != nil:
		if len(configurationMap1) != 0 && len(configurationMap2) != 0 {
			err = opts.updateSidecarConfiguration(mergeYAMLConfigurations(configurationMap2, configurationMap1))
			if err != nil {
				return err
			}
			for key := range configurationMap1 {
				inlineTemp = append(inlineTemp, key)
			}
			for key := range configurationMap2 {
				fileTemp = append(fileTemp, key)
			}

			// compare inline and file
			// remove duplicates from file
			// fmt.Println("Original fileTemp:", fileTemp)
			// removeDuplicates(&fileTemp, &inlineTemp)
			// fmt.Println("New fileTemp:", fileTemp)

			// fmt.Println("Before inline:", inlineTemp)
			// fmt.Println("Before filetemp:", fileTemp)
			// removeDuplicates(&fileTemp, inlineTemp)
			// fmt.Println("After filetemp:", fileTemp)

			// compare flag and inline
			// remove duplicates from inline
			// fmt.Println("Before flagTemp:", flagTemp)
			// fmt.Println("Before inlineTemp:", inlineTemp)
			// removeDuplicates(&inlineTemp, flagTemp)
			// fmt.Println("After inlineTemp:", inlineTemp)

			// fmt.Println("Original inlineTemp:", inlineTemp)
			// fmt.Println("flagTemp:", flagTemp)
			// fmt.Println("Result:", result2)

			fmt.Printf("AAAAAAAAAAAAAAAAAAAA-101-DEFAULT-%#v\n", defaultTemp)
			fmt.Printf("AAAAAAAAAAAAAAAAAAAA-102-FLAG-%#v\n", flagTemp)
			fmt.Printf("AAAAAAAAAAAAAAAAAAAA-103-INLINE-%#v\n", inlineTemp)
			fmt.Printf("AAAAAAAAAAAAAAAAAAAA-104-FILE-%#v\n", fileTemp)
		}

	// Only inline specification has data
	// No file data
	case configurationMap1 != nil && configurationMap2 == nil:
		if len(configurationMap1) != 0 {
			err = opts.updateSidecarConfiguration(configurationMap1)
			if err != nil {
				return err
			}
			// opts.ContainsYAMLInlineSpecification = true
			// opts.ContainsYAMLFile = false
			for key := range configurationMap1 {
				inlineTemp = append(inlineTemp, key)
			}

			// compare flag and inline
			// remove duplicates from inline
			// fmt.Println("Before flagTemp:", flagTemp)
			// fmt.Println("Before inlineTemp:", inlineTemp)
			// removeDuplicates(&inlineTemp, flagTemp)
			// fmt.Println("After inlineTemp:", inlineTemp)

			fmt.Printf("AAAAAAAAAAAAAAAAAAAA-201-DEFAULT-%#v\n", defaultTemp)
			fmt.Printf("AAAAAAAAAAAAAAAAAAAA-202-FLAG-%#v\n", flagTemp)
			fmt.Printf("AAAAAAAAAAAAAAAAAAAA-203-INLINE-%#v\n", inlineTemp)
			fmt.Printf("AAAAAAAAAAAAAAAAAAAA-204-FILE-%#v\n", fileTemp)
		}

	// No inline specification data
	// Only file data
	case configurationMap1 == nil && configurationMap2 != nil:
		if len(configurationMap2) != 0 {
			err = opts.updateSidecarConfiguration(configurationMap2)
			if err != nil {
				return err
			}
			for key := range configurationMap2 {
				fileTemp = append(fileTemp, key)
			}
			// opts.ContainsYAMLInlineSpecification = false
			// opts.ContainsYAMLFile = true
			fmt.Printf("AAAAAAAAAAAAAAAAAAAA-301-DEFAULT-%#v\n", defaultTemp)
			fmt.Printf("AAAAAAAAAAAAAAAAAAAA-302-FLAG-%#v\n", flagTemp)
			fmt.Printf("AAAAAAAAAAAAAAAAAAAA-303-INLINE-%#v\n", inlineTemp)
			fmt.Printf("AAAAAAAAAAAAAAAAAAAA-304-FILE-%#v\n", fileTemp)

			// remove duplicates from fileTemp
			// compare file and flag
			// fmt.Println("Before flagTemp:", flagTemp)
			// fmt.Println("Before fileTemp:", fileTemp)
			// removeDuplicates(&inlineTemp, fileTemp)
			// fmt.Println("After fileTemp:", inlineTemp)
		}

	// No inline specification and no file data
	default:
		// opts.ContainsYAMLInlineSpecification = false
		// opts.ContainsYAMLFile = false
	}
	return nil
}

// isDefault checks if flag contains default or parsed value
func (o *Options) isDefault(parameter string) bool {
	flag := o.FlagSet.Lookup(parameter)
	result := flag == nil || !flag.Changed
	if result {
		// o.ContainsDefaultValues = true
	}
	return result
}

// YAMLConfigurationFromInlineSpecification extracts YAML configuration provided as inline specification
// "--configuration={port: 8085, vllm-port: 8203}"
func YAMLConfigurationFromInlineSpecification(config string) (map[string]any, error) {
	var temp map[string]any
	if err := yaml.Unmarshal([]byte(config), &temp); err != nil {
		return nil, errors.New("Failed to unmarshal sidecar configuration")
	}
	return temp, nil

}

// YAMLConfigurationFromFile extracts YAML configuration from file path
// "--configuration-file=/etc/config/sidecar-config.yaml"
func YAMLConfigurationFromFile(configFile string) (map[string]any, error) {
	var temp map[string]any
	rawFile, err := os.ReadFile(configFile)
	if err != nil {
		return nil, errors.New("Failed to read sidecar configuration")
	}
	if err := yaml.Unmarshal(rawFile, &temp); err != nil {
		return nil, errors.New("Failed to unmarshal sidecar configuration")

	}
	return temp, nil
}

// mergeYAMLConfigurations merges following:
// 1. YAML configuration from file path `--configuration-file`
// 2. YAML configuration provided as inline specification `--configuration“,
// and gives higher priority to configuration provided in inline specification `--configuration`
func mergeYAMLConfigurations(fileYAML, parameterYAML map[string]any) map[string]any {
	for fileKey := range fileYAML {
		fileTemp = append(fileTemp, fileKey)
	}
	for parameterKey, parameterValue := range parameterYAML {
		inlineTemp = append(inlineTemp, parameterKey)
		fmt.Printf("AAAAAAAAAAAAAAAAAAAA-700-%#v\n", inlineTemp)
		if fileYAMLValue, ok := fileYAML[parameterKey]; ok {
			fileYAMLMap, fileYAMLOk := fileYAMLValue.(map[string]any)
			parameterYAMLMap, parameterYAMLOk := parameterValue.(map[string]any)
			if fileYAMLOk && parameterYAMLOk {
				fileYAML[parameterKey] = mergeYAMLConfigurations(fileYAMLMap, parameterYAMLMap)
				continue
			}
		}
		fileYAML[parameterKey] = parameterValue
	}
	removeDuplicates(&fileTemp, inlineTemp)
	fmt.Printf("\n\n\nAAAAAAAAAAAAAAAAAAAA-11-DEFAULT-%#v\n", defaultTemp)
	fmt.Printf("AAAAAAAAAAAAAAAAAAAA-22-FLAG-%#v\n", flagTemp)
	fmt.Printf("AAAAAAAAAAAAAAAAAAAA-33-INLINE-%#v\n", inlineTemp)
	fmt.Printf("AAAAAAAAAAAAAAAAAAAA-44-FILE-%#v\n\n\n", fileTemp)
	return fileYAML
}

// remove() removes all occurrences in-place (more memory efficient)
func remove(slice *[]string, value string) {
	filtered := (*slice)[:0]
	for _, v := range *slice {
		if v != value {
			filtered = append(filtered, v)
		}
	}

	fmt.Printf("++++++++++++++++++++++++++++++++++++----%#v", filtered)
	*slice = filtered
}

func appendKey(slice *[]string, element string) {
	if !slices.Contains(*slice, element) {
		*slice = append(*slice, element)
	}
}

// updateSidecarConfiguration updates value from YAML only when:
// 1. YAML configuration contains non-zero value
// 2. sidecar configuration contains value not explicitely set by flag
// i.e. gives higher priority to configuration provided individually through flags (e.g. `--port`, `--vllm-port`) over configuration provided through YAML
func (opts *Options) updateSidecarConfiguration(configurationMap configurationMap) error {
	if configurationMap["port"] != nil {
		if v, ok := configurationMap["port"].(float64); ok {
			if opts.isDefault("port") {
				opts.Port = strconv.Itoa(int(v))
				remove(&defaultTemp, "port")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-1001-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-1002-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-1003-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-1004-%#v\n", fileTemp)
			} else {
				remove(&defaultTemp, "port")
				appendKey(&flagTemp, "port")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-1005-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-1006-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-1007-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-1008-%#v\n", fileTemp)
			}
		} else {
			return errors.New("Type assertion failed for port: " + fmt.Sprintf("%v", configurationMap["port"]))
		}
	}
	if configurationMap["vllm-port"] != nil {
		if v, ok := configurationMap["vllm-port"].(float64); ok {
			if opts.isDefault("vllm-port") {
				opts.VLLMPort = strconv.Itoa(int(v))
				remove(&defaultTemp, "vllm-port")
				//remove(fileTemp, "vllm-port")
				//remove(inlineTemp, "vllm-port")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-1009-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-1010-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-1011-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-1012-%#v\n", fileTemp)
			} else {
				remove(&defaultTemp, "vllm-port")
				appendKey(&flagTemp, "vllm-port")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-1013-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-1014-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-1015-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-1016-%#v\n", fileTemp)
			}
		} else {
			return errors.New("Type assertion failed for vllm-port: " + fmt.Sprintf("%v", configurationMap["vllm-port"]))
		}
	}
	if configurationMap["data-parallel-size"] != nil {
		if v, ok := configurationMap["data-parallel-size"].(float64); ok {
			if opts.isDefault("data-parallel-size") {
				opts.DataParallelSize = int(v)
				remove(&defaultTemp, "data-parallel-size")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-1017-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-1018-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-1019-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-1020-%#v\n", fileTemp)
			} else {
				remove(&defaultTemp, "data-parallel-size")
				appendKey(&flagTemp, "data-parallel-size")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-1021-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-1022-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-1023-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-1024-%#v\n", fileTemp)
			}
		} else {
			return errors.New("Type assertion failed for data-parallel-size: " + fmt.Sprintf("%v", configurationMap["data-parallel-size"]))
		}
	}
	if configurationMap["connector"] != nil {
		if v, ok := configurationMap["connector"].(string); ok {
			if opts.isDefault("connector") {
				opts.Connector = v
				remove(&defaultTemp, "connector")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-1025-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-1026-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-1027-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-1028-%#v\n", fileTemp)
			} else {
				remove(&defaultTemp, "connector")
				appendKey(&flagTemp, "connector")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-1029-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-1030-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-1031-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-1032-%#v\n", fileTemp)
			}
		} else {
			return errors.New("Type assertion failed for connector: " + fmt.Sprintf("%v", configurationMap["connector"]))
		}
	}
	if configurationMap["kv-connector"] != nil {
		if v, ok := configurationMap["kv-connector"].(string); ok {
			if opts.isDefault("kv-connector") {
				opts.KVConnector = v
				remove(&defaultTemp, "kv-connector")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-1033-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-1034-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-1035-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-1036-%#v\n", fileTemp)
			} else {
				remove(&defaultTemp, "kv-connector")
				appendKey(&flagTemp, "kv-connector")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-1037-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-1038-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-1039-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-1040-%#v\n", fileTemp)
			}
		} else {
			return errors.New("Type assertion failed for kv-connector: " + fmt.Sprintf("%v", configurationMap["kv-connector"]))
		}
	}
	if configurationMap["ec-connector"] != nil {
		if v, ok := configurationMap["ec-connector"].(string); ok {
			if opts.isDefault("ec-connector") {
				opts.ECConnector = v
				remove(&defaultTemp, "ec-connector")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-1041-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-1042-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-1043-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-1044-%#v\n", fileTemp)
			} else {
				remove(&defaultTemp, "ec-connector")
				appendKey(&flagTemp, "ec-connector")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-1045-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-1046-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-1047-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-1048-%#v\n", fileTemp)
			}
		} else {
			return errors.New("Type assertion failed for ec-connector: " + fmt.Sprintf("%v", configurationMap["ec-connector"]))
		}
	}
	if configurationMap["enable-tls"] != nil {
		switch v := configurationMap["enable-tls"].(type) {
		case string:
			opts.EnableTLS = append(opts.EnableTLS, strings.Split(v, ",")...)
		case []any:
			for _, val := range v {
				opts.EnableTLS = append(opts.EnableTLS, fmt.Sprintf("%v", val))
			}
		default:
			return errors.New("Type assertion failed for enable-tls: " + fmt.Sprintf("%v", configurationMap["enable-tls"]))
		}
	}
	if configurationMap["prefiller-use-tls"] != nil {
		if v, ok := configurationMap["prefiller-use-tls"].(bool); ok {
			if opts.isDefault("prefiller-use-tls") {
				opts.PrefillerUseTLS = v
				remove(&defaultTemp, "prefiller-use-tls")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-1049-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-1050-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-1051-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-1052-%#v\n", fileTemp)
			} else {
				remove(&defaultTemp, "prefiller-use-tls")
				appendKey(&flagTemp, "prefiller-use-tls")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-1053-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-1054-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-1055-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-1056-%#v\n", fileTemp)
			}
		} else {
			return errors.New("Type assertion failed for prefiller-use-tls: " + fmt.Sprintf("%v", configurationMap["prefiller-use-tls"]))
		}
	}
	if configurationMap["decoder-use-tls"] != nil {
		if v, ok := configurationMap["decoder-use-tls"].(bool); ok {
			if opts.isDefault("decoder-use-tls") {
				opts.DecoderUseTLS = v
				remove(&defaultTemp, "decoder-use-tls")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-1057-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-1058-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-1059-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-1060-%#v\n", fileTemp)
			} else {
				remove(&defaultTemp, "decoder-use-tls")
				appendKey(&flagTemp, "decoder-use-tls")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-1061-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-1062-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-1063-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-1064-%#v\n", fileTemp)
			}
		} else {
			return errors.New("Type assertion failed for decoder-use-tls: " + fmt.Sprintf("%v", configurationMap["decoder-use-tls"]))
		}
	}
	if configurationMap["tls-insecure-skip-verify"] != nil {
		if v, ok := configurationMap["tls-insecure-skip-verify"].(bool); ok {
			if opts.isDefault("tls-insecure-skip-verify") {
				opts.PrefillerInsecureSkipVerify = v
				remove(&defaultTemp, "tls-insecure-skip-verify")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-1065-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-1066-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-1067-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-1068-%#v\n", fileTemp)
			} else {
				remove(&defaultTemp, "tls-insecure-skip-verify")
				appendKey(&flagTemp, "tls-insecure-skip-verify")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-1069-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-1070-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-1071-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-1072-%#v\n", fileTemp)
			}
		} else {
			return errors.New("Type assertion failed for tls-insecure-skip-verify: " + fmt.Sprintf("%v", configurationMap["tls-insecure-skip-verify"]))
		}
	}
	if configurationMap["prefiller-tls-insecure-skip-verify"] != nil {
		if v, ok := configurationMap["prefiller-tls-insecure-skip-verify"].(bool); ok {
			if opts.isDefault("prefiller-tls-insecure-skip-verify") {
				opts.PrefillerInsecureSkipVerify = v
				remove(&defaultTemp, "prefiller-tls-insecure-skip-verify")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-1073-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-1074-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-1075-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-1076-%#v\n", fileTemp)
			} else {
				remove(&defaultTemp, "prefiller-tls-insecure-skip-verify")
				appendKey(&flagTemp, "prefiller-tls-insecure-skip-verify")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-1077-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-1078-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-1079-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-1080-%#v\n", fileTemp)
			}
		} else {
			return errors.New("Type assertion failed for prefiller-tls-insecure-skip-verify: " + fmt.Sprintf("%v", configurationMap["prefiller-tls-insecure-skip-verify"]))
		}
	}
	if configurationMap["decoder-tls-insecure-skip-verify"] != nil {
		if v, ok := configurationMap["decoder-tls-insecure-skip-verify"].(bool); ok {
			if opts.isDefault("decoder-tls-insecure-skip-verify") {
				opts.DecoderInsecureSkipVerify = v
				remove(&defaultTemp, "decoder-tls-insecure-skip-verify")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-1081-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-1082-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-1083-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-1084-%#v\n", fileTemp)
			} else {
				remove(&defaultTemp, "decoder-tls-insecure-skip-verify")
				appendKey(&flagTemp, "decoder-tls-insecure-skip-verify")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-1085-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-1086-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-1087-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-1088-%#v\n", fileTemp)
			}
		} else {
			return errors.New("Type assertion failed for decoder-tls-insecure-skip-verify: " + fmt.Sprintf("%v", configurationMap["decoder-tls-insecure-skip-verify"]))
		}
	}
	if configurationMap["secure-proxy"] != nil {
		if v, ok := configurationMap["secure-proxy"].(bool); ok {
			if opts.isDefault("secure-proxy") {
				opts.SecureProxy = v
				remove(&defaultTemp, "secure-proxy")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-1089-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-1090-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-1091-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-1092-%#v\n", fileTemp)
			} else {
				remove(&defaultTemp, "secure-proxy")
				appendKey(&flagTemp, "secure-proxy")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-1093-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-1094-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-1095-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-1096-%#v\n", fileTemp)
			}
		} else {
			return errors.New("Type assertion failed for secure-proxy: " + fmt.Sprintf("%v", configurationMap["secure-proxy"]))
		}
	}
	if configurationMap["cert-path"] != nil {
		if v, ok := configurationMap["cert-path"].(string); ok {
			if opts.isDefault("cert-path") {
				opts.CertPath = v
				remove(&defaultTemp, "cert-path")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-5-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-6-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-7-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-8-%#v\n", fileTemp)
			} else {
				remove(&defaultTemp, "cert-path")
				appendKey(&flagTemp, "cert-path")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-17-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-18-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-19-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-20-%#v\n", fileTemp)
			}
		} else {
			return errors.New("Type assertion failed for cert-path: " + fmt.Sprintf("%v", configurationMap["cert-path"]))
		}
	}
	if configurationMap["enable-ssrf-protection"] != nil {
		if v, ok := configurationMap["enable-ssrf-protection"].(bool); ok {
			if opts.isDefault("enable-ssrf-protection") {
				opts.EnableSSRFProtection = v
				remove(&defaultTemp, "enable-ssrf-protection")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-5-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-6-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-7-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-8-%#v\n", fileTemp)
			} else {
				remove(&defaultTemp, "enable-ssrf-protection")
				appendKey(&flagTemp, "enable-ssrf-protection")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-17-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-18-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-19-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-20-%#v\n", fileTemp)
			}
		} else {
			return errors.New("Type assertion failed for enable-ssrf-protection: " + fmt.Sprintf("%v", configurationMap["enable-ssrf-protection"]))
		}
	}
	if configurationMap["inference-pool"] != nil {
		if v, ok := configurationMap["inference-pool"].(string); ok {
			if opts.isDefault("inference-pool") {
				opts.InferencePool = v
				remove(&defaultTemp, "inference-pool")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-5-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-6-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-7-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-8-%#v\n", fileTemp)
			} else {
				remove(&defaultTemp, "inference-pool")
				appendKey(&flagTemp, "inference-pool")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-17-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-18-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-19-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-20-%#v\n", fileTemp)
			}
		} else {
			return errors.New("Type assertion failed for inference-pool: " + fmt.Sprintf("%v", configurationMap["inference-pool"]))
		}
	}
	if configurationMap["inference-pool-namespace"] != nil {
		if v, ok := configurationMap["inference-pool-namespace"].(string); ok {
			if opts.isDefault("inference-pool-namespace") {
				opts.InferencePoolNamespace = v
				remove(&defaultTemp, "inference-pool-namespace")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-5-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-6-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-7-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-8-%#v\n", fileTemp)
			} else {
				remove(&defaultTemp, "inference-pool-namespace")
				appendKey(&flagTemp, "inference-pool-namespace")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-17-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-18-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-19-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-20-%#v\n", fileTemp)
			}
		} else {
			return errors.New("Type assertion failed for inference-pool-namespace: " + fmt.Sprintf("%v", configurationMap["inference-pool-namespace"]))
		}
	}
	if configurationMap["inference-pool-name"] != nil {
		if v, ok := configurationMap["inference-pool-name"].(string); ok {
			if opts.isDefault("inference-pool-name") {
				opts.InferencePoolName = v
				remove(&defaultTemp, "inference-pool-name")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-5-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-6-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-7-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-8-%#v\n", fileTemp)
			} else {
				remove(&defaultTemp, "inference-pool-name")
				appendKey(&flagTemp, "inference-pool-name")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-17-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-18-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-19-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-20-%#v\n", fileTemp)
			}
		} else {
			return errors.New("Type assertion failed for inference-pool-name: " + fmt.Sprintf("%v", configurationMap["inference-pool-name"]))
		}
	}
	if configurationMap["enable-prefiller-sampling"] != nil {
		if v, ok := configurationMap["enable-prefiller-sampling"].(bool); ok {
			if opts.isDefault("enable-prefiller-sampling") {
				opts.EnablePrefillerSampling = v
				remove(&defaultTemp, "enable-prefiller-sampling")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-5-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-6-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-7-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-8-%#v\n", fileTemp)
			} else {
				remove(&defaultTemp, "enable-prefiller-sampling")
				appendKey(&flagTemp, "enable-prefiller-sampling")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-17-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-18-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-19-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-20-%#v\n", fileTemp)
			}
		} else {
			return errors.New("Type assertion failed for enable-prefiller-sampling: " + fmt.Sprintf("%v", configurationMap["enable-prefiller-sampling"]))
		}
	}
	if configurationMap["pool-group"] != nil {
		if v, ok := configurationMap["pool-group"].(string); ok {
			if opts.isDefault("pool-group") {
				opts.PoolGroup = v
				remove(&defaultTemp, "pool-group")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-5-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-6-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-7-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-8-%#v\n", fileTemp)
			} else {
				remove(&defaultTemp, "pool-group")
				appendKey(&flagTemp, "pool-group")
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-DEFAULT-17-%#v\n", defaultTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FLAG-18-%#v\n", flagTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-INLINE-19-%#v\n", inlineTemp)
				fmt.Printf("AAAAAAAAAAAAAAAAAAAA-FILE-20-%#v\n", fileTemp)
			}
		} else {
			return errors.New("Type assertion failed for pool-group: " + fmt.Sprintf("%v", configurationMap["pool-group"]))
		}
	}
	return nil
}
