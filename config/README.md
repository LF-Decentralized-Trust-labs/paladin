## Adding New Configuration Properties

### 1. Define the property

Add the configuration property to the appropriate struct in this package. e.g.

```go
type DBConfig struct {
    Type        string         `json:"type"`
    // ...
    NewProperty string         `json:"newProperty"`
}
```

`PaladinConfig` in [`config.go`](./pkg/pldconf/config.go) is the root of the Paladin configuration struct.

### 2. Add Default Values if needed

Define default values inside the `PaladinConfigDefaults` variable:

```go
var DBDefaults = DBConfig{
    NewProperty 
}
var PaladinConfigDefaults = &PaladinConfig{
    DB: DBDefaults,
    // ... other defaults
}
```

For properties of structs that are used in maps or arrays, defaults are handled differently. This is because the map or array necessarily has a size/length of 0 in  `PaladinConfigDefaults`. Instead add default values for these structs to the `PaladinConfigMapStructDefaults` map. The key for the struct in this map should be set as a `configdefaults` tag on the map/array.

```go
type DomainManagerInlineConfig struct {
    Domains map[string]*DomainConfig `json:"domains" configdefaults:"DomainsConfigDefaults"`
}

var PaladinConfigMapStructDefaults = map[string]any{
    "DomainsConfigDefaults": DomainConfigDefaults,
}
```

### 3. Add Internationalized Descriptions

Add message keys for field descriptions in [`common/go/pkg/pldmsgs/en_descriptions.go`](../../common/go/pkg/pldmsgs/en_descriptions.go):

```go
// DBConfig field descriptions
DBConfigType     = pdm("DBConfig.type", "Database type (postgres, sqlite)")
DBConfigPostgres = pdm("DBConfig.postgres", "PostgreSQL configuration")
DBConfigSQLite   = pdm("DBConfig.sqlite", "SQLite configuration")
```

### 4. Generate Documentation

Run the documentation generation:

```bash
gradle :toolkit:go:generateDocs
```

The generated documentation will be written to `doc-site/docs/administration/configuration.md`.

