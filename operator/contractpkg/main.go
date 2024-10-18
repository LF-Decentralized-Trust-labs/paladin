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
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

const crTemplate = `
apiVersion: core.paladin.io/v1alpha1
kind: SmartContractDeployment
metadata:
  labels:
    app.kubernetes.io/name: operator-go
    app.kubernetes.io/managed-by: kustomize
  name: {{ .name }}
spec:
  abi: |
{{ .abi | indent 4 }}  
  bytecode: "{{ .bytecode }}"
  deployNode: node1
  deployKey: deployKey
  paramsJSON: |
{{ .params | indent 4 }}

`

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	os.Exit(0)
}

type buildJSON struct {
	ABI      abi.ABI `json:"abi"`
	Bytecode string  `json:"bytecode"`
}

func run() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("usage: go run ./contractpkg [path/to/build.json] [name]")
	}
	name := os.Args[2]
	outPath := fmt.Sprintf("config/samples/core_v1alpha1_smartcontractdeployment_%s.yaml", name)

	t, err := template.New("").Option("missingkey=error").Funcs(sprig.FuncMap()).Parse(crTemplate)
	if err != nil {
		return err
	}

	var build buildJSON
	inBuildData, err := os.ReadFile(os.Args[1])
	if err == nil {
		err = json.Unmarshal(inBuildData, &build)
	}
	if err != nil {
		return err
	}
	tData := map[string]any{
		"name":     name,
		"abi":      tktypes.JSONString(build.ABI).Pretty(),
		"bytecode": build.Bytecode,
		"params":   "{}",
	}
	if len(os.Args) > 3 {
		tData["params"], err = json.MarshalIndent(os.Args[3], "", "  ")
		if err != nil {
			return fmt.Errorf("params must be a single parameter containing JSON (array or object)")
		}
	}

	outData := new(bytes.Buffer)
	if err := t.Execute(outData, tData); err != nil {
		return err
	}

	return os.WriteFile(outPath, outData.Bytes(), 0664)

}
