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
	"encoding/json"
	"fmt"
	"os"
	"strings"

	corev1alpha1 "github.com/kaleido-io/paladin/operator/api/v1alpha1"
	"github.com/kaleido-io/paladin/toolkit/pkg/solutils"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/yaml"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	os.Exit(0)
}

type ContractMap map[string]*ContractMapBuild

type ContractMapBuild struct {
	Filename   string            `json:"filename"`
	LinkedLibs map[string]string `json:"linkedContracts"`
	Params     any               `json:"params"`
}

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: go run ./contractpkg [path/to/contractMap.json] [true|false]")
	}

	helmCompatible := false
	if len(os.Args) >= 3 {
		helmCompatible = os.Args[2] == "true"
	}

	var buildMap ContractMap
	mapFileData, err := os.ReadFile(os.Args[1])
	if err == nil {
		err = json.Unmarshal(mapFileData, &buildMap)
	}
	if err != nil {
		return fmt.Errorf("failed to parse build map: %s", err)
	}

	for name, build := range buildMap {
		if err := buildMap.process(name, build, helmCompatible); err != nil {
			return err
		}
	}

	// See https://github.com/kubernetes-sigs/kustomize/issues/119 for this bit of stupidity
	var kustomizeMap map[string]any
	kustomizeFileData, err := os.ReadFile("config/kustomization.yaml")
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
			return fmt.Errorf("you need to manually add %s to config/kustomization.yaml", expectedEntry)
		}
	}

	return nil
}

func (m *ContractMap) process(name string, b *ContractMapBuild, helmCompatible bool) error {
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
		linkReferencesJSON = tktypes.JSONString(build.LinkReferences).Pretty()
		libCount := 0
		for _, libsInFile := range build.LinkReferences {
			for range libsInFile {
				libCount++
			}
		}

		for libName, link := range b.LinkedLibs {
			link = strings.ReplaceAll(link, "_", "-")
			requiredBuilds = append(requiredBuilds, link)
			l := fmt.Sprintf(`{{ index .status.resolvedContractAddresses "%s"}}`, link)
			if helmCompatible {
				l = fmt.Sprintf("{{`%s`}}", l)
			}
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
			ParamsJSON:                  tktypes.JSONString(b.Params).Pretty(),
			ABIJSON:                     tktypes.JSONString(build.ABI).Pretty(),
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
