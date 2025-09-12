# Contract Package Tool

This tool auto-generates the CRs that appear in the Helm chart but aren't in the `templates/` directory. It's part of the build pipeline that bridges smart contract compilation and Kubernetes deployment.

## Commands

### `generate`
Converts contract build artifacts to `SmartContractDeployment` CRs.
```bash
go run ./contractpkg/main.go generate [contractMap.json]
```

### `template` 
Converts CRs to Helm templates with proper templating syntax.
```bash
go run ./contractpkg/main.go template [srcDir] [destDir]
```

### `artifacts`
Packages resources by installation mode (basenet, devnet, customnet, attach).
```bash
go run ./contractpkg/main.go artifacts [srcDir] [outDir]
```

## Contract Map Format
```json
{
  "contract_name": {
    "filename": "path/to/contract.sol.json",
    "linkedContracts": {"LibName": "linked_contract"},
    "params": {"param1": "value1"}
  },
  "contract_name2": {
    "filename": "path/to/contract2.sol.json",
    "linkedContracts": {"LibName": "linked_contract"},
    "params": ["value2"]
  }
}
```


 