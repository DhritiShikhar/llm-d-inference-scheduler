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

package proxy

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
)

var validSidecarfilePath, invalidSidecarfilePath string

// createSidecarConfigurationWithValidYAML creates sidecar configuration file with valid YAML
func createSidecarConfigurationWithValidYAML(t *testing.T) error {
	tempDir := t.TempDir()

	// This shows that YAML from file overrides default values
	validYAML := []byte(`
port: 8100
vllm-port: 8200
data-parallel-size: 4
kv-connector: "sglang"
ec-connector: "ec-example"
enable-ssrf-protection: true
enable-prefiller-sampling: true
enable-tls: 
- prefiller
- decoder
prefiller-use-tls: false
tls-insecure-skip-verify:
- prefiller
decoder-tls-insecure-skip-verify: true
secure-proxy: false
cert-path: "/etc/certificates3"
inference-pool: "namespace3/inferencepool3"
pool-group: "poolgroup3"
`)

	validSidecarfilePath = filepath.Join(tempDir, "sidecar-configuration-valid.yaml")
	err := os.WriteFile(validSidecarfilePath, validYAML, 0644)
	if err != nil {
		return err
	}
	return nil
}

// createSidecarConfigurationWithInvalidYAML creates sidecar configuration files with invalid YAML
func createSidecarConfigurationWithInvalidYAML(t *testing.T) error {
	tempDir := t.TempDir()

	invalidYAML := []byte(`
port: 8100
vllm-port: 8200
*&&&&&&#######!!
`)

	invalidSidecarfilePath = filepath.Join(tempDir, "sidecar-configuration-invalid.yaml")
	err := os.WriteFile(invalidSidecarfilePath, invalidYAML, 0644)
	if err != nil {
		return err
	}
	return nil
}

// newTestOptions creates:
// 1. new flag set for test
// 2. new Options struct initialized with default values
func newTestOptions(t *testing.T) (*Options, *pflag.FlagSet) {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	testFlagSet := pflag.NewFlagSet(t.Name(), pflag.ContinueOnError)
	opts := NewOptions()
	opts.AddFlags(testFlagSet)
	opts.FlagSet = testFlagSet
	return opts, testFlagSet
}

// TestSidecarConfiguration tests following cases:
// Case 1: No sidecar configuration provided by user i.e. default configuration is used.
// Case 2: Configuration provided individually through flags (e.g `--port`, `--vllm-port`). Configuration from flags over-ride default configuration.
// Case 3: Configuration provided through environment variables: `INFERENCE_POOL`, `INFERENCE_POOL_NAMESPACE` and `INFERENCE_POOL_NAME`. `INFERENCE_POOL` has higher priority over `INFERENCE_POOL_NAMESPACE and `INFERENCE_POOL_NAME`. Environment variables over-ride default values.
// Case 4: YAML configuration provided through inline specification `--configuration`. Configuration from YAML inline specification over-ride default configuration.
// Case 5: YAML configuration provided through file `--configuration-file`. Configuration from YAML file over-ride default configuration.
// Case 6: Case 2 + Case 3: Configuration provided through individual flags + environment variables. Configuration from individual flags over-ride configuration from environment variable.
// Case 7:
// Case 8: Case 2 + Case 4: Configuration provided through individual flags + YAML inline specification. Configuration from individual flags over-ride configuration from inline specification.
// Case 9: Case 2 + Case 5: Configuration provided through individual flags + file. Configuration from individual flags over-ride configuration from file.
// Case 10: Case 3 + Case 4: Configuration provided through environment variable + YAML inline specification. Configuration from environment variable over-rides configuration from inline specification.
// Case 11: Case 3 + case 4: Configuration provided through environment variable + file. Configuration from environment variable over-rides configuration from inline specification.
// Case 12: Case 4 + Case 5: Configuration provided through YAML inline specification + file. YAML inline specification over-rides values from file.
// Case 13: Case 2 + Case 4 + Case 5: Configuration provided through individual flags + YAML inline specification + file. Individual flags have highest priority, then inline specification, then file.
// Case 14: Case 3 + Case 4 + Case 5 i.e. configuration provided through environment variable + YAML inline specification + file. Configuration from environment variable over-rides configuration from inline specification. Configuration from inline specification over-rides configuration from file.
// Case 15: Case 2 + Case 3 + Case 4 + Case 5 i.e. configuration provided through individual flags + environment variable + YAML inline specification + file. Configuration from Individual flags over-ride configuration from everything else.
// Case 16: Invalid YAML configuration provided through inline specification `--configuration`. Error is expected.
// Case 17: Invalid YAML configuration provided through file `--configuration`. Error is expected.

func TestSidecarConfiguration(t *testing.T) {
	kvConnectorNIXLV2 := fmt.Sprintf("%v", KVConnectorNIXLV2)
	kvConnectorSGLang := fmt.Sprintf("%v", KVConnectorSGLang)
	ecConnectorValue := fmt.Sprintf("%v", ECExampleConnector)

	// 	YAMLInlineSpecificationConfiguration1 to check that inline specification overrides default values
	YAMLInlineSpecificationConfiguration1 :=
		`{
		port: 8011,
		vllm-port: 8021,
		data-parallel-size: 3,
		kv-connector: sglang,
		ec-connector: ec-example,
		enable-ssrf-protection: true,
		enable-prefiller-sampling: true,
		enable-tls: ['prefiller', 'decoder'],
		prefiller-use-tls: false,
		tls-insecure-skip-verify: ['decoder'],
		prefiller-tls-insecure-skip-verify: true,
		secure-proxy: false,
		cert-path: '/etc/certificatesForInline',
		inference-pool: inlineNamespace1/inlineInferencepool1,
		pool-group: inlinePoolgroup1
	}`

	// YAMLInlineSpecificationConfiguration2 to check that inline specification does not override values from individual flags like `--kv-connector: sglang`
	YAMLInlineSpecificationConfiguration2 :=
		`{
		port: 8101,
		vllm-port: 8201,
		data-parallel-size: 3,
		kv-connector: nixlv2,
		enable-ssrf-protection: false,
		enable-prefiller-sampling: false,
		enable-tls: ['prefiller', 'decoder'],
		prefiller-use-tls: false,
		tls-insecure-skip-verify: ['decoder'],
		prefiller-tls-insecure-skip-verify: true,
		secure-proxy: false,
		cert-path: '/certificates',
		inference-pool: inlineNamespace2/inlineInferencepool2,
		pool-group: inlinePoolgroup2
	}`

	invalidConfiguration := "{port: 8200, vllm-port: 'sh'"
	expectedError := errors.New("Failed to unmarshal sidecar configuration")

	err := createSidecarConfigurationWithValidYAML(t)
	if err != nil {
		t.Fatalf("failed to write sidecar configuration file: %v", err)
	}
	err = createSidecarConfigurationWithInvalidYAML(t)
	if err != nil {
		t.Fatalf("failed to write sidecar configuration file: %v", err)
	}

	tests := []struct {
		name                                         string
		expectedPort                                 string
		expectedVLLMPort                             string
		expectedDataParallelSize                     int
		expectedKVConnector                          string
		expectedECConnector                          string
		expectedSecureServing                        bool
		expectedEnableSSRFProtection                 bool
		expectedEnablePrefillerSampling              bool
		expectedEnableTLS                            []string
		expectedUseTLSForPrefiller                   bool
		expectedUseTLSForDecoder                     bool
		expectedUseTLSForEncoder                     bool
		expectedTLSInsecureSkipVerify                []string
		expectedPrefillerInsecureSkipVerify          bool
		expectedDecoderInsecureSkipVerify            bool
		expectedEncoderInsecureSkipVerify            bool
		expectedCertPath                             string
		expectedInferencePool                        string
		expectedInferencePoolNamespace               string
		expectedInferencePoolName                    string
		expectedPoolGroup                            string
		expectedConfigurationFromInlineSpecification string
		expectedConfigurationFromFile                string
		expectedConfigurationState                   struct {
			FromEnv    []string
			FromFlags  []string
			FromInline []string
			FromFile   []string
			Defaults   []string
		}
		expectedError error
		inputFlags    map[string]any
		inputEnvVar   map[string]string
	}{
		{
			name:                                         "Case 1: No sidecar configuration provided by user i.e. default configuration is used.",
			inputFlags:                                   nil,
			expectedPort:                                 defaultPort,
			expectedVLLMPort:                             defaultvLLMPort,
			expectedDataParallelSize:                     defaultDataParallelSize,
			expectedKVConnector:                          KVConnectorNIXLV2,
			expectedECConnector:                          "",
			expectedEnableSSRFProtection:                 false,
			expectedEnablePrefillerSampling:              false,
			expectedEnableTLS:                            nil,
			expectedUseTLSForPrefiller:                   false,
			expectedUseTLSForDecoder:                     false,
			expectedUseTLSForEncoder:                     false,
			expectedTLSInsecureSkipVerify:                nil,
			expectedPrefillerInsecureSkipVerify:          false,
			expectedDecoderInsecureSkipVerify:            false,
			expectedEncoderInsecureSkipVerify:            false,
			expectedSecureServing:                        true,
			expectedCertPath:                             "",
			expectedInferencePool:                        "",
			expectedPoolGroup:                            DefaultPoolGroup,
			expectedConfigurationFromInlineSpecification: "",
			expectedConfigurationFromFile:                "",
			expectedConfigurationState: struct {
				FromEnv    []string
				FromFlags  []string
				FromInline []string
				FromFile   []string
				Defaults   []string
			}{
				Defaults: []string{
					port,
					vllmPort,
					dataParallelSize,
					kvConnector,
					ecConnector,
					enableSSRFProtection,
					enablePrefillerSampling,
					enableTLS,
					TLSInsecureSkipVerify,
					SecureServing,
					certPath,
					poolGroup},
			},
			expectedError: nil,
		},
		{
			name: "Case 2: Configuration provided individually through flags (e.g `--port`, `--vllm-port`). Configuration from flags over-ride default configuration.",
			inputFlags: map[string]any{
				port:                    "8001",
				vllmPort:                "8002",
				dataParallelSize:        2,
				kvConnector:             kvConnectorSGLang,
				ecConnector:             ecConnectorValue,
				enableSSRFProtection:    true,
				enablePrefillerSampling: true,
				enableTLS:               &[]string{prefillStage},
				TLSInsecureSkipVerify:   &[]string{prefillStage},
				SecureServing:           false,
				certPath:                "/etc/certificatesForCase2",
				inferencePool:           "namespaceForCase2/inferencepoolForCase2",
				poolGroup:               "poolgroupForCase2",
			},
			expectedPort:                                 "8001",
			expectedVLLMPort:                             "8002",
			expectedDataParallelSize:                     2,
			expectedKVConnector:                          KVConnectorSGLang,
			expectedECConnector:                          ecConnectorValue,
			expectedEnableSSRFProtection:                 true,
			expectedEnablePrefillerSampling:              true,
			expectedEnableTLS:                            []string{prefillStage},
			expectedUseTLSForPrefiller:                   true,
			expectedUseTLSForDecoder:                     false,
			expectedUseTLSForEncoder:                     false,
			expectedTLSInsecureSkipVerify:                []string{prefillStage},
			expectedPrefillerInsecureSkipVerify:          true,
			expectedDecoderInsecureSkipVerify:            false,
			expectedEncoderInsecureSkipVerify:            false,
			expectedSecureServing:                        false,
			expectedCertPath:                             "/etc/certificatesForCase2",
			expectedInferencePool:                        "namespaceForCase2/inferencepoolForCase2",
			expectedInferencePoolNamespace:               "namespaceForCase2",
			expectedInferencePoolName:                    "inferencepoolForCase2",
			expectedPoolGroup:                            "poolgroupForCase2",
			expectedConfigurationFromInlineSpecification: "",
			expectedConfigurationFromFile:                "",
			expectedConfigurationState: struct {
				FromEnv    []string
				FromFlags  []string
				FromInline []string
				FromFile   []string
				Defaults   []string
			}{
				FromFlags: []string{
					port,
					vllmPort,
					dataParallelSize,
					kvConnector,
					ecConnector,
					enableSSRFProtection,
					enablePrefillerSampling,
					enableTLS,
					TLSInsecureSkipVerify,
					SecureServing,
					certPath,
					inferencePool,
					poolGroup,
				},
			},
			expectedError: nil,
		},
		{
			name:       "Case 3: Configuration provided through environment variables: `INFERENCE_POOL`, `INFERENCE_POOL_NAMESPACE` and `INFERENCE_POOL_NAME`. `INFERENCE_POOL` has higher priority over `INFERENCE_POOL_NAMESPACE and `INFERENCE_POOL_NAME`. Environment variables over-ride default values.",
			inputFlags: nil,
			inputEnvVar: map[string]string{
				"INFERENCE_POOL":           "namespaceForCase3/inferencepoolForCase3",
				"INFERENCE_POOL_NAMESPACE": "ignoredInferenceNamespaceForCase3",
				"INFERENCE_POOL_NAME":      "ignoredInferenceNameForCase3",
			},
			expectedPort:                                 defaultPort,
			expectedVLLMPort:                             defaultvLLMPort,
			expectedDataParallelSize:                     defaultDataParallelSize,
			expectedKVConnector:                          KVConnectorNIXLV2,
			expectedECConnector:                          "",
			expectedEnableSSRFProtection:                 false,
			expectedEnablePrefillerSampling:              false,
			expectedEnableTLS:                            nil,
			expectedUseTLSForPrefiller:                   false,
			expectedUseTLSForDecoder:                     false,
			expectedUseTLSForEncoder:                     false,
			expectedTLSInsecureSkipVerify:                nil,
			expectedPrefillerInsecureSkipVerify:          false,
			expectedDecoderInsecureSkipVerify:            false,
			expectedEncoderInsecureSkipVerify:            false,
			expectedSecureServing:                        true,
			expectedInferencePool:                        "namespaceForCase3/inferencepoolForCase3",
			expectedInferencePoolNamespace:               "namespaceForCase3",
			expectedInferencePoolName:                    "inferencepoolForCase3",
			expectedPoolGroup:                            DefaultPoolGroup,
			expectedConfigurationFromInlineSpecification: "",
			expectedConfigurationFromFile:                "",
			expectedConfigurationState: struct {
				FromEnv    []string
				FromFlags  []string
				FromInline []string
				FromFile   []string
				Defaults   []string
			}{
				FromEnv: []string{
					inferencePool,
					inferencePoolNamespace,
					inferencePoolName,
				},
				Defaults: []string{
					port,
					vllmPort,
					dataParallelSize,
					kvConnector,
					ecConnector,
					enableSSRFProtection,
					enablePrefillerSampling,
					enableTLS,
					TLSInsecureSkipVerify,
					SecureServing,
					certPath,
					poolGroup,
				},
			},
			expectedError: nil,
		},
		{
			name: "Case 4: YAML configuration provided through inline specification `--configuration`. Configuration from YAML inline specification over-ride default configuration.",
			inputFlags: map[string]any{
				"configuration": &YAMLInlineSpecificationConfiguration1,
			},
			expectedPort:                                 "8011",
			expectedVLLMPort:                             "8021",
			expectedDataParallelSize:                     3,
			expectedKVConnector:                          KVConnectorSGLang,
			expectedECConnector:                          ecConnectorValue,
			expectedEnableSSRFProtection:                 true,
			expectedEnablePrefillerSampling:              true,
			expectedEnableTLS:                            []string{prefillStage, decodeStage},
			expectedUseTLSForPrefiller:                   true,
			expectedUseTLSForDecoder:                     true,
			expectedUseTLSForEncoder:                     false,
			expectedTLSInsecureSkipVerify:                []string{prefillStage, decodeStage},
			expectedPrefillerInsecureSkipVerify:          true,
			expectedDecoderInsecureSkipVerify:            true,
			expectedEncoderInsecureSkipVerify:            false,
			expectedSecureServing:                        false,
			expectedCertPath:                             "/etc/certificatesForInline",
			expectedInferencePool:                        "inlineNamespace1/inlineInferencepool1",
			expectedInferencePoolNamespace:               "inlineNamespace1",
			expectedInferencePoolName:                    "inlineInferencepool1",
			expectedPoolGroup:                            "inlinePoolgroup1",
			expectedConfigurationFromInlineSpecification: YAMLInlineSpecificationConfiguration1,
			expectedConfigurationFromFile:                "",
			expectedConfigurationState: struct {
				FromEnv    []string
				FromFlags  []string
				FromInline []string
				FromFile   []string
				Defaults   []string
			}{
				FromInline: []string{
					port,
					vllmPort,
					dataParallelSize,
					kvConnector,
					ecConnector,
					enableSSRFProtection,
					enablePrefillerSampling,
					enableTLS,
					"prefiller-use-tls",
					TLSInsecureSkipVerify,
					"prefiller-tls-insecure-skip-verify",
					SecureServing,
					certPath,
					inferencePool,
					poolGroup,
				},
			},
			expectedError: nil,
		},
		{
			name: "Case 5: YAML configuration provided through file `--configuration-file`. Configuration from YAML file over-ride default configuration.",
			inputFlags: map[string]any{
				"configuration-file": validSidecarfilePath,
			},
			expectedPort:                                 "8100",
			expectedVLLMPort:                             "8200",
			expectedDataParallelSize:                     4,
			expectedKVConnector:                          KVConnectorSGLang,
			expectedECConnector:                          ecConnectorValue,
			expectedEnableSSRFProtection:                 true,
			expectedEnablePrefillerSampling:              true,
			expectedEnableTLS:                            []string{prefillStage, decodeStage},
			expectedUseTLSForPrefiller:                   true,
			expectedUseTLSForDecoder:                     true,
			expectedUseTLSForEncoder:                     false,
			expectedTLSInsecureSkipVerify:                []string{prefillStage, decodeStage},
			expectedPrefillerInsecureSkipVerify:          true,
			expectedDecoderInsecureSkipVerify:            true,
			expectedEncoderInsecureSkipVerify:            false,
			expectedSecureServing:                        false,
			expectedCertPath:                             "/etc/certificates3",
			expectedInferencePool:                        "namespace3/inferencepool3",
			expectedInferencePoolNamespace:               "namespace3",
			expectedInferencePoolName:                    "inferencepool3",
			expectedPoolGroup:                            "poolgroup3",
			expectedConfigurationFromInlineSpecification: "",
			expectedConfigurationFromFile:                validSidecarfilePath,
			expectedConfigurationState: struct {
				FromEnv    []string
				FromFlags  []string
				FromInline []string
				FromFile   []string
				Defaults   []string
			}{
				FromFile: []string{
					port,
					vllmPort,
					dataParallelSize,
					kvConnector,
					ecConnector,
					enableSSRFProtection,
					enablePrefillerSampling,
					enableTLS,
					"prefiller-use-tls",
					TLSInsecureSkipVerify,
					"decoder-tls-insecure-skip-verify",
					SecureServing,
					certPath,
					inferencePool,
					poolGroup,
				},
			},
			expectedError: nil,
		},
		{
			name: "Case 6: Case 2 + Case 3: Configuration provided through individual flags + environment variables. Configuration from individual flags over-ride  configuration from environment variable.",
			inputFlags: map[string]any{
				port:                    "8100",
				vllmPort:                "8200",
				dataParallelSize:        2,
				kvConnector:             kvConnectorSGLang,
				ecConnector:             ecConnectorValue,
				enableSSRFProtection:    true,
				enablePrefillerSampling: true,
				enableTLS:               &[]string{prefillStage},
				TLSInsecureSkipVerify:   &[]string{prefillStage},
				SecureServing:           false,
				certPath:                "/etc/certificatesForCase7",
				inferencePool:           "namespaceForCase7/inferencepoolForCase7",
				poolGroup:               "poolgroupForCase7",
			},
			inputEnvVar: map[string]string{
				"INFERENCE_POOL":           "ignoredNamespaceForCase7FromEnvVar/ignoredInferencepoolForCase7FromEnvVar",
				"INFERENCE_POOL_NAMESPACE": "ignoredInferenceNamespaceForCase7",
				"INFERENCE_POOL_NAME":      "ignoredInferenceNameForCase7",
			},
			expectedPort:                                 "8100",
			expectedVLLMPort:                             "8200",
			expectedDataParallelSize:                     2,
			expectedKVConnector:                          KVConnectorSGLang,
			expectedECConnector:                          ecConnectorValue,
			expectedEnableSSRFProtection:                 true,
			expectedEnablePrefillerSampling:              true,
			expectedEnableTLS:                            []string{prefillStage},
			expectedUseTLSForPrefiller:                   true,
			expectedUseTLSForDecoder:                     false,
			expectedUseTLSForEncoder:                     false,
			expectedTLSInsecureSkipVerify:                []string{prefillStage},
			expectedPrefillerInsecureSkipVerify:          true,
			expectedDecoderInsecureSkipVerify:            false,
			expectedEncoderInsecureSkipVerify:            false,
			expectedSecureServing:                        false,
			expectedCertPath:                             "/etc/certificatesForCase7",
			expectedInferencePool:                        "namespaceForCase7/inferencepoolForCase7",
			expectedInferencePoolNamespace:               "namespaceForCase7",
			expectedInferencePoolName:                    "inferencepoolForCase7",
			expectedPoolGroup:                            "poolgroupForCase7",
			expectedConfigurationFromInlineSpecification: "",
			expectedConfigurationFromFile:                "",
			expectedConfigurationState: struct {
				FromEnv    []string
				FromFlags  []string
				FromInline []string
				FromFile   []string
				Defaults   []string
			}{
				FromFlags: []string{
					port,
					vllmPort,
					dataParallelSize,
					kvConnector,
					ecConnector,
					enableSSRFProtection,
					enablePrefillerSampling,
					enableTLS,
					TLSInsecureSkipVerify,
					SecureServing,
					certPath,
					inferencePool,
					poolGroup,
				},
				FromEnv: []string{
					inferencePoolNamespace,
					inferencePoolName,
				},
			},
			expectedError: nil,
		},
		// this is wrong need to change this: flag has higher priority over env var
		// add inference_pool again here
		{
			name: "case 7: configuration provided through environment variables: `INFERENCE_POOL_NAMESPACE` and `INFERENCE_POOL_NAME. Individual flags have higher priority over environment variables.",
			inputFlags: map[string]any{
				port:                    "8001",
				vllmPort:                "8002",
				dataParallelSize:        2,
				kvConnector:             kvConnectorSGLang,
				ecConnector:             ecConnectorValue,
				enableSSRFProtection:    true,
				enablePrefillerSampling: true,
				enableTLS:               &[]string{prefillStage},
				TLSInsecureSkipVerify:   &[]string{prefillStage},
				SecureServing:           false,
				certPath:                "/etc/certificatesForCase4",
				inferencePoolNamespace:  "inferencePoolNamespaceFromFlag",
				inferencePoolName:       "inferencePoolNameFromFlag",
				poolGroup:               "poolgroupForCase4",
			},
			inputEnvVar: map[string]string{
				"INFERENCE_POOL_NAMESPACE": "inferencePoolNamespaceFromEnvVar",
				"INFERENCE_POOL_NAME":      "inferencePoolNameFromEnvVar",
			},
			expectedPort:                                 "8001",
			expectedVLLMPort:                             "8002",
			expectedDataParallelSize:                     2,
			expectedKVConnector:                          KVConnectorSGLang,
			expectedECConnector:                          ecConnectorValue,
			expectedEnableSSRFProtection:                 true,
			expectedEnablePrefillerSampling:              true,
			expectedEnableTLS:                            []string{prefillStage},
			expectedUseTLSForPrefiller:                   true,
			expectedUseTLSForDecoder:                     false,
			expectedUseTLSForEncoder:                     false,
			expectedTLSInsecureSkipVerify:                []string{prefillStage},
			expectedPrefillerInsecureSkipVerify:          true,
			expectedDecoderInsecureSkipVerify:            false,
			expectedEncoderInsecureSkipVerify:            false,
			expectedSecureServing:                        false,
			expectedCertPath:                             "/etc/certificatesForCase4",
			expectedInferencePool:                        "",
			expectedInferencePoolNamespace:               "inferencePoolNamespaceFromFlag",
			expectedInferencePoolName:                    "inferencePoolNameFromFlag",
			expectedPoolGroup:                            "poolgroupForCase4",
			expectedConfigurationFromInlineSpecification: "",
			expectedConfigurationFromFile:                "",
			expectedConfigurationState: struct {
				FromEnv    []string
				FromFlags  []string
				FromInline []string
				FromFile   []string
				Defaults   []string
			}{
				FromEnv: []string{
					inferencePoolNamespace,
					inferencePoolName,
				},
				FromFlags: []string{
					port,
					vllmPort,
					dataParallelSize,
					kvConnector,
					ecConnector,
					enableSSRFProtection,
					enablePrefillerSampling,
					enableTLS,
					TLSInsecureSkipVerify,
					SecureServing,
					certPath,
					inferencePoolNamespace,
					inferencePoolName,
					poolGroup,
				},
			},
			expectedError: nil,
		},
		{
			name: "Case 8: Case 2 + Case 4: Configuration provided through individual flags + YAML inline specification. Configuration from individual flags over-ride configuration from inline specification.",
			inputFlags: map[string]any{
				port:                    "8100",
				vllmPort:                "8200",
				dataParallelSize:        2,
				kvConnector:             kvConnectorSGLang,
				ecConnector:             ecConnectorValue,
				enableSSRFProtection:    true,
				enablePrefillerSampling: true,
				enableTLS:               &[]string{prefillStage},
				TLSInsecureSkipVerify:   &[]string{prefillStage},
				SecureServing:           false,
				certPath:                "/etc/certificatesForCase8",
				inferencePool:           "namespaceForCase8/inferencepoolForCase8",
				poolGroup:               "poolgroupForCase8",
				"configuration":         YAMLInlineSpecificationConfiguration2,
			},
			expectedPort:                                 "8100",
			expectedVLLMPort:                             "8200",
			expectedDataParallelSize:                     2,
			expectedKVConnector:                          kvConnectorSGLang,
			expectedECConnector:                          ecConnectorValue,
			expectedEnableSSRFProtection:                 true,
			expectedEnablePrefillerSampling:              true,
			expectedEnableTLS:                            []string{prefillStage},
			expectedUseTLSForPrefiller:                   true,
			expectedUseTLSForDecoder:                     false,
			expectedUseTLSForEncoder:                     false,
			expectedTLSInsecureSkipVerify:                []string{prefillStage},
			expectedPrefillerInsecureSkipVerify:          true,
			expectedDecoderInsecureSkipVerify:            false,
			expectedEncoderInsecureSkipVerify:            false,
			expectedSecureServing:                        false,
			expectedCertPath:                             "/etc/certificatesForCase8",
			expectedInferencePool:                        "namespaceForCase8/inferencepoolForCase8",
			expectedInferencePoolNamespace:               "namespaceForCase8",
			expectedInferencePoolName:                    "inferencepoolForCase8",
			expectedPoolGroup:                            "poolgroupForCase8",
			expectedConfigurationFromInlineSpecification: YAMLInlineSpecificationConfiguration2,
			expectedConfigurationFromFile:                "",
			expectedConfigurationState: struct {
				FromEnv    []string
				FromFlags  []string
				FromInline []string
				FromFile   []string
				Defaults   []string
			}{
				FromFlags: []string{
					port,
					vllmPort,
					dataParallelSize,
					kvConnector,
					ecConnector,
					enableSSRFProtection,
					enablePrefillerSampling,
					enableTLS,
					TLSInsecureSkipVerify,
					SecureServing,
					certPath,
					inferencePool,
					poolGroup,
				},
				FromInline: []string{
					"prefiller-use-tls",
					"prefiller-tls-insecure-skip-verify",
				},
			},
			expectedError: nil,
		},
		{
			name: "Case 9: Case 2 + Case 5: Configuration provided through individual flags + file. Configuration from individual flags over-ride configuration from file.",
			inputFlags: map[string]any{
				port:                    "8001",
				vllmPort:                "8002",
				dataParallelSize:        2,
				kvConnector:             kvConnectorNIXLV2,
				ecConnector:             ecConnectorValue,
				enableSSRFProtection:    true,
				enablePrefillerSampling: true,
				enableTLS:               &[]string{},
				TLSInsecureSkipVerify:   &[]string{prefillStage, decodeStage, encodeStage},
				SecureServing:           false,
				certPath:                "/etc/certificatesForCase9",
				inferencePool:           "namespaceForCase9/inferencepoolForCase9",
				poolGroup:               "poolgroupForCase9",
				"configuration-file":    validSidecarfilePath,
			},
			expectedPort:                                 "8001",
			expectedVLLMPort:                             "8002",
			expectedDataParallelSize:                     2,
			expectedKVConnector:                          KVConnectorNIXLV2,
			expectedECConnector:                          ecConnectorValue,
			expectedEnableSSRFProtection:                 true,
			expectedEnablePrefillerSampling:              true,
			expectedEnableTLS:                            []string{},
			expectedUseTLSForPrefiller:                   false,
			expectedUseTLSForDecoder:                     false,
			expectedUseTLSForEncoder:                     false,
			expectedTLSInsecureSkipVerify:                []string{prefillStage, decodeStage, encodeStage},
			expectedPrefillerInsecureSkipVerify:          true,
			expectedDecoderInsecureSkipVerify:            true,
			expectedEncoderInsecureSkipVerify:            true,
			expectedSecureServing:                        false,
			expectedCertPath:                             "/etc/certificatesForCase9",
			expectedInferencePool:                        "namespaceForCase9/inferencepoolForCase9",
			expectedInferencePoolNamespace:               "namespaceForCase9",
			expectedInferencePoolName:                    "inferencepoolForCase9",
			expectedPoolGroup:                            "poolgroupForCase9",
			expectedConfigurationFromInlineSpecification: "",
			expectedConfigurationFromFile:                validSidecarfilePath,
			expectedConfigurationState: struct {
				FromEnv    []string
				FromFlags  []string
				FromInline []string
				FromFile   []string
				Defaults   []string
			}{
				FromFlags: []string{
					port,
					vllmPort,
					dataParallelSize,
					kvConnector,
					ecConnector,
					enableSSRFProtection,
					enablePrefillerSampling,
					enableTLS,
					TLSInsecureSkipVerify,
					SecureServing,
					certPath,
					inferencePool,
					poolGroup,
				},
				FromFile: []string{
					"prefiller-use-tls",
					"decoder-tls-insecure-skip-verify",
				},
			},
			expectedError: nil,
		},
		{
			name: "Case 10: Case 3 + Case 4: Configuration provided through environment variable + YAML inline specification. Configuration from environment variable over-rides configuration from inline specification.",
			inputFlags: map[string]any{
				"configuration": YAMLInlineSpecificationConfiguration1,
			},
			inputEnvVar: map[string]string{
				"INFERENCE_POOL":           "inferencePoolNamespace1/inferencePoolName1",
				"INFERENCE_POOL_NAMESPACE": "inferencePoolNamespace2",
				"INFERENCE_POOL_NAME":      "inferencePoolName2",
			},
			expectedPort:                                 "8011",
			expectedVLLMPort:                             "8021",
			expectedDataParallelSize:                     3,
			expectedKVConnector:                          KVConnectorSGLang,
			expectedECConnector:                          ecConnectorValue,
			expectedEnableSSRFProtection:                 true,
			expectedEnablePrefillerSampling:              true,
			expectedEnableTLS:                            []string{prefillStage, decodeStage},
			expectedUseTLSForPrefiller:                   true,
			expectedUseTLSForDecoder:                     true,
			expectedUseTLSForEncoder:                     false,
			expectedTLSInsecureSkipVerify:                []string{prefillStage, decodeStage},
			expectedPrefillerInsecureSkipVerify:          true,
			expectedDecoderInsecureSkipVerify:            true,
			expectedEncoderInsecureSkipVerify:            false,
			expectedSecureServing:                        false,
			expectedCertPath:                             "/etc/certificatesForInline",
			expectedInferencePool:                        "inferencePoolNamespace1/inferencePoolName1",
			expectedInferencePoolNamespace:               "inferencePoolNamespace1",
			expectedInferencePoolName:                    "inferencePoolName1",
			expectedPoolGroup:                            "inlinePoolgroup1",
			expectedConfigurationFromInlineSpecification: YAMLInlineSpecificationConfiguration1,
			expectedConfigurationFromFile:                "",
			expectedConfigurationState: struct {
				FromEnv    []string
				FromFlags  []string
				FromInline []string
				FromFile   []string
				Defaults   []string
			}{
				FromEnv: []string{
					inferencePool,
					inferencePoolNamespace,
					inferencePoolName,
				},
				FromInline: []string{
					port,
					vllmPort,
					dataParallelSize,
					kvConnector,
					ecConnector,
					enableSSRFProtection,
					enablePrefillerSampling,
					enableTLS,
					"prefiller-use-tls",
					TLSInsecureSkipVerify,
					"prefiller-tls-insecure-skip-verify",
					SecureServing,
					certPath,
					poolGroup,
				},
			},
			expectedError: nil,
		},
		{
			name: "Case 11: Case 3 + case 4: Configuration provided through environment variable + file. Configuration from environment variable over-rides configuration from inline specification.",
			inputFlags: map[string]any{
				"configuration-file": validSidecarfilePath,
			},
			inputEnvVar: map[string]string{
				"INFERENCE_POOL":           "namespaceForCase11/inferencepoolForCase11",
				"INFERENCE_POOL_NAMESPACE": "ignoredInferenceNamespaceForCase3",
				"INFERENCE_POOL_NAME":      "ignoredInferenceNameForCase3",
			},
			expectedPort:                                 "8100",
			expectedVLLMPort:                             "8200",
			expectedDataParallelSize:                     4,
			expectedKVConnector:                          KVConnectorSGLang,
			expectedECConnector:                          ecConnectorValue,
			expectedEnableSSRFProtection:                 true,
			expectedEnablePrefillerSampling:              true,
			expectedEnableTLS:                            []string{prefillStage, decodeStage},
			expectedUseTLSForPrefiller:                   true,
			expectedUseTLSForDecoder:                     true,
			expectedUseTLSForEncoder:                     false,
			expectedTLSInsecureSkipVerify:                []string{prefillStage, decodeStage},
			expectedPrefillerInsecureSkipVerify:          true,
			expectedDecoderInsecureSkipVerify:            true,
			expectedEncoderInsecureSkipVerify:            false,
			expectedSecureServing:                        false,
			expectedCertPath:                             "/etc/certificates3",
			expectedInferencePool:                        "namespaceForCase11/inferencepoolForCase11",
			expectedInferencePoolNamespace:               "namespaceForCase11",
			expectedInferencePoolName:                    "inferencepoolForCase11",
			expectedPoolGroup:                            "poolgroup3",
			expectedConfigurationFromInlineSpecification: "",
			expectedConfigurationFromFile:                validSidecarfilePath,
			expectedConfigurationState: struct {
				FromEnv    []string
				FromFlags  []string
				FromInline []string
				FromFile   []string
				Defaults   []string
			}{
				FromEnv: []string{
					inferencePool,
					inferencePoolNamespace,
					inferencePoolName,
				},
				FromFile: []string{
					port,
					vllmPort,
					dataParallelSize,
					kvConnector,
					ecConnector,
					enableSSRFProtection,
					enablePrefillerSampling,
					enableTLS,
					"prefiller-use-tls",
					TLSInsecureSkipVerify,
					"decoder-tls-insecure-skip-verify",
					SecureServing,
					certPath,
					poolGroup,
				},
			},
			expectedError: nil,
		},
		{
			name: "Case 12: Case 4 + Case 5: Configuration provided through YAML inline specification + file. YAML inline specification over-rides values from file.",
			inputFlags: map[string]any{
				"configuration":      YAMLInlineSpecificationConfiguration1,
				"configuration-file": validSidecarfilePath,
			},
			expectedPort:                                 "8011",
			expectedVLLMPort:                             "8021",
			expectedDataParallelSize:                     3,
			expectedKVConnector:                          KVConnectorSGLang,
			expectedECConnector:                          ecConnectorValue,
			expectedEnableSSRFProtection:                 true,
			expectedEnablePrefillerSampling:              true,
			expectedEnableTLS:                            []string{prefillStage, decodeStage},
			expectedUseTLSForPrefiller:                   true,
			expectedUseTLSForDecoder:                     true,
			expectedUseTLSForEncoder:                     false,
			expectedTLSInsecureSkipVerify:                []string{prefillStage, decodeStage},
			expectedPrefillerInsecureSkipVerify:          true,
			expectedDecoderInsecureSkipVerify:            true,
			expectedEncoderInsecureSkipVerify:            false,
			expectedSecureServing:                        false,
			expectedCertPath:                             "/etc/certificatesForInline",
			expectedInferencePool:                        "inlineNamespace1/inlineInferencepool1",
			expectedInferencePoolNamespace:               "inlineNamespace1",
			expectedInferencePoolName:                    "inlineInferencepool1",
			expectedPoolGroup:                            "inlinePoolgroup1",
			expectedConfigurationFromInlineSpecification: YAMLInlineSpecificationConfiguration1,
			expectedConfigurationFromFile:                validSidecarfilePath,
			expectedConfigurationState: struct {
				FromEnv    []string
				FromFlags  []string
				FromInline []string
				FromFile   []string
				Defaults   []string
			}{
				FromInline: []string{
					port,
					vllmPort,
					dataParallelSize,
					kvConnector,
					ecConnector,
					enableSSRFProtection,
					enablePrefillerSampling,
					enableTLS,
					"prefiller-use-tls",
					TLSInsecureSkipVerify,
					"prefiller-tls-insecure-skip-verify",
					SecureServing,
					certPath,
					inferencePool,
					poolGroup,
				},
				FromFile: []string{
					"decoder-tls-insecure-skip-verify",
				},
			},
			expectedError: nil,
		},
		{
			name: "Case 13: Case 2 + Case 4 + Case 5: Configuration provided through individual flags + YAML inline specification + file. Individual flags have highest priority, then inline specification, then file.",
			inputFlags: map[string]any{
				port:                    "8001",
				vllmPort:                "8002",
				dataParallelSize:        2,
				kvConnector:             kvConnectorSGLang,
				ecConnector:             ecConnectorValue,
				enableSSRFProtection:    true,
				enablePrefillerSampling: true,
				enableTLS:               &[]string{prefillStage},
				TLSInsecureSkipVerify:   &[]string{prefillStage},
				SecureServing:           false,
				certPath:                "/etc/certificatesForCase2",
				inferencePool:           "namespaceForCase2/inferencepoolForCase2",
				poolGroup:               "poolgroupForCase2",
				"configuration":         YAMLInlineSpecificationConfiguration1,
				"configuration-file":    validSidecarfilePath,
			},
			expectedPort:                                 "8001",
			expectedVLLMPort:                             "8002",
			expectedDataParallelSize:                     2,
			expectedKVConnector:                          KVConnectorSGLang,
			expectedECConnector:                          ecConnectorValue,
			expectedEnableSSRFProtection:                 true,
			expectedEnablePrefillerSampling:              true,
			expectedEnableTLS:                            []string{prefillStage},
			expectedUseTLSForPrefiller:                   true,
			expectedUseTLSForDecoder:                     false,
			expectedUseTLSForEncoder:                     false,
			expectedTLSInsecureSkipVerify:                []string{prefillStage, decodeStage},
			expectedPrefillerInsecureSkipVerify:          true,
			expectedDecoderInsecureSkipVerify:            true,
			expectedEncoderInsecureSkipVerify:            false,
			expectedSecureServing:                        false,
			expectedCertPath:                             "/etc/certificatesForCase2",
			expectedInferencePool:                        "namespaceForCase2/inferencepoolForCase2",
			expectedInferencePoolNamespace:               "namespaceForCase2",
			expectedInferencePoolName:                    "inferencepoolForCase2",
			expectedPoolGroup:                            "poolgroupForCase2",
			expectedConfigurationFromInlineSpecification: YAMLInlineSpecificationConfiguration1,
			expectedConfigurationFromFile:                validSidecarfilePath,
			expectedConfigurationState: struct {
				FromEnv    []string
				FromFlags  []string
				FromInline []string
				FromFile   []string
				Defaults   []string
			}{
				FromFlags: []string{
					port,
					vllmPort,
					dataParallelSize,
					kvConnector,
					ecConnector,
					enableSSRFProtection,
					enablePrefillerSampling,
					enableTLS,
					TLSInsecureSkipVerify,
					SecureServing,
					certPath,
					inferencePool,
					poolGroup,
				},
				FromInline: []string{
					"prefiller-use-tls",
					"prefiller-tls-insecure-skip-verify",
				},
				FromFile: []string{
					"decoder-tls-insecure-skip-verify",
				},
			},
			expectedError: nil,
		},
		{
			name: "Case 12: Case 4 + Case 5: Configuration provided through YAML inline specification + file. YAML inline specification over-rides values from file.",
			inputFlags: map[string]any{
				"configuration":      YAMLInlineSpecificationConfiguration1,
				"configuration-file": validSidecarfilePath,
			},
			expectedPort:                                 "8011",
			expectedVLLMPort:                             "8021",
			expectedDataParallelSize:                     3,
			expectedKVConnector:                          KVConnectorSGLang,
			expectedECConnector:                          ecConnectorValue,
			expectedEnableSSRFProtection:                 true,
			expectedEnablePrefillerSampling:              true,
			expectedEnableTLS:                            []string{prefillStage, decodeStage},
			expectedUseTLSForPrefiller:                   true,
			expectedUseTLSForDecoder:                     true,
			expectedUseTLSForEncoder:                     false,
			expectedTLSInsecureSkipVerify:                []string{prefillStage, decodeStage},
			expectedPrefillerInsecureSkipVerify:          true,
			expectedDecoderInsecureSkipVerify:            true,
			expectedEncoderInsecureSkipVerify:            false,
			expectedSecureServing:                        false,
			expectedCertPath:                             "/etc/certificatesForInline",
			expectedInferencePool:                        "inlineNamespace1/inlineInferencepool1",
			expectedInferencePoolNamespace:               "inlineNamespace1",
			expectedInferencePoolName:                    "inlineInferencepool1",
			expectedPoolGroup:                            "inlinePoolgroup1",
			expectedConfigurationFromInlineSpecification: YAMLInlineSpecificationConfiguration1,
			expectedConfigurationFromFile:                validSidecarfilePath,
			expectedConfigurationState: struct {
				FromEnv    []string
				FromFlags  []string
				FromInline []string
				FromFile   []string
				Defaults   []string
			}{
				FromInline: []string{
					port,
					vllmPort,
					dataParallelSize,
					kvConnector,
					ecConnector,
					enableSSRFProtection,
					enablePrefillerSampling,
					enableTLS,
					"prefiller-use-tls",
					TLSInsecureSkipVerify,
					"prefiller-tls-insecure-skip-verify",
					SecureServing,
					certPath,
					inferencePool,
					poolGroup,
				},
				FromFile: []string{
					"decoder-tls-insecure-skip-verify",
				},
			},
			expectedError: nil,
		},
		{
			name: "Case 14: Case 3 + Case 4 + Case 5 i.e. configuration provided through environment variable + YAML inline specification + file. Configuration from environment variable over-rides configuration from inline specification. Configuration from inline specification over-rides configuration from file.",
			inputFlags: map[string]any{
				"configuration":      YAMLInlineSpecificationConfiguration1,
				"configuration-file": validSidecarfilePath,
			},
			inputEnvVar: map[string]string{
				"INFERENCE_POOL":           "namespaceForCase14FromEnvVar/nameForCase14FromEnvVar",
				"INFERENCE_POOL_NAMESPACE": "ignoredInferenceNamespaceForCase14",
				"INFERENCE_POOL_NAME":      "ignoredInferenceNameForCase14",
			},
			expectedPort:                                 "8011",
			expectedVLLMPort:                             "8021",
			expectedDataParallelSize:                     3,
			expectedKVConnector:                          KVConnectorSGLang,
			expectedECConnector:                          ecConnectorValue,
			expectedEnableSSRFProtection:                 true,
			expectedEnablePrefillerSampling:              true,
			expectedEnableTLS:                            []string{prefillStage, decodeStage},
			expectedUseTLSForPrefiller:                   true,
			expectedUseTLSForDecoder:                     true,
			expectedUseTLSForEncoder:                     false,
			expectedTLSInsecureSkipVerify:                []string{prefillStage, decodeStage},
			expectedPrefillerInsecureSkipVerify:          true,
			expectedDecoderInsecureSkipVerify:            true,
			expectedEncoderInsecureSkipVerify:            false,
			expectedSecureServing:                        false,
			expectedCertPath:                             "/etc/certificatesForInline",
			expectedInferencePool:                        "namespaceForCase14FromEnvVar/nameForCase14FromEnvVar",
			expectedInferencePoolNamespace:               "namespaceForCase14FromEnvVar",
			expectedInferencePoolName:                    "nameForCase14FromEnvVar",
			expectedPoolGroup:                            "inlinePoolgroup1",
			expectedConfigurationFromInlineSpecification: YAMLInlineSpecificationConfiguration1,
			expectedConfigurationFromFile:                validSidecarfilePath,
			expectedConfigurationState: struct {
				FromEnv    []string
				FromFlags  []string
				FromInline []string
				FromFile   []string
				Defaults   []string
			}{
				FromInline: []string{
					port,
					vllmPort,
					dataParallelSize,
					kvConnector,
					ecConnector,
					enableSSRFProtection,
					enablePrefillerSampling,
					enableTLS,
					TLSInsecureSkipVerify,
					SecureServing,
					certPath,
					// inferencePool,
					poolGroup,
					"prefiller-use-tls",
					"prefiller-tls-insecure-skip-verify",
				},
				FromEnv: []string{
					inferencePool,
					inferencePoolNamespace,
					inferencePoolName,
				},
				FromFile: []string{
					"decoder-tls-insecure-skip-verify",
				},
			},
			expectedError: nil,
		},
		{
			name: "Case 15: Case 2 + Case 3 + Case 4 + Case 5 i.e. configuration provided through individual flags + environment variable + YAML inline specification + file. Configuration from Individual flags over-ride configuration from everything else.",
			inputFlags: map[string]any{
				port:                    "8001",
				vllmPort:                "8002",
				dataParallelSize:        2,
				kvConnector:             kvConnectorSGLang,
				ecConnector:             ecConnectorValue,
				enableSSRFProtection:    true,
				enablePrefillerSampling: true,
				enableTLS:               &[]string{prefillStage},
				TLSInsecureSkipVerify:   &[]string{prefillStage},
				SecureServing:           false,
				certPath:                "/etc/certificatesForCase14",
				inferencePool:           "namespaceForCase14/nameForCase14",
				poolGroup:               "poolgroupForCase14",
				"configuration":         YAMLInlineSpecificationConfiguration1,
				"configuration-file":    validSidecarfilePath,
			},
			inputEnvVar: map[string]string{
				"INFERENCE_POOL":           "ignoredNamespaceForCase14FromEnvVar/ignoredNameForCase14FromEnvVar",
				"INFERENCE_POOL_NAMESPACE": "ignoredInferenceNamespaceForCase14",
				"INFERENCE_POOL_NAME":      "ignoredInferenceNameForCase14",
			},
			expectedPort:                                 "8001",
			expectedVLLMPort:                             "8002",
			expectedDataParallelSize:                     2,
			expectedKVConnector:                          KVConnectorSGLang,
			expectedECConnector:                          ecConnectorValue,
			expectedEnableSSRFProtection:                 true,
			expectedEnablePrefillerSampling:              true,
			expectedEnableTLS:                            []string{prefillStage},
			expectedUseTLSForPrefiller:                   true,
			expectedUseTLSForDecoder:                     false,
			expectedUseTLSForEncoder:                     false,
			expectedTLSInsecureSkipVerify:                []string{prefillStage, decodeStage},
			expectedPrefillerInsecureSkipVerify:          true,
			expectedDecoderInsecureSkipVerify:            true,
			expectedEncoderInsecureSkipVerify:            false,
			expectedSecureServing:                        false,
			expectedCertPath:                             "/etc/certificatesForCase14",
			expectedInferencePool:                        "namespaceForCase14/nameForCase14",
			expectedInferencePoolNamespace:               "namespaceForCase14",
			expectedInferencePoolName:                    "nameForCase14",
			expectedPoolGroup:                            "poolgroupForCase14",
			expectedConfigurationFromInlineSpecification: YAMLInlineSpecificationConfiguration1,
			expectedConfigurationFromFile:                validSidecarfilePath,
			expectedConfigurationState: struct {
				FromEnv    []string
				FromFlags  []string
				FromInline []string
				FromFile   []string
				Defaults   []string
			}{
				FromFlags: []string{
					port,
					vllmPort,
					dataParallelSize,
					kvConnector,
					ecConnector,
					enableSSRFProtection,
					enablePrefillerSampling,
					enableTLS,
					TLSInsecureSkipVerify,
					SecureServing,
					certPath,
					inferencePool,
					poolGroup,
				},
				FromEnv: []string{
					inferencePoolNamespace,
					inferencePoolName,
				},
				FromInline: []string{
					"prefiller-use-tls",
					"prefiller-tls-insecure-skip-verify",
				},
				FromFile: []string{
					"decoder-tls-insecure-skip-verify",
				},
			},
			expectedError: nil,
		},
		{
			name: "Case 16: Invalid YAML configuration provided through inline specification `--configuration`. Error is expected.",
			inputFlags: map[string]any{
				"configuration": invalidConfiguration,
			},
			expectedPort:                                 defaultPort,
			expectedVLLMPort:                             defaultvLLMPort,
			expectedDataParallelSize:                     defaultDataParallelSize,
			expectedKVConnector:                          KVConnectorNIXLV2,
			expectedECConnector:                          "",
			expectedEnableSSRFProtection:                 false,
			expectedEnablePrefillerSampling:              false,
			expectedEnableTLS:                            nil,
			expectedUseTLSForPrefiller:                   false,
			expectedUseTLSForDecoder:                     false,
			expectedUseTLSForEncoder:                     false,
			expectedTLSInsecureSkipVerify:                nil,
			expectedPrefillerInsecureSkipVerify:          false,
			expectedDecoderInsecureSkipVerify:            false,
			expectedEncoderInsecureSkipVerify:            false,
			expectedSecureServing:                        true,
			expectedCertPath:                             "",
			expectedInferencePool:                        "",
			expectedPoolGroup:                            DefaultPoolGroup,
			expectedConfigurationFromInlineSpecification: invalidConfiguration,
			expectedConfigurationFromFile:                "",
			expectedConfigurationState: struct {
				FromEnv    []string
				FromFlags  []string
				FromInline []string
				FromFile   []string
				Defaults   []string
			}{
				Defaults: []string{
					port,
					vllmPort,
					dataParallelSize,
					kvConnector,
					ecConnector,
					enableSSRFProtection,
					enablePrefillerSampling,
					enableTLS,
					TLSInsecureSkipVerify,
					SecureServing,
					certPath,
					poolGroup},
			},
			expectedError: expectedError,
		},
		{
			name: "Case 17: Invalid YAML configuration provided through file `--configuration`. Error is expected.",
			inputFlags: map[string]any{
				"configuration-file": invalidSidecarfilePath,
			},
			expectedPort:                                 defaultPort,
			expectedVLLMPort:                             defaultvLLMPort,
			expectedDataParallelSize:                     defaultDataParallelSize,
			expectedKVConnector:                          KVConnectorNIXLV2,
			expectedECConnector:                          "",
			expectedEnableSSRFProtection:                 false,
			expectedEnablePrefillerSampling:              false,
			expectedEnableTLS:                            nil,
			expectedUseTLSForPrefiller:                   false,
			expectedUseTLSForDecoder:                     false,
			expectedUseTLSForEncoder:                     false,
			expectedTLSInsecureSkipVerify:                nil,
			expectedPrefillerInsecureSkipVerify:          false,
			expectedDecoderInsecureSkipVerify:            false,
			expectedEncoderInsecureSkipVerify:            false,
			expectedSecureServing:                        true,
			expectedCertPath:                             "",
			expectedInferencePool:                        "",
			expectedPoolGroup:                            DefaultPoolGroup,
			expectedConfigurationFromInlineSpecification: "",
			expectedConfigurationFromFile:                invalidSidecarfilePath,
			expectedConfigurationState: struct {
				FromEnv    []string
				FromFlags  []string
				FromInline []string
				FromFile   []string
				Defaults   []string
			}{
				Defaults: []string{
					port,
					vllmPort,
					dataParallelSize,
					kvConnector,
					ecConnector,
					enableSSRFProtection,
					enablePrefillerSampling,
					enableTLS,
					TLSInsecureSkipVerify,
					SecureServing,
					certPath,
					poolGroup},
			},
			expectedError: expectedError,
		},
		// {
		// 	name: "Case 18",
		// 	inputFlags: map[string]any{
		// 		"configuration": validSidecarfilePath,
		// 		"configuration": validSidecarfilePath,
		// 	},
		// 	expectedPort:                                 defaultPort,
		// 	expectedVLLMPort:                             defaultvLLMPort,
		// 	expectedDataParallelSize:                     defaultDataParallelSize,
		// 	expectedKVConnector:                          KVConnectorNIXLV2,
		// 	expectedECConnector:                          "",
		// 	expectedEnableSSRFProtection:                 false,
		// 	expectedEnablePrefillerSampling:              false,
		// 	expectedEnableTLS:                            nil,
		// 	expectedUseTLSForPrefiller:                   false,
		// 	expectedUseTLSForDecoder:                     false,
		// 	expectedUseTLSForEncoder:                     false,
		// 	expectedTLSInsecureSkipVerify:                nil,
		// 	expectedPrefillerInsecureSkipVerify:          false,
		// 	expectedDecoderInsecureSkipVerify:            false,
		// 	expectedEncoderInsecureSkipVerify:            false,
		// 	expectedSecureServing:                        true,
		// 	expectedCertPath:                             "",
		// 	expectedInferencePool:                        "",
		// 	expectedPoolGroup:                            DefaultPoolGroup,
		// 	expectedConfigurationFromInlineSpecification: "",
		// 	expectedConfigurationFromFile:                invalidSidecarfilePath,
		// 	expectedConfigurationState: struct {
		// 		FromEnv    []string
		// 		FromFlags  []string
		// 		FromInline []string
		// 		FromFile   []string
		// 		Defaults   []string
		// 	}{
		// 		Defaults: []string{
		// 			port,
		// 			vllmPort,
		// 			dataParallelSize,
		// 			kvConnector,
		// 			ecConnector,
		// 			enableSSRFProtection,
		// 			enablePrefillerSampling,
		// 			enableTLS,
		// 			TLSInsecureSkipVerify,
		// 			SecureServing,
		// 			certPath,
		// 			poolGroup},
		// 	},
		// 	expectedError: expectedError,
		// },
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for envVarKey, envVarValue := range tt.inputEnvVar {
				if tt.inputEnvVar[envVarKey] != "" {
					t.Setenv(envVarKey, envVarValue)
				}
			}
			opts, testFlagSet := newTestOptions(t)
			for flagName, flagValue := range tt.inputFlags {
				if flagValue != nil {
					switch v := flagValue.(type) {
					case int:
						require.NoError(t, testFlagSet.Set(flagName, strconv.Itoa(v)))
					case float64:
						require.NoError(t, testFlagSet.Set(flagName, fmt.Sprintf("%v", v)))
					case string:
						require.NoError(t, testFlagSet.Set(flagName, v))
					case *string:
						require.NoError(t, testFlagSet.Set(flagName, *v))
					case *[]string:
						require.NoError(t, testFlagSet.Set(flagName, strings.Join(*v, ",")))
					case bool:
						require.NoError(t, testFlagSet.Set(flagName, strconv.FormatBool(v)))
					default:
						t.Errorf("%v type is unknown, value: %v", flagName, flagValue)
					}
				}
			}
			err := testFlagSet.Parse(nil)
			require.NoError(t, err, "Flag Parse() error: %v", err)
			opts.GetConfigurationState()
			err = opts.Complete()
			if tt.expectedError != nil {
				require.ErrorContains(t, err, tt.expectedError.Error(), "Error should be: %v, got: %v", tt.expectedError, err)
				return
			} else {
				require.NoError(t, err, "Complete() error: %v", err)
			}
			err = opts.Validate()
			require.NoError(t, err, "Validate() error: %v", err)

			require.Equal(t, tt.expectedPort, opts.Port,
				"expected %v to be %v but got %v", port, tt.expectedPort, opts.Port)
			require.Equal(t, tt.expectedVLLMPort, opts.vllmPort,
				"expected %v to be %v but got %v", vllmPort, tt.expectedVLLMPort, opts.vllmPort)
			require.Equal(t, tt.expectedDataParallelSize, opts.DataParallelSize,
				"expected %v to be %v but got %v", dataParallelSize, tt.expectedDataParallelSize, opts.DataParallelSize)

			require.Equal(t, tt.expectedKVConnector, opts.KVConnector,
				"expected %v to be %v but got %v", kvConnector, tt.expectedKVConnector, opts.KVConnector)
			require.Equal(t, tt.expectedECConnector, opts.ECConnector,
				"expected %v to be %v but got %v", ecConnector, tt.expectedECConnector, opts.ECConnector)

			require.Equal(t, tt.expectedEnableSSRFProtection, opts.EnableSSRFProtection,
				"expected %v to be %v but got %v", enableSSRFProtection, tt.expectedEnableSSRFProtection, opts.EnableSSRFProtection)
			require.Equal(t, tt.expectedEnablePrefillerSampling, opts.EnablePrefillerSampling,
				"expected %v to be %v but got %v", enablePrefillerSampling, tt.expectedEnablePrefillerSampling, opts.EnablePrefillerSampling)

			require.Equal(t, tt.expectedUseTLSForPrefiller, opts.UseTLSForPrefiller,
				"expected %v to be %v but got %v", prefillerUseTLS, tt.expectedUseTLSForPrefiller, opts.UseTLSForPrefiller)
			require.Equal(t, tt.expectedUseTLSForDecoder, opts.UseTLSForDecoder,
				"expected %v to be %v but got %v", decoderUseTLS, tt.expectedUseTLSForDecoder, opts.UseTLSForDecoder)
			require.Equal(t, tt.expectedUseTLSForEncoder, opts.UseTLSForEncoder,
				"expected UseTLSForEncoder to be %v but got %v", tt.expectedUseTLSForEncoder, opts.UseTLSForEncoder)

			require.Equal(t, tt.expectedPrefillerInsecureSkipVerify, opts.InsecureSkipVerifyForPrefiller,
				"expected %v to be %v but got %v", prefillerTLSInsecureSkipVerify, tt.expectedPrefillerInsecureSkipVerify, opts.InsecureSkipVerifyForPrefiller)
			require.Equal(t, tt.expectedDecoderInsecureSkipVerify, opts.InsecureSkipVerifyForDecoder,
				"expected %v to be %v but got %v", decoderTLSInsecureSkipVerify, tt.expectedDecoderInsecureSkipVerify, opts.InsecureSkipVerifyForDecoder)
			require.Equal(t, tt.expectedEncoderInsecureSkipVerify, opts.InsecureSkipVerifyForEncoder,
				"expected InsecureSkipVerifyForEncoder to be %v but got %v", tt.expectedEncoderInsecureSkipVerify, opts.InsecureSkipVerifyForEncoder)

			require.True(t, sameElements(tt.expectedTLSInsecureSkipVerify, opts.tlsInsecureSkipVerify),
				"%v should contain same values: expected %v but got %v", TLSInsecureSkipVerify, tt.expectedTLSInsecureSkipVerify, opts.tlsInsecureSkipVerify)
			require.True(t, sameElements(tt.expectedEnableTLS, opts.enableTLS),
				"%v should contain same values: expected %v but got %v", enableTLS, tt.expectedEnableTLS, opts.enableTLS)

			require.Equal(t, tt.expectedCertPath, opts.CertPath,
				"expected %v to be %v but got %v", certPath, tt.expectedCertPath, opts.CertPath)
			require.Equal(t, tt.expectedSecureServing, opts.SecureServing,
				"expected %v to be %v but got %v", SecureServing, tt.expectedSecureServing, opts.SecureServing)

			require.Equal(t, tt.expectedInferencePool, opts.inferencePool,
				"expected %v to be %v but got %v", inferencePool, tt.expectedInferencePool, opts.inferencePool)
			require.Equal(t, tt.expectedInferencePoolNamespace, opts.InferencePoolNamespace,
				"expected %v to be %v but got %v", inferencePoolNamespace, tt.expectedInferencePoolNamespace, opts.InferencePoolNamespace)
			require.Equal(t, tt.expectedInferencePoolName, opts.InferencePoolName,
				"expected %v to be %v but got %v", inferencePoolName, tt.expectedInferencePoolName, opts.InferencePoolName)

			require.Equal(t, tt.expectedPoolGroup, opts.PoolGroup,
				"expected %v to be %v but got %v", poolGroup, tt.expectedPoolGroup, opts.PoolGroup)

			require.Equal(t, tt.expectedConfigurationFromInlineSpecification, opts.ConfigurationFromInlineSpecification,
				"expected configuration from inline specification to be %v but got %v", tt.expectedConfigurationFromInlineSpecification, opts.ConfigurationFromInlineSpecification)
			require.Equal(t, tt.expectedConfigurationFromFile, opts.ConfigurationFromFile,
				"expected configuration from file to be %v but got %v", tt.expectedConfigurationFromFile, opts.ConfigurationFromFile)

			require.True(t, sameElements(tt.expectedConfigurationState.Defaults, opts.ConfigurationState.Defaults),
				"Configuration state (defaults) should contain same values: expected %v but got %v", tt.expectedConfigurationState.Defaults, opts.ConfigurationState.Defaults)
			require.True(t, sameElements(tt.expectedConfigurationState.FromEnv, opts.ConfigurationState.FromEnv),
				"Configuration state (env var) should contain same values: expected %v but got %v", tt.expectedConfigurationState.FromEnv, opts.ConfigurationState.FromEnv)
			require.True(t, sameElements(tt.expectedConfigurationState.FromFlags, opts.ConfigurationState.FromFlags),
				"Configuration state (flags) should contain same values: expected %v but got %v", tt.expectedConfigurationState.FromFlags, opts.ConfigurationState.FromFlags)
			require.True(t, sameElements(tt.expectedConfigurationState.FromInline, opts.ConfigurationState.FromInline),
				"Configuration state (inline) should contain same values: expected %v but got %v", tt.expectedConfigurationState.FromInline, opts.ConfigurationState.FromInline)
			require.True(t, sameElements(tt.expectedConfigurationState.FromFile, opts.ConfigurationState.FromFile),
				"Configuration state (file) should contain same values: expected %v but got %v", tt.expectedConfigurationState.FromFile, opts.ConfigurationState.FromFile)

			require.Equal(t, calculateURL(t, tt.expectedUseTLSForDecoder, tt.expectedVLLMPort), opts.DecoderURL)
		})
	}
}

// TODO: update function name
func calculateURL(t *testing.T, decoder bool, vllmport string) *url.URL {
	expectedScheme := "http"
	if decoder {
		expectedScheme = schemeHTTPS
	}
	a, err := url.Parse(expectedScheme + "://localhost:" + vllmport)
	require.NoError(t, err)
	return a
}

// TODO: change the name of this function
func sameElements(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[string]int)
	for _, x := range a {
		counts[x]++
	}
	for _, x := range b {
		if counts[x] == 0 {
			return false
		}
		counts[x]--
	}
	return true
}

func TestNewOptionsWithEnvVars(t *testing.T) {
	// Set environment variables - t.Setenv automatically handles cleanup
	t.Setenv("INFERENCE_POOL_NAMESPACE", "test-namespace")
	t.Setenv("INFERENCE_POOL_NAME", "test-pool")
	t.Setenv("ENABLE_PREFILLER_SAMPLING", "true")

	opts := NewOptions()

	if opts.InferencePoolNamespace != "test-namespace" {
		t.Errorf("Expected InferencePoolNamespace to be 'test-namespace', got '%s'", opts.InferencePoolNamespace)
	}
	if opts.InferencePoolName != "test-pool" {
		t.Errorf("Expected InferencePoolName to be 'test-pool', got '%s'", opts.InferencePoolName)
	}
	if !opts.EnablePrefillerSampling {
		t.Error("Expected EnablePrefillerSampling to be true")
	}
}

func TestValidateConnector(t *testing.T) {
	tests := []struct {
		name      string
		connector string
		wantErr   bool
	}{
		{"valid nixlv2", KVConnectorNIXLV2, false},
		{"valid shared-storage", KVConnectorSharedStorage, false},
		{"valid sglang", KVConnectorSGLang, false},
		{"invalid connector", "invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := NewOptions()
			opts.connector = tt.connector
			_ = opts.Complete() // Complete must be called before Validate
			err := opts.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateTLSStages(t *testing.T) {
	tests := []struct {
		name      string
		enableTLS []string
		wantErr   bool
	}{
		{name: "valid prefiller", enableTLS: []string{"prefiller"}, wantErr: false},
		{name: "valid decoder", enableTLS: []string{"decoder"}, wantErr: false},
		{name: "valid both", enableTLS: []string{"prefiller", "decoder"}, wantErr: false},
		{name: "invalid stage", enableTLS: []string{"invalid"}, wantErr: true},
		{name: "mixed valid and invalid", enableTLS: []string{"prefiller", "invalid"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := NewOptions()
			opts.enableTLS = tt.enableTLS
			_ = opts.Complete() // Complete must be called before Validate
			err := opts.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateSSRFProtection(t *testing.T) {
	tests := []struct {
		name      string
		enabled   bool
		namespace string
		poolName  string
		wantErr   bool
	}{
		{name: "disabled", enabled: false, namespace: "", poolName: "", wantErr: false},
		{name: "enabled with both", enabled: true, namespace: "ns", poolName: "pool", wantErr: false},
		{name: "enabled missing namespace", enabled: true, namespace: "", poolName: "pool", wantErr: true},
		{name: "enabled missing pool name", enabled: true, namespace: "ns", poolName: "", wantErr: true},
		{name: "enabled missing both", enabled: true, namespace: "", poolName: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := NewOptions()
			opts.EnableSSRFProtection = tt.enabled
			opts.InferencePoolNamespace = tt.namespace
			opts.InferencePoolName = tt.poolName
			_ = opts.Complete() // Complete must be called before Validate
			err := opts.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCompleteInferencePoolParsing(t *testing.T) {
	tests := []struct {
		name              string
		inferencePool     string
		expectedNamespace string
		expectedName      string
	}{
		{
			name:              "namespace/name format",
			inferencePool:     "my-namespace/my-pool",
			expectedNamespace: "my-namespace",
			expectedName:      "my-pool",
		},
		{
			name:              "name only implies default namespace",
			inferencePool:     "my-pool",
			expectedNamespace: "default",
			expectedName:      "my-pool",
		},
		{
			name:              "empty string does not set values",
			inferencePool:     "",
			expectedNamespace: "",
			expectedName:      "",
		},
		{
			name:              "deprecated flags take precedence when InferencePool is empty",
			inferencePool:     "",
			expectedNamespace: "",
			expectedName:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := NewOptions()
			opts.inferencePool = tt.inferencePool

			err := opts.Complete()
			if err != nil {
				t.Fatalf("Complete() unexpected error: %v", err)
			}

			if opts.InferencePoolNamespace != tt.expectedNamespace {
				t.Errorf("InferencePoolNamespace = %v, want %v", opts.InferencePoolNamespace, tt.expectedNamespace)
			}
			if opts.InferencePoolName != tt.expectedName {
				t.Errorf("InferencePoolName = %v, want %v", opts.InferencePoolName, tt.expectedName)
			}
		})
	}
}

func TestCompleteTLSConfiguration(t *testing.T) {
	tests := []struct {
		name                         string
		enableTLS                    []string
		tlsInsecureSkipVerify        []string
		deprecatedPrefillerUseTLS    bool
		deprecatedDecoderUseTLS      bool
		deprecatedPrefillerInsecure  bool
		deprecatedDecoderInsecure    bool
		vllmPort                     string
		expectedDecoderURL           string
		expectedUseTLSForPrefiller   bool
		expectedUseTLSForDecoder     bool
		expectedInsecureForPrefiller bool
		expectedInsecureForDecoder   bool
	}{
		{
			name:                         "no TLS configuration",
			enableTLS:                    []string{},
			tlsInsecureSkipVerify:        []string{},
			vllmPort:                     "8001",
			expectedDecoderURL:           "http://localhost:8001",
			expectedUseTLSForPrefiller:   false,
			expectedUseTLSForDecoder:     false,
			expectedInsecureForPrefiller: false,
			expectedInsecureForDecoder:   false,
		},
		{
			name:                         "prefiller TLS only",
			enableTLS:                    []string{"prefiller"},
			tlsInsecureSkipVerify:        []string{},
			vllmPort:                     "8001",
			expectedDecoderURL:           "http://localhost:8001",
			expectedUseTLSForPrefiller:   true,
			expectedUseTLSForDecoder:     false,
			expectedInsecureForPrefiller: false,
			expectedInsecureForDecoder:   false,
		},
		{
			name:                         "decoder TLS only",
			enableTLS:                    []string{"decoder"},
			tlsInsecureSkipVerify:        []string{},
			vllmPort:                     "8001",
			expectedDecoderURL:           "https://localhost:8001",
			expectedUseTLSForPrefiller:   false,
			expectedUseTLSForDecoder:     true,
			expectedInsecureForPrefiller: false,
			expectedInsecureForDecoder:   false,
		},
		{
			name:                         "both stages TLS",
			enableTLS:                    []string{"prefiller", "decoder"},
			tlsInsecureSkipVerify:        []string{},
			vllmPort:                     "9000",
			expectedDecoderURL:           "https://localhost:9000",
			expectedUseTLSForPrefiller:   true,
			expectedUseTLSForDecoder:     true,
			expectedInsecureForPrefiller: false,
			expectedInsecureForDecoder:   false,
		},
		{
			name:                         "TLS with insecure skip verify",
			enableTLS:                    []string{"prefiller", "decoder"},
			tlsInsecureSkipVerify:        []string{"prefiller", "decoder"},
			vllmPort:                     "8001",
			expectedDecoderURL:           "https://localhost:8001",
			expectedUseTLSForPrefiller:   true,
			expectedUseTLSForDecoder:     true,
			expectedInsecureForPrefiller: true,
			expectedInsecureForDecoder:   true,
		},
		{
			name:                         "deprecated flags migration",
			enableTLS:                    []string{},
			tlsInsecureSkipVerify:        []string{},
			deprecatedPrefillerUseTLS:    true,
			deprecatedDecoderUseTLS:      true,
			deprecatedPrefillerInsecure:  true,
			deprecatedDecoderInsecure:    true,
			vllmPort:                     "8001",
			expectedDecoderURL:           "https://localhost:8001",
			expectedUseTLSForPrefiller:   true,
			expectedUseTLSForDecoder:     true,
			expectedInsecureForPrefiller: true,
			expectedInsecureForDecoder:   true,
		},
		{
			name:                         "mixed deprecated and new flags",
			enableTLS:                    []string{"prefiller"},
			tlsInsecureSkipVerify:        []string{},
			deprecatedDecoderUseTLS:      true,
			deprecatedDecoderInsecure:    true,
			vllmPort:                     "8001",
			expectedDecoderURL:           "https://localhost:8001",
			expectedUseTLSForPrefiller:   true,
			expectedUseTLSForDecoder:     true,
			expectedInsecureForPrefiller: false,
			expectedInsecureForDecoder:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := NewOptions()
			opts.enableTLS = tt.enableTLS
			opts.tlsInsecureSkipVerify = tt.tlsInsecureSkipVerify
			opts.prefillerUseTLS = tt.deprecatedPrefillerUseTLS
			opts.decoderUseTLS = tt.deprecatedDecoderUseTLS
			opts.prefillerInsecureSkipVerify = tt.deprecatedPrefillerInsecure
			opts.decoderInsecureSkipVerify = tt.deprecatedDecoderInsecure
			opts.vllmPort = tt.vllmPort

			err := opts.Complete()
			if err != nil {
				t.Fatalf("Complete() unexpected error: %v", err)
			}

			// Verify configuration fields
			if opts.UseTLSForPrefiller != tt.expectedUseTLSForPrefiller {
				t.Errorf("UseTLSForPrefiller = %v, want %v", opts.UseTLSForPrefiller, tt.expectedUseTLSForPrefiller)
			}
			if opts.UseTLSForDecoder != tt.expectedUseTLSForDecoder {
				t.Errorf("UseTLSForDecoder = %v, want %v", opts.UseTLSForDecoder, tt.expectedUseTLSForDecoder)
			}
			if opts.InsecureSkipVerifyForPrefiller != tt.expectedInsecureForPrefiller {
				t.Errorf("InsecureSkipVerifyForPrefiller = %v, want %v", opts.InsecureSkipVerifyForPrefiller, tt.expectedInsecureForPrefiller)
			}
			if opts.InsecureSkipVerifyForDecoder != tt.expectedInsecureForDecoder {
				t.Errorf("InsecureSkipVerifyForDecoder = %v, want %v", opts.InsecureSkipVerifyForDecoder, tt.expectedInsecureForDecoder)
			}
			if opts.DecoderURL == nil || opts.DecoderURL.String() != tt.expectedDecoderURL {
				t.Errorf("TargetURL = %v, want %v", opts.DecoderURL, tt.expectedDecoderURL)
			}

		})
	}
}
