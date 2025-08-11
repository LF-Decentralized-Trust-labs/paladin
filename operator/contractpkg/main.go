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

package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	corev1alpha1 "github.com/LF-Decentralized-Trust-labs/paladin/operator/api/v1alpha1"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/solutils"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/yaml"
)

var scope = map[string][]string{
	"basenet":   {"issuer", "smartcontractdeployment", "transactioninvoke"},
	"devnet":    {"issuer", "smartcontractdeployment", "transactioninvoke", "paladindomain", "paladinregistry"},
	"customnet": {"issuer", "smartcontractdeployment", "transactioninvoke", "paladindomain", "paladinregistry"},
	"attach":    {"issuer", "paladindomain", "paladinregistry"},
}

type ContractMap map[string]*ContractMapBuild

type ContractMapBuild struct {
	Filename   string            `json:"filename"`
	LinkedLibs map[string]string `json:"linkedContracts"`
	Params     any               `json:"params"`
}

var cmd = map[string]func() error{
	"generate":  generateSmartContracts,
	"template":  template,
	"artifacts": generateArtifacts,
}

func generateSmartContracts() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("usage: go run ./%s %s [path/to/contractMap.json]", filepath.Base(os.Args[0]), os.Args[1])
	}

	var buildMap ContractMap
	mapFileData, err := os.ReadFile(os.Args[2])
	if err == nil {
		err = json.Unmarshal(mapFileData, &buildMap)
	}
	if err != nil {
		return fmt.Errorf("failed to parse build map: %s", err)
	}

	for name, build := range buildMap {
		if err := buildMap.process(name, build); err != nil {
			return err
		}
	}

	// See https://github.com/kubernetes-sigs/kustomize/issues/119 for this bit of stupidity
	var kustomizeMap map[string]any
	kustomizeFileData, err := os.ReadFile("config/samples/kustomization.yaml")
	if err == nil {
		err = yaml.Unmarshal(kustomizeFileData, &kustomizeMap)
	}
	if err != nil {
		return err
	}
	for name := range buildMap {
		expectedEntry := fmt.Sprintf("core_v1alpha1_smartcontractdeployment_%s.yaml", name)
		found := false
		for _, entry := range kustomizeMap["resources"].([]any) {
			if entry.(string) == expectedEntry {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("you need to manually add %s to config/samples/kustomization.yaml", expectedEntry)
		}
	}

	return nil
}

func (m *ContractMap) process(name string, b *ContractMapBuild) error {
	outPath := fmt.Sprintf("config/samples/core_v1alpha1_smartcontractdeployment_%s.yaml", name)

	var build solutils.SolidityBuildWithLinks
	inBuildData, err := os.ReadFile(b.Filename)
	if err == nil {
		err = json.Unmarshal(inBuildData, &build)
	}
	if err != nil {
		return err
	}
	if b.Params == nil {
		b.Params = map[string]any{}
	}
	requiredBuilds := []string{}
	linkedContracts := map[string]string{}

	if build.ABI == nil {
		return fmt.Errorf("no ABI: %s", b.Filename)
	}

	if len(build.Bytecode) == 0 || !strings.HasPrefix(build.Bytecode, "0x") {
		return fmt.Errorf("bad bytecode: %s", b.Filename)
	}

	linkReferencesJSON := ""
	if len(build.LinkReferences) > 0 {
		linkReferencesJSON = pldtypes.JSONString(build.LinkReferences).Pretty()
		libCount := 0
		for _, libsInFile := range build.LinkReferences {
			for range libsInFile {
				libCount++
			}
		}

		for libName, link := range b.LinkedLibs {
			link = strings.ReplaceAll(link, "_", "-")
			requiredBuilds = append(requiredBuilds, link)
			l := fmt.Sprintf(`{{index .status.resolvedContractAddresses "%s"}}`, link)
			linkedContracts[libName] = l
		}

		if len(b.LinkedLibs) != libCount {
			return fmt.Errorf("mismatch in links for unlinked Solidity %s expected=%d provided=%d", name, libCount, len(b.LinkedLibs))
		}
	}
	firstNameSegment := strings.SplitN(name, "_", 2)[0]
	scd := corev1alpha1.SmartContractDeployment{
		TypeMeta: v1.TypeMeta{
			APIVersion: "core.paladin.io/v1alpha1",
			Kind:       "SmartContractDeployment",
		},
		ObjectMeta: v1.ObjectMeta{
			Name: strings.ReplaceAll(name, "_", "-"),
			Labels: map[string]string{
				"app.kubernetes.io/name":       "operator-go",
				"app.kubernetes.io/managed-by": "kustomize",
			},
		},
		Spec: corev1alpha1.SmartContractDeploymentSpec{
			Node:                        "node1",
			TxType:                      "public",
			From:                        fmt.Sprintf("%s.operator", firstNameSegment),
			ParamsJSON:                  pldtypes.JSONString(b.Params).Pretty(),
			ABIJSON:                     pldtypes.JSONString(build.ABI).Pretty(),
			Bytecode:                    build.Bytecode,
			LinkReferencesJSON:          linkReferencesJSON,
			RequiredContractDeployments: requiredBuilds,
			LinkedContracts:             linkedContracts,
		},
	}

	outData, err := yaml.Marshal(scd)
	if err != nil {
		return err
	}

	return os.WriteFile(outPath, outData, 0664)

}

// adjust all .yaml files in the directory to use the new template syntax
func template() error {
	if len(os.Args) < 4 {
		return fmt.Errorf("usage: go run ./%s %s [src] [dist]", filepath.Base(os.Args[0]), os.Args[1])
	}
	srcDir := os.Args[2]
	destDir := os.Args[3]

	// Remove the destination directory if it exists
	os.RemoveAll(destDir)

	// Step 1: Create the destination directory if it doesn't exist
	err := os.MkdirAll(destDir, 0755)
	if err != nil {
		return fmt.Errorf("error creating directory %s: %v", destDir, err)
	}

	// Step 2: Copy files from source patterns to the destination directory
	sourcePatterns := []string{
		"*issuer*",
		"*paladindomain*",
		"*paladinregistry*",
		"*smartcontractdeployment*",
		"*transactioninvoke*",
	}

	for _, pattern := range sourcePatterns {

		pattern = filepath.Join(srcDir, pattern)
		files, err := filepath.Glob(pattern)
		if err != nil {
			return fmt.Errorf("error finding files with pattern %s: %v", pattern, err)
		}

		for _, srcFile := range files {

			dstFile := filepath.Join(destDir, filepath.Base(srcFile))
			err := copyFile(srcFile, dstFile)
			if err != nil {
				return fmt.Errorf("error copying file from %s to %s: %v", srcFile, dstFile, err)
			}
		}
	}

	// Step 3: Process all .yaml files in the destination directory
	files, err := filepath.Glob(filepath.Join(destDir, "*.yaml"))
	if err != nil {
		return fmt.Errorf("error finding files: %v", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("no '.yaml' files found in the directory")
	}

	// Compile the regular expression pattern
	pattern := regexp.MustCompile(`\{\{([^}]*)\}\}`)

	// Iterate over each file
	for _, file := range files {
		// Read the file content
		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("error reading file %s: %v", file, err)
		}

		newContent := string(content)

		//  if paladinDomain, add more templating
		if strings.Contains(file, "paladindomain") {

			var domain corev1alpha1.PaladinDomain
			if err := yaml.Unmarshal(content, &domain); err != nil {
				return fmt.Errorf("error unmarshalling content: %v", err)
			}
			n := fmt.Sprintf(".Values.smartContractsReferences.%sFactory", domain.Name)
			domain.Spec.RegistryAddress = fmt.Sprintf("{{ %s.address }}", n)
			domain.Spec.SmartContractDeployment = fmt.Sprintf("{{ %s.deployment }}", n)
			if content, err = yaml.Marshal(domain); err != nil {
				return fmt.Errorf("error marshalling content: %v", err)
			}
			newContent = string(content)
		} else if strings.Contains(file, "paladinregistry") {
			var registry corev1alpha1.PaladinRegistry
			if err := yaml.Unmarshal(content, &registry); err != nil {
				return fmt.Errorf("error unmarshalling content: %v", err)
			}
			registry.Spec.EVM.ContractAddress = "{{ .Values.smartContractsReferences.registry.address }}"
			registry.Spec.EVM.SmartContractDeployment = "{{ .Values.smartContractsReferences.registry.deployment }}"
			if content, err = yaml.Marshal(registry); err != nil {
				return fmt.Errorf("error marshalling content: %v", err)
			}
			newContent = string(content)
		} else {
			// Perform the regex replacement
			newContent = pattern.ReplaceAllString(newContent, "{{ `{{${1}}}` }}")
		}

		// Replace the node name prefix
		newContent = strings.ReplaceAll(newContent, "node1", "\"{{- if eq .Values.mode \"customnet\" }}{{ (index .Values.paladinNodes 0).name }}{{- else }}{{ .Values.paladin.nodeNamePrefix }}1{{- end }}\"")

		// Add conditional wrapper around the content
		vScopes := scopes(file)
		conditions := []string{}
		var condition string
		for _, s := range vScopes {
			conditions = append(conditions, fmt.Sprintf("(eq .Values.mode \"%s\")", s))

			// Build the condition string for the template
			if len(conditions) == 1 {
				// Single condition doesn't need 'or'
				condition = conditions[0]
			} else {
				// Multiple conditions use 'or' to combine them
				condition = fmt.Sprintf("(or %s)", strings.Join(conditions, " "))
			}
		}

		// Wrap newContent with the conditional template
		if len(condition) != 0 {
			newContent = fmt.Sprintf("{{- if %s }}\n\n%s\n{{- end }}", condition, newContent)
		}

		bContent := []byte(newContent)

		// Write the modified content back to the same file
		err = os.WriteFile(file, bContent, fs.FileMode(0644))
		if err != nil {
			return fmt.Errorf("error writing file %s: %v", file, err)
		}

		// Print a message indicating the file has been processed
		fmt.Printf("Processed %s\n", file)
	}
	return nil
}

func generateArtifacts() error {
	if len(os.Args) < 4 {
		return fmt.Errorf("usage: go run ./%s %s [srcDir] [outDir]", filepath.Base(os.Args[0]), os.Args[1])
	}
	srcDir := os.Args[2]
	outDir := os.Args[3]

	// Create the output directory if it doesn't exist
	err := os.MkdirAll(outDir, 0755)
	if err != nil {
		return fmt.Errorf("error creating directory %s: %v", outDir, err)
	}

	// For each scope, combine the YAML files
	for scopeName := range scope {
		combinedContent := ""
		// Collect all files that match the scope
		files, err := filepath.Glob(filepath.Join(srcDir, "*.yaml"))
		if err != nil {
			return fmt.Errorf("error finding YAML files in %s: %v", srcDir, err)
		}

		for _, file := range files {
			filename := filepath.Base(file)
			// Check if the file belongs to the current scope
			if fileBelongsToScope(filename, scopeName) {
				content, err := os.ReadFile(file)
				if err != nil {
					return fmt.Errorf("error reading file %s: %v", file, err)
				}
				// Add a YAML document separator if needed
				if len(combinedContent) > 0 {
					combinedContent += "\n---\n"
				}
				combinedContent += string(content)
			}
		}

		// Write the combined content to a file
		if combinedContent != "" {
			outFile := filepath.Join(outDir, fmt.Sprintf("%s.yaml", scopeName))
			err = os.WriteFile(outFile, []byte(combinedContent), 0644)
			if err != nil {
				return fmt.Errorf("error writing combined YAML file %s: %v", outFile, err)
			}
			fmt.Printf("Combined YAML for scope '%s' written to %s\n", scopeName, outFile)
		} else {
			fmt.Printf("No YAML files found for scope '%s'\n", scopeName)
		}
	}

	// Create a .tar.gz archive for all YAML files in the source directory
	err = createTarGz(srcDir, filepath.Join(outDir, "artifacts.tar.gz"))
	if err != nil {
		return fmt.Errorf("error creating tar.gz archive: %v", err)
	}

	fmt.Printf("Tar.gz archive created at %s\n", filepath.Join(outDir, "artifacts.tar.gz"))
	return nil
}

// createTarGz compresses all YAML files in the source directory into a .tar.gz archive
func createTarGz(srcDir, destFile string) error {
	// Create the output file
	outFile, err := os.Create(destFile)
	if err != nil {
		return fmt.Errorf("error creating tar.gz file %s: %v", destFile, err)
	}
	defer outFile.Close()

	// Create a gzip writer
	gw := gzip.NewWriter(outFile)
	defer gw.Close()

	// Create a tar writer
	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Walk through the source directory and add .yaml files to the archive
	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Only add YAML files
		if filepath.Ext(path) == ".yaml" {
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("error opening file %s: %v", path, err)
			}
			defer file.Close()

			// Create a tar header for the file
			header := &tar.Header{
				Name:    filepath.Base(path),
				Size:    info.Size(),
				Mode:    int64(info.Mode()),
				ModTime: info.ModTime(),
			}
			if err := tw.WriteHeader(header); err != nil {
				return fmt.Errorf("error writing tar header for file %s: %v", path, err)
			}

			// Copy the file content to the tar writer
			_, err = io.Copy(tw, file)
			if err != nil {
				return fmt.Errorf("error writing file %s to tar: %v", path, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("error walking the directory %s: %v", srcDir, err)
	}

	return nil
}
func fileBelongsToScope(filename, scopeName string) bool {
	for _, s := range scopes(filename) {
		if s == scopeName {
			return true
		}
	}
	return false
}

func scopes(filename string) []string {
	var s []string
	for k, v := range scope {
		for _, f := range v {
			if strings.Contains(filename, f) {
				s = append(s, k)
				break
			}
		}
	}
	return s
}

// Helper function to copy a file from src to dst
func copyFile(src, dst string) error {
	// Open the source file
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("error opening source file %s: %w", src, err)
	}
	defer srcFile.Close()

	// Create the destination file
	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("error creating destination file %s: %w", dst, err)
	}
	defer dstFile.Close()

	// Copy the content from source to destination
	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return fmt.Errorf("error copying from %s to %s: %w", src, dst, err)
	}

	// Flush and close the files
	err = dstFile.Sync()
	if err != nil {
		return fmt.Errorf("error syncing destination file %s: %w", dst, err)
	}

	return nil
}

func usageMessage() string {
	commands := []string{}
	for k := range cmd {
		commands = append(commands, k)
	}
	return fmt.Sprintf("usage: go run ./%s %s [ARGS]", filepath.Base(os.Args[0]), strings.Join(commands, "|"))
}

func main() {

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usageMessage())
		os.Exit(1)
	}
	if f, ok := cmd[os.Args[1]]; ok {
		if err := f(); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		return
	}
	fmt.Fprintln(os.Stderr, usageMessage())
	os.Exit(1)
}
