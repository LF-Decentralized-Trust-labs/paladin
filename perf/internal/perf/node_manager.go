// Copyright © 2025 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package perf

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"github.com/LFDT-Paladin/paladin/perf/internal/conf"
	log "github.com/sirupsen/logrus"
)

type NodeManager interface {
	KillNode(ctx context.Context, nodeIndex int) error
	WaitForNodeRestart(ctx context.Context, nodeIndex int, timeout time.Duration) error
	IsNodeHealthy(ctx context.Context, nodeIndex int) bool
}

type nodeManager struct {
	cfg   *conf.NodeKillConfig
	nodes []conf.NodeConfig
}

func NewNodeManager(cfg *conf.NodeKillConfig, nodes []conf.NodeConfig) NodeManager {
	return &nodeManager{
		cfg:   cfg,
		nodes: nodes,
	}
}

func (nm *nodeManager) KillNode(ctx context.Context, nodeIndex int) error {
	if nodeIndex < 0 || nodeIndex >= len(nm.nodes) {
		return fmt.Errorf("invalid node index: %d", nodeIndex)
	}

	if nm.cfg.KillCommandTemplate == "" {
		return fmt.Errorf("kill command template not configured")
	}

	node := nm.nodes[nodeIndex]
	tmplData := struct {
		NodeName string
	}{
		NodeName: node.Name,
	}

	// Parse and execute the template
	tmpl, err := template.New("killCommand").Parse(nm.cfg.KillCommandTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse kill command template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, tmplData); err != nil {
		return fmt.Errorf("failed to execute kill command template: %w", err)
	}

	killCommand := buf.String()
	log.Infof("Executing kill command for node %d (%s): %s", nodeIndex, node.Name, killCommand)

	// Split command into parts for execution
	parts := strings.Fields(killCommand)
	if len(parts) == 0 {
		return fmt.Errorf("empty kill command")
	}

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to kill node %d: %w, output: %s", nodeIndex, err, string(output))
	}

	log.Infof("Successfully killed node %d (%s)", nodeIndex, node.Name)
	return nil
}

func (nm *nodeManager) WaitForNodeRestart(ctx context.Context, nodeIndex int, timeout time.Duration) error {
	if nodeIndex < 0 || nodeIndex >= len(nm.nodes) {
		return fmt.Errorf("invalid node index: %d", nodeIndex)
	}

	deadline := time.Now().Add(timeout)
	checkInterval := 2 * time.Second

	log.Infof("Waiting for node %d (%s) to restart (timeout: %v)", nodeIndex, nm.nodes[nodeIndex].Name, timeout)

	for time.Now().Before(deadline) {
		if nm.IsNodeHealthy(ctx, nodeIndex) {
			log.Infof("Node %d (%s) is healthy again", nodeIndex, nm.nodes[nodeIndex].Name)
			return nil
		}

		time.Sleep(checkInterval)
	}

	return fmt.Errorf("node %d (%s) did not restart within timeout %v", nodeIndex, nm.nodes[nodeIndex].Name, timeout)
}

func (nm *nodeManager) IsNodeHealthy(ctx context.Context, nodeIndex int) bool {
	if nodeIndex < 0 || nodeIndex >= len(nm.nodes) {
		return false
	}

	if nm.cfg.HealthCheckCommand == "" {
		log.Debugf("Health check command not configured for node %d", nodeIndex)
		return false
	}

	node := nm.nodes[nodeIndex]
	tmplData := struct {
		NodeName string
	}{
		NodeName: node.Name,
	}

	// Parse and execute the health check command template
	tmpl, err := template.New("healthCheckCommand").Parse(nm.cfg.HealthCheckCommand)
	if err != nil {
		log.Debugf("Failed to parse health check command template for node %d: %v", nodeIndex, err)
		return false
	}

	var cmdBuf bytes.Buffer
	if err := tmpl.Execute(&cmdBuf, tmplData); err != nil {
		log.Debugf("Failed to execute health check command template for node %d: %v", nodeIndex, err)
		return false
	}

	healthCheckCommand := cmdBuf.String()
	parts := strings.Fields(healthCheckCommand)
	if len(parts) == 0 {
		log.Debugf("Empty health check command for node %d", nodeIndex)
		return false
	}

	// Execute the health check command
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Debugf("Health check command failed for node %d: %v, output: %s", nodeIndex, err, string(output))
		return false
	}

	// Parse the output using the health check template
	if nm.cfg.HealthCheckTemplate == "" {
		log.Debugf("Health check template not configured for node %d", nodeIndex)
		return false
	}

	outputTmpl, err := template.New("healthCheckOutput").Parse(nm.cfg.HealthCheckTemplate)
	if err != nil {
		log.Debugf("Failed to parse health check output template for node %d: %v", nodeIndex, err)
		return false
	}

	// Create a struct to hold the command output for template evaluation
	// Strip quotes that may be included in the output (e.g., from jsonpath)
	trimmedOutput := strings.TrimSpace(string(output))
	trimmedOutput = strings.Trim(trimmedOutput, "'\"")
	outputData := struct {
		Output string
	}{
		Output: trimmedOutput,
	}

	var resultBuf bytes.Buffer
	if err := outputTmpl.Execute(&resultBuf, outputData); err != nil {
		log.Debugf("Failed to execute health check output template for node %d: %v", nodeIndex, err)
		return false
	}

	// The template should output "true" or "false" (or we could use a different approach)
	result := strings.TrimSpace(resultBuf.String())
	return result == "true"
}
