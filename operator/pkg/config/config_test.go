/*
	Copyright 2024.

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

package config

import (
	"os"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_Success(t *testing.T) {
	// Create a temporary directory to hold the config file
	tempDir := t.TempDir()

	// Create a config file in the tempDir
	configFilePath := filepath.Join(tempDir, "config.json")
	configContent := `{
		"paladin": {
			"image": "paladin-image",
			"imagePullPolicy": "Always",
			"labels": {"app": "paladin"},
			"annotations": {"key": "value"},
			"envs": {"ENV_VAR": "value"},
			"tolerations": [{"key": "key1", "operator": "Exists", "effect": "NoSchedule"}],
			"nodeSelector": {"disktype": "ssd"},
			"securityContext": {"runAsUser": 1000}
		},
		"besu": {
			"image": "besu-image",
			"imagePullPolicy": "IfNotPresent"
		},
		"postgres": {
			"image": "postgres-image",
			"imagePullPolicy": "Always"
		}
	}`

	err := os.WriteFile(configFilePath, []byte(configContent), 0644)
	require.NoError(t, err, "Failed to write config file")

	// Set CONFIG_PATH environment variable to tempDir
	err = os.Setenv("CONFIG_PATH", tempDir)
	require.NoError(t, err)
	defer os.Unsetenv("CONFIG_PATH")

	// Reset viper to ensure a clean state
	viper.Reset()

	// Call LoadConfig
	config, err := LoadConfig()
	require.NoError(t, err, "LoadConfig failed")

	// Validate the config
	assert.Equal(t, "paladin-image", config.Paladin.Image, "Paladin.Image mismatch")
	assert.Equal(t, corev1.PullAlways, config.Paladin.ImagePullPolicy, "Paladin.ImagePullPolicy mismatch")
	assert.Equal(t, "paladin", config.Paladin.Labels["app"], "Paladin.Labels['app'] mismatch")
	assert.Equal(t, "value", config.Paladin.Annotations["key"], "Paladin.Annotations['key'] mismatch")

	// viper does not support case-sensitivity: https://github.com/spf13/viper/issues/1014
	assert.Equal(t, "value", config.Paladin.Envs["env_var"], "Paladin.Envs['ENV_VAR'] mismatch")

	require.Len(t, config.Paladin.Tolerations, 1, "Expected 1 toleration")
	tol := config.Paladin.Tolerations[0]
	assert.Equal(t, "key1", tol.Key, "Toleration key mismatch")
	assert.Equal(t, corev1.TolerationOperator("Exists"), tol.Operator, "Toleration operator mismatch")
	assert.Equal(t, corev1.TaintEffect("NoSchedule"), tol.Effect, "Toleration effect mismatch")

	assert.Equal(t, "ssd", config.Paladin.NodeSelector["disktype"], "Paladin.NodeSelectors['disktype'] mismatch")

	require.NotNil(t, config.Paladin.SecurityContext, "Expected Paladin.SecurityContext to be set")
	require.NotNil(t, config.Paladin.SecurityContext.RunAsUser, "Expected Paladin.SecurityContext.RunAsUser to be set")
	assert.Equal(t, int64(1000), *config.Paladin.SecurityContext.RunAsUser, "Paladin.SecurityContext.RunAsUser mismatch")
}

func TestLoadConfig_MissingFile(t *testing.T) {
	// Create a temporary directory without a config file
	tempDir := t.TempDir()

	// Set CONFIG_PATH environment variable to tempDir
	err := os.Setenv("CONFIG_PATH", tempDir)
	require.NoError(t, err)
	defer os.Unsetenv("CONFIG_PATH")

	// Reset viper to ensure a clean state
	viper.Reset()

	// Call LoadConfig
	_, err = LoadConfig()
	require.Error(t, err, "Expected LoadConfig to fail due to missing config file")
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	// Create a temporary directory to hold the config file
	tempDir := t.TempDir()

	// Create an invalid config file in the tempDir
	configFilePath := filepath.Join(tempDir, "config.json")
	configContent := `{ invalid json }`

	err := os.WriteFile(configFilePath, []byte(configContent), 0644)
	require.NoError(t, err, "Failed to write config file")

	// Set CONFIG_PATH environment variable to tempDir
	err = os.Setenv("CONFIG_PATH", tempDir)
	require.NoError(t, err)
	defer os.Unsetenv("CONFIG_PATH")

	// Reset viper to ensure a clean state
	viper.Reset()

	// Call LoadConfig
	_, err = LoadConfig()
	require.Error(t, err, "Expected LoadConfig to fail due to invalid JSON")
}
