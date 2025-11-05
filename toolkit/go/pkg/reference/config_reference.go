// Copyright © 2025 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package reference

// Assumptions and Constraints:
//
// Configuration Structure Requirements:
// - All configuration structs must have proper json tags for field identification
// - Field names must be unique within their scope (no duplicate json field names)
// - Inlined structs cannot have circular references to avoid infinite recursion
// - Map keys are assumed to be strings (not validated, but required for proper path building)
// - Pointer types are expected to be non-nil when accessing their values
//
// Internationalization Requirements:
// - Internationalization messages must exist for all fields (panics if missing)
// - Message keys follow the pattern: "{StructName}.{JsonFieldName}"
// - Missing i18n messages cause panic to catch configuration errors early
//
// Default Value Requirements:
// - PaladinConfigDefaults must contain default values for all root-level fields
// - PaladinConfigMapStructDefaults must contain defaults for structs used in maps/arrays
// - Fields with configdefaults tags must have corresponding entries in map struct defaults
// - Default values must be compatible with their corresponding struct field types
//
// Reflection and Type Safety:
// - All struct types must be reflectable (exported fields, proper tags)
// - Complex types (structs, maps, arrays) in defaults are formatted as "-" for display
// - Interface{} types in maps are handled but may not have meaningful defaults
// - Anonymous/embedded structs are treated the same as inlined structs
//
// Path Building Constraints:
// - Configuration paths use dot notation (e.g., "blockchain.http.timeout")
// - Array/map paths use "[]" suffix for consistency (e.g., "domains[]")
// - Path separators are not configurable (hardcoded as ".")
// - Empty string path represents the root configuration level
//
// Error Handling Assumptions:
// - Missing fields in defaults return "-" rather than causing errors
// - Invalid reflection operations return "-" for graceful degradation
// - Panic is used for missing i18n messages to catch configuration errors early
// - Error context includes field names and paths for easier debugging

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	_ "github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
)

// ConfigDocGenerator generates markdown documentation from Go configuration structs.
// It uses reflection to dynamically discover configuration paths and types,
// then generates documentation with proper linking, default values, and internationalized descriptions.
type ConfigDocGenerator struct {
	// rootDefaults contains the main PaladinConfigDefaults struct for looking up default values
	rootDefaults reflect.Value
	// mapStructDefaults contains PaladinConfigMapStructDefaults for structs that appear in maps/arrays
	mapStructDefaults reflect.Value
	// jsonPaths maps configuration paths (like "domains[]" or "blockchain.http") to their struct types
	// This enables cross-referencing and linking between configuration sections
	jsonPaths map[string]reflect.Type
}

// GenerateConfigReferenceMarkdown creates markdown documentation for all configuration options.
// It discovers all configuration paths dynamically and generates a single comprehensive document
// with proper linking between related configuration sections.
func GenerateConfigReferenceMarkdown(ctx context.Context) (map[string][]byte, error) {
	generator := &ConfigDocGenerator{
		rootDefaults:      reflect.ValueOf(pldconf.PaladinConfigDefaults),
		mapStructDefaults: reflect.ValueOf(pldconf.PaladinConfigMapStructDefaults),
		jsonPaths:         make(map[string]reflect.Type),
	}

	// Discover all configuration paths by traversing the PaladinConfig struct tree
	err := generator.discoverConfigPaths()
	if err != nil {
		return nil, fmt.Errorf("failed to discover config paths: %w", err)
	}

	return generator.generateConfigPages(ctx)
}

// discoverConfigPaths traverses the PaladinConfig struct tree to build a complete map
// of all configuration paths and their corresponding struct types.
// This enables the generator to create proper links between related configuration sections.
func (g *ConfigDocGenerator) discoverConfigPaths() error {
	// Start with the root PaladinConfig struct
	rootConfig := pldconf.PaladinConfig{}
	rootType := reflect.TypeOf(rootConfig)

	// Add the root config to jsonPaths with empty string as key (special case)
	g.jsonPaths[""] = rootType

	// Recursively traverse all nested structs to discover all configuration paths
	return g.traverseConfigStructWithPath(rootType, "", false, false)
}

// traverseConfigStructWithPath recursively traverses a struct and discovers all configuration paths.
// It handles different field types (structs, slices, maps) and creates appropriate paths for each.
//
// Process Overview:
//  1. Dereference pointer types to get the actual struct type
//  2. Add non-inlined structs to jsonPaths map for later cross-referencing
//  3. Process each field in the struct:
//     a. Skip fields with no json tag, "-" tag, or "docexclude" tag
//     b. Extract JSON field name and check for inline flag
//     c. Build configuration path (inlined fields use current path, others append field name)
//     d. Handle different field types recursively:
//     - Regular structs: traverse directly
//     - Slices of structs: create array path with "[]" suffix
//     - Slices of pointers to structs: dereference and traverse
//     - Maps with struct values: create map path with "[]" suffix
//     - Maps with pointer to struct values: dereference and traverse
//
// Key design decisions:
// - Inlined structs (json:",inline") don't create separate sections but flatten their fields
// - Arrays/slices get "[]" suffix in their path (e.g., "wallets[]")
// - Maps with struct values also get "[]" suffix for consistency with arrays
// - Pointer types are dereferenced to get the actual struct type
func (g *ConfigDocGenerator) traverseConfigStructWithPath(structType reflect.Type, currentPath string, isInlineStruct bool, isArrayContext bool) error {
	// Dereference pointer types to get the actual struct
	if structType.Kind() == reflect.Ptr {
		structType = structType.Elem()
	}

	// Only process struct types
	if structType.Kind() != reflect.Struct {
		return nil
	}

	// Add this struct to jsonPaths if it's not inlined and has a meaningful path
	// Inlined structs don't get their own sections - their fields are flattened into the parent
	if !isInlineStruct && currentPath != "" {
		g.jsonPaths[currentPath] = structType
	}

	// Process each field in the struct
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		fieldType := field.Type

		// Dereference pointer types to get the actual type
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}

		// Skip fields that shouldn't be documented
		jsonTag := field.Tag.Get("json")
		excludeTag := field.Tag.Get("docexclude")
		if jsonTag == "" || jsonTag == "-" || excludeTag != "" {
			continue
		}

		// Extract the JSON field name (before any comma-separated options)
		jsonFieldName := strings.Split(jsonTag, ",")[0]

		// Check if this field is inlined (flattened into parent)
		isInlined := strings.Contains(jsonTag, ",inline")

		// Build the configuration path for this field
		var fieldPath string
		if isInlined {
			// Inlined fields use the current path (no additional nesting)
			fieldPath = currentPath
		} else {
			// Regular fields append their name to the current path
			if currentPath == "" {
				fieldPath = jsonFieldName
			} else {
				fieldPath = currentPath + "." + jsonFieldName
			}
		}

		// Handle different field types by recursively traversing nested structures

		// Regular struct fields - traverse directly
		if fieldType.Kind() == reflect.Struct {
			err := g.traverseConfigStructWithPath(fieldType, fieldPath, isInlined, isArrayContext)
			if err != nil {
				return fmt.Errorf("failed to traverse field %s: %w", field.Name, err)
			}
		}

		// Slice of structs - create array path with "[]" suffix and traverse element type
		if fieldType.Kind() == reflect.Slice && fieldType.Elem().Kind() == reflect.Struct {
			elemType := fieldType.Elem()
			arrayPath := g.buildArrayPath(currentPath, jsonFieldName)
			g.jsonPaths[arrayPath] = elemType

			// Traverse with array context so nested fields get correct paths
			err := g.traverseConfigStructWithPath(elemType, arrayPath, false, true)
			if err != nil {
				return fmt.Errorf("failed to traverse slice element type for field %s: %w", field.Name, err)
			}
		}

		// Slice of pointers to structs - dereference pointer and traverse
		if fieldType.Kind() == reflect.Slice && fieldType.Elem().Kind() == reflect.Ptr && fieldType.Elem().Elem().Kind() == reflect.Struct {
			elemType := fieldType.Elem().Elem()
			arrayPath := g.buildArrayPath(currentPath, jsonFieldName)
			g.jsonPaths[arrayPath] = elemType

			err := g.traverseConfigStructWithPath(elemType, arrayPath, false, true)
			if err != nil {
				return fmt.Errorf("failed to traverse slice pointer element type for field %s: %w", field.Name, err)
			}
		}

		// Map with struct values - create map path with "[]" suffix for consistency with arrays
		if fieldType.Kind() == reflect.Map && fieldType.Elem().Kind() == reflect.Struct {
			elemType := fieldType.Elem()
			mapPath := g.buildArrayPath(currentPath, jsonFieldName)
			g.jsonPaths[mapPath] = elemType

			err := g.traverseConfigStructWithPath(elemType, mapPath, false, isArrayContext)
			if err != nil {
				return fmt.Errorf("failed to traverse map value type for field %s: %w", field.Name, err)
			}
		}

		// Map with pointer to struct values - dereference pointer and traverse
		if fieldType.Kind() == reflect.Map && fieldType.Elem().Kind() == reflect.Ptr && fieldType.Elem().Elem().Kind() == reflect.Struct {
			elemType := fieldType.Elem().Elem()
			mapPath := g.buildArrayPath(currentPath, jsonFieldName)
			g.jsonPaths[mapPath] = elemType

			err := g.traverseConfigStructWithPath(elemType, mapPath, false, isArrayContext)
			if err != nil {
				return fmt.Errorf("failed to traverse map pointer value type for field %s: %w", field.Name, err)
			}
		}
	}

	return nil
}

// buildArrayPath creates a path with "[]" suffix for arrays and maps.
// This ensures consistent naming for both arrays and maps with struct values.
func (g *ConfigDocGenerator) buildArrayPath(currentPath, jsonFieldName string) string {
	if currentPath == "" {
		return jsonFieldName + "[]"
	}
	return currentPath + "." + jsonFieldName + "[]"
}

// generateConfigPages creates the final markdown documentation by generating
// a single comprehensive page with all configuration sections.
func (g *ConfigDocGenerator) generateConfigPages(ctx context.Context) (map[string][]byte, error) {
	markdownMap := make(map[string][]byte)

	// Generate single config page with all sections
	mainPage := g.generateSingleConfigPage(ctx)

	// Use the correct relative path for config file in administration directory
	configPath := filepath.Join("..", "..", "..", "..", "doc-site", "docs", "administration", "configuration.md")
	markdownMap[configPath] = mainPage

	return markdownMap, nil
}

// generateSingleConfigPage creates a single markdown document containing all configuration sections.
// It generates tables for each configuration path with proper linking between related sections.
func (g *ConfigDocGenerator) generateSingleConfigPage(ctx context.Context) []byte {
	var buf bytes.Buffer

	buf.WriteString("# Paladin Configuration\n\n")

	// Sort paths alphabetically, but keep root config at the top
	sortedPaths := g.sortConfigPaths()

	// Generate sections for each config path, avoiding duplicates
	processedHeadings := make(map[string]bool)

	for _, path := range sortedPaths {
		var structType reflect.Type
		var actualPath string

		if path == "PaladinConfig" {
			// Special case for root config - no heading, just the fields table
			structType = g.jsonPaths[""]
			actualPath = ""

			// Generate field table for root config
			buf.WriteString("| Key | Description | Type | Default |\n")
			buf.WriteString("|-----|-------------|------|---------|\n")
			g.writeConfigFieldsWithPathAndLinks(ctx, structType, actualPath, &buf)
			buf.WriteString("\n")
		} else {
			// Regular sections with headings
			structType = g.jsonPaths[path]
			actualPath = path

			// Use the full path as the heading (including [] for array sections)
			heading := path

			// Skip if we've already processed a section with this heading
			if processedHeadings[heading] {
				continue
			}
			processedHeadings[heading] = true

			buf.WriteString(fmt.Sprintf("## %s\n\n", heading))
			buf.WriteString("| Key | Description | Type | Default |\n")
			buf.WriteString("|-----|-------------|------|---------|\n")
			g.writeConfigFieldsWithPathAndLinks(ctx, structType, actualPath, &buf)
			buf.WriteString("\n")
		}
	}

	return buf.Bytes()
}

// sortConfigPaths sorts configuration paths alphabetically while keeping the root config at the top.
// This ensures a logical order in the generated documentation.
func (g *ConfigDocGenerator) sortConfigPaths() []string {
	var rootPath string
	var otherPaths []string

	// Separate root path from other paths
	for path := range g.jsonPaths {
		if path == "" {
			rootPath = path
		} else {
			otherPaths = append(otherPaths, path)
		}
	}

	// Sort other paths alphabetically using simple bubble sort
	for i := 0; i < len(otherPaths)-1; i++ {
		for j := i + 1; j < len(otherPaths); j++ {
			if otherPaths[i] > otherPaths[j] {
				otherPaths[i], otherPaths[j] = otherPaths[j], otherPaths[i]
			}
		}
	}

	// Combine root path at the top with sorted other paths
	var sortedPaths []string
	if rootPath != "" || len(otherPaths) > 0 {
		// Use "PaladinConfig" as the display name for the root path
		sortedPaths = append(sortedPaths, "PaladinConfig")
	}
	sortedPaths = append(sortedPaths, otherPaths...)

	return sortedPaths
}

// fieldInfo contains all the information needed to generate documentation for a single field.
type fieldInfo struct {
	field         reflect.StructField // The actual struct field
	jsonFieldName string              // The JSON field name (from json tag)
	fieldPath     string              // The full configuration path (e.g., "blockchain.http.timeout")
	structName    string              // The struct type name (for message key lookup)
}

// writeConfigFieldsWithPathAndLinks generates markdown table rows for all fields in a struct.
// It handles inlined structs by flattening their fields into the parent table.
func (g *ConfigDocGenerator) writeConfigFieldsWithPathAndLinks(ctx context.Context, t reflect.Type, currentPath string, buf *bytes.Buffer) {
	// Collect all fields first (including from inlined structs)
	var allFields []fieldInfo

	// Recursively collect fields from this struct and any inlined structs
	g.collectAllFields(t, currentPath, &allFields)

	// Sort fields alphabetically by JSON field name for consistent output
	for i := 0; i < len(allFields)-1; i++ {
		for j := i + 1; j < len(allFields); j++ {
			if allFields[i].jsonFieldName > allFields[j].jsonFieldName {
				allFields[i], allFields[j] = allFields[j], allFields[i]
			}
		}
	}

	// Generate table rows for each field
	for _, fieldInfo := range allFields {
		field := fieldInfo.field
		jsonFieldName := fieldInfo.jsonFieldName
		fieldPath := fieldInfo.fieldPath
		structName := fieldInfo.structName

		// Get field type string with links to other configuration sections
		fieldTypeStr := g.getFieldTypeStringWithPathLinks(field.Type, fieldPath)

		// Get default value for this field using the configuration path
		defaultValue := g.getDefaultValueForPath(fieldPath)

		// Get internationalized description using struct name and field name
		messageKeyName := structName + "." + jsonFieldName
		description := i18n.Expand(ctx, i18n.MessageKey(messageKeyName))

		// Ensure description exists - panic if missing to catch configuration errors early
		if description == messageKeyName {
			panic(fmt.Sprintf("Missing description for config field %s", messageKeyName))
		}

		// Write the table row with Key | Description | Type | Default format
		fmt.Fprintf(buf, "| %s | %s | %s | %s |\n", jsonFieldName, description, fieldTypeStr, defaultValue)
	}
}

// collectAllFields recursively traverses a struct and collects all its fields,
// including fields from inlined structs. This enables flattening of inlined structures
// into the parent table for better documentation organization.
//
// Process Overview:
//  1. Handle pointer types by dereferencing to get actual struct type
//  2. Iterate through all fields in the struct
//  3. Skip fields that are excluded (docexclude tag) or have no json tag
//  4. For each valid field:
//     a. Extract JSON field name from json tag
//     b. Check if field is inlined (json:",inline" tag)
//     c. If inlined: recursively collect fields from the inlined struct using current path
//     d. If embedded (anonymous field): recursively collect fields using current path
//     e. If regular field: build field path and add to collection
//  5. Return all collected fields for sorting and documentation generation
func (g *ConfigDocGenerator) collectAllFields(t reflect.Type, currentPath string, allFields *[]fieldInfo) {
	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		jsonTag := field.Tag.Get("json")
		excludeTag := field.Tag.Get("docexclude")

		// If this is an embedded struct (anonymous field), recursively collect its fields
		// This must be checked before the JSON tag check because anonymous fields may not have JSON tags
		if field.Anonymous {
			structType := field.Type
			if structType.Kind() == reflect.Ptr {
				structType = structType.Elem()
			}
			// For embedded structs, use the current path (don't append field name)
			g.collectAllFields(structType, currentPath, allFields)
			continue
		}

		// If the field is specifically excluded, or doesn't have a json tag, skip it
		if excludeTag != "" || jsonTag == "" || jsonTag == "-" {
			continue
		}

		// Parse JSON tag to get field name
		jsonFieldName := strings.Split(jsonTag, ",")[0]

		// Check if this field is inlined (has json:",inline" tag)
		isInlined := strings.Contains(jsonTag, ",inline")

		// If this is an inlined struct, recursively collect its fields
		if isInlined {
			structType := field.Type
			if structType.Kind() == reflect.Ptr {
				structType = structType.Elem()
			}
			// For inlined structs, use the current path (don't append field name)
			g.collectAllFields(structType, currentPath, allFields)
			continue
		}

		// Build the field path
		var fieldPath string
		if currentPath == "" {
			fieldPath = jsonFieldName
		} else {
			fieldPath = currentPath + "." + jsonFieldName
		}

		// Add this field to the collection
		*allFields = append(*allFields, fieldInfo{
			field:         field,
			jsonFieldName: jsonFieldName,
			fieldPath:     fieldPath,
			structName:    t.Name(),
		})
	}
}

// getFieldTypeStringWithPathLinks generates a formatted type string with links to related configuration sections.
// It handles different Go types and creates appropriate markdown links for struct types.
//
// Process Overview:
//  1. Dereference pointer types to get actual type
//  2. Handle different type kinds:
//     a. Struct: Look up in jsonPaths using fieldPath context to find the correct specific instance
//     b. Slice: Check element type, create array notation with links for struct elements
//     c. Map: Check value type, create map notation with links for struct values
//     d. Other types: Wrap in backticks for markdown formatting
//  3. For struct types in arrays/maps, clean anchor links by removing "[]" from paths
//
// Design decisions:
// - Struct types link to their configuration sections using fieldPath context for disambiguation
// - Array types show element type in brackets with links
// - Map types show as map[string][Type] with links for struct values
// - All type names are wrapped in backticks for markdown formatting
func (g *ConfigDocGenerator) getFieldTypeStringWithPathLinks(fieldType reflect.Type, fieldPath string) string {
	// Handle pointer types
	if fieldType.Kind() == reflect.Ptr {
		fieldType = fieldType.Elem()
	}

	switch fieldType.Kind() {
	case reflect.Struct:
		structName := fieldType.Name()

		// Build link based on current path context
		// Each link reflects the path to that section, so a link within a section
		// is the current path with the field name added
		if fieldPath != "" {
			// Convert fieldPath to clean anchor format (remove all special characters and make lowercase)
			anchor := strings.ReplaceAll(fieldPath, ".", "")
			anchor = strings.ReplaceAll(anchor, "-", "")
			anchor = strings.ReplaceAll(anchor, "[", "")
			anchor = strings.ReplaceAll(anchor, "]", "")
			anchor = strings.ToLower(anchor)
			return fmt.Sprintf("[`%s`](#%s)", structName, anchor)
		}

		// If no fieldPath provided, just return the struct name without link
		return fmt.Sprintf("`%s`", structName)
	case reflect.Slice:
		elemType := fieldType.Elem()
		if elemType.Kind() == reflect.Ptr {
			elemType = elemType.Elem()
		}
		if elemType.Kind() == reflect.Struct {
			structName := elemType.Name()

			// Build link based on current path context with [] suffix
			if fieldPath != "" {
				// Convert fieldPath to clean anchor format (remove all special characters and make lowercase)
				anchor := strings.ReplaceAll(fieldPath, ".", "")
				anchor = strings.ReplaceAll(anchor, "-", "")
				anchor = strings.ReplaceAll(anchor, "[", "")
				anchor = strings.ReplaceAll(anchor, "]", "")
				anchor = strings.ToLower(anchor)
				return fmt.Sprintf("[`[%s]`](#%s)", structName, anchor)
			}

			// If no fieldPath provided, just return the struct name without link
			return fmt.Sprintf("`[%s]`", structName)
		}
		// For primitive types, show in square brackets with backticks
		return fmt.Sprintf("`[%s]`", elemType.Name())
	case reflect.Map:
		valueType := fieldType.Elem()
		if valueType.Kind() == reflect.Ptr {
			valueType = valueType.Elem()
		}
		if valueType.Kind() == reflect.Struct {
			structName := valueType.Name()

			// Build link based on current path context with [] suffix
			if fieldPath != "" {
				// Convert fieldPath to clean anchor format (remove all special characters and make lowercase)
				anchor := strings.ReplaceAll(fieldPath, ".", "")
				anchor = strings.ReplaceAll(anchor, "-", "")
				anchor = strings.ReplaceAll(anchor, "[", "")
				anchor = strings.ReplaceAll(anchor, "]", "")
				anchor = strings.ToLower(anchor)
				return fmt.Sprintf("[`map[string][%s]`](#%s)", structName, anchor)
			}

			// If no fieldPath provided, just return the struct name without link
			return fmt.Sprintf("`map[string][%s]`", structName)
		}
		// Handle interface{} / any types
		if valueType.Kind() == reflect.Interface {
			return "`map[string][any]`"
		}
		return fmt.Sprintf("`map[string][%s]`", valueType.Name())
	default:
		return fmt.Sprintf("`%s`", fieldType.Name())
	}
}

// getDefaultValueForPath retrieves the default value for a configuration field using its path.
// It handles both regular defaults from PaladinConfigDefaults and map/array defaults from PaladinConfigMapStructDefaults.
// The path-based lookup enables finding defaults for deeply nested configuration options.
//
// Process Overview:
// 1. Split the field path into components (e.g., "blockchain.http.timeout" -> ["blockchain", "http", "timeout"])
// 2. Navigate through the root defaults struct following the path components
// 3. Handle special cases: inlined structs, configdefaults tags, map/array defaults
// 4. Format the final value for display in markdown
//
// Special handling for:
// - Inlined structs: fields are flattened into parent, use current path without nesting
// - Map/array defaults: lookup in PaladinConfigMapStructDefaults using configdefaults tag
// - Pointer types: dereference before processing to get actual struct type
// - Array/map paths: strip "[]" suffix when looking up field names in defaults
func (g *ConfigDocGenerator) getDefaultValueForPath(fieldPath string) string {
	// Split the path into components
	pathParts := strings.Split(fieldPath, ".")

	// Start from the root defaults
	currentValue := g.rootDefaults

	// Navigate through the path
	for i, part := range pathParts {
		if !currentValue.IsValid() {
			return "-"
		}

		// Handle pointer types
		if currentValue.Kind() == reflect.Ptr {
			if currentValue.IsNil() {
				return "-"
			}
			currentValue = currentValue.Elem()
		}

		// Find the field by JSON tag
		if currentValue.Kind() == reflect.Struct {
			found := false
			for j := 0; j < currentValue.Type().NumField(); j++ {
				field := currentValue.Type().Field(j)
				jsonTag := field.Tag.Get("json")
				if jsonTag != "" && jsonTag != "-" {
					jsonFieldName := strings.Split(jsonTag, ",")[0]
					// Strip [] from field name for lookup (added by map/array handling)
					lookupFieldName := strings.TrimSuffix(part, "[]")
					if jsonFieldName == lookupFieldName {
						fieldValue := currentValue.Field(j)

						// Check if this field is inlined
						isInlined := strings.Contains(jsonTag, ",inline")
						if isInlined {
							// Check if this inlined struct has configdefaults tag for map/array struct defaults
							configDefaultsTag := field.Tag.Get("configdefaults")
							if configDefaultsTag != "" {
								return g.getDefaultValueFromMapStructDefaults(configDefaultsTag, pathParts[i+1:])
							}
							// For inlined fields, we need to merge the defaults from the field value
							// with any defaults that might be defined elsewhere (like DefaultHTTPConfig)
							return g.getDefaultValueForInlinedField(fieldValue, pathParts[i+1:])
						}

						// Check if this field has configdefaults tag for map/array struct defaults
						configDefaultsTag := field.Tag.Get("configdefaults")
						if configDefaultsTag != "" {
							return g.getDefaultValueFromMapStructDefaults(configDefaultsTag, pathParts[i+1:])
						}

						currentValue = fieldValue
						found = true
						break
					}
				}
			}

			// If we didn't find the field directly, check if any inlined fields contain it
			if !found {
				for j := 0; j < currentValue.Type().NumField(); j++ {
					field := currentValue.Type().Field(j)
					jsonTag := field.Tag.Get("json")
					if strings.Contains(jsonTag, ",inline") {
						fieldValue := currentValue.Field(j)
						// Try to find the field within the inlined struct
						if g.hasFieldInStruct(fieldValue.Type(), part) {
							// Look for configdefaults tag on the specific field within the inlined struct
							inlinedStructType := fieldValue.Type()
							if inlinedStructType.Kind() == reflect.Ptr {
								inlinedStructType = inlinedStructType.Elem()
							}
							for k := 0; k < inlinedStructType.NumField(); k++ {
								inlinedField := inlinedStructType.Field(k)
								inlinedJsonTag := inlinedField.Tag.Get("json")
								if inlinedJsonTag != "" && inlinedJsonTag != "-" {
									inlinedFieldName := strings.Split(inlinedJsonTag, ",")[0]
									lookupFieldName := strings.TrimSuffix(part, "[]")
									if inlinedFieldName == lookupFieldName {
										configDefaultsTag := inlinedField.Tag.Get("configdefaults")
										if configDefaultsTag != "" {
											// Call the map struct defaults function
											return g.getDefaultValueFromMapStructDefaults(configDefaultsTag, pathParts[i+1:])
										}
									}
								}
							}
							return g.getDefaultValueForInlinedField(fieldValue, pathParts[i+1:])
						}
					}
				}
			}

			// If we still didn't find the field, check if any anonymous/embedded fields contain it
			if !found {
				for j := 0; j < currentValue.Type().NumField(); j++ {
					field := currentValue.Type().Field(j)
					if field.Anonymous {
						fieldValue := currentValue.Field(j)
						// Try to find the field within the embedded struct
						if g.hasFieldInStruct(fieldValue.Type(), part) {
							// Navigate to the specific field within the embedded struct
							return g.getDefaultValueForFieldInStruct(fieldValue, part, pathParts[i+1:])
						}
					}
				}
			}

			if !found {
				return "-"
			}
		} else {
			return "-"
		}
	}

	// Format the final value
	return g.formatDefaultValue(currentValue)
}

// hasFieldInStruct checks if a struct type has a field with the given JSON name
func (g *ConfigDocGenerator) hasFieldInStruct(structType reflect.Type, jsonFieldName string) bool {
	// Handle pointer types
	if structType.Kind() == reflect.Ptr {
		structType = structType.Elem()
	}

	if structType.Kind() != reflect.Struct {
		return false
	}

	// Strip [] from field name for lookup (added by map/array handling)
	lookupFieldName := strings.TrimSuffix(jsonFieldName, "[]")

	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		jsonTag := field.Tag.Get("json")

		// Check if this is an anonymous field first
		if field.Anonymous {
			// Recursively check if the embedded struct has the field
			if g.hasFieldInStruct(field.Type, lookupFieldName) {
				return true
			}
		}

		if jsonTag != "" && jsonTag != "-" {
			fieldJsonName := strings.Split(jsonTag, ",")[0]
			if fieldJsonName == lookupFieldName {
				return true
			}
		}
	}

	return false
}

// getDefaultValueForFieldInStruct finds a specific field within a struct and returns its default value
func (g *ConfigDocGenerator) getDefaultValueForFieldInStruct(structValue reflect.Value, fieldName string, remainingPath []string) string {
	// Handle pointer types
	if structValue.Kind() == reflect.Ptr {
		if structValue.IsNil() {
			return "-"
		}
		structValue = structValue.Elem()
	}

	if structValue.Kind() != reflect.Struct {
		return "-"
	}

	// Find the field by JSON tag
	for i := 0; i < structValue.Type().NumField(); i++ {
		field := structValue.Type().Field(i)
		jsonTag := field.Tag.Get("json")
		if jsonTag != "" && jsonTag != "-" {
			jsonFieldName := strings.Split(jsonTag, ",")[0]
			if jsonFieldName == fieldName {
				fieldValue := structValue.Field(i)

				// If there are remaining path components, continue navigating
				if len(remainingPath) > 0 {
					return g.getDefaultValueForPath(strings.Join(remainingPath, "."))
				}

				// Otherwise, format and return this field's value
				return g.formatDefaultValue(fieldValue)
			}
		}
	}

	return "-"
}

// getDefaultValueForInlinedField handles getting default values for fields within inlined structs
func (g *ConfigDocGenerator) getDefaultValueForInlinedField(inlinedValue reflect.Value, remainingPath []string) string {
	if len(remainingPath) == 0 {
		return g.formatDefaultValue(inlinedValue)
	}

	// Handle pointer types
	if inlinedValue.Kind() == reflect.Ptr {
		if inlinedValue.IsNil() {
			return "-"
		}
		inlinedValue = inlinedValue.Elem()
	}

	// Navigate through the remaining path within the inlined struct
	currentValue := inlinedValue
	for i, part := range remainingPath {
		if !currentValue.IsValid() {
			return "-"
		}

		// Handle pointer types
		if currentValue.Kind() == reflect.Ptr {
			if currentValue.IsNil() {
				return "-"
			}
			currentValue = currentValue.Elem()
		}

		// Find the field by JSON tag
		if currentValue.Kind() == reflect.Struct {
			found := false
			for j := 0; j < currentValue.Type().NumField(); j++ {
				field := currentValue.Type().Field(j)
				jsonTag := field.Tag.Get("json")
				if jsonTag != "" && jsonTag != "-" {
					jsonFieldName := strings.Split(jsonTag, ",")[0]
					// Strip [] from field name for lookup (added by map/array handling)
					lookupFieldName := strings.TrimSuffix(part, "[]")
					if jsonFieldName == lookupFieldName {
						currentValue = currentValue.Field(j)
						found = true
						break
					}
				}
			}

			// If we didn't find the field directly, check if any inlined fields contain it
			if !found {
				for j := 0; j < currentValue.Type().NumField(); j++ {
					field := currentValue.Type().Field(j)
					jsonTag := field.Tag.Get("json")
					if strings.Contains(jsonTag, ",inline") {
						fieldValue := currentValue.Field(j)
						// Try to find the field within the inlined struct
						if g.hasFieldInStruct(fieldValue.Type(), part) {
							return g.getDefaultValueForInlinedField(fieldValue, remainingPath[i+1:])
						}
					}
				}
			}

			if !found {
				return "-"
			}
		} else {
			return "-"
		}
	}

	// Format the final value
	return g.formatDefaultValue(currentValue)
}

// getDefaultValueFromMapStructDefaults retrieves default values from PaladinConfigMapStructDefaults.
// This is used for structs that appear in maps or arrays, where the default value of the container
// is empty, but we still want to show defaults for the struct elements.
//
// Process Overview:
//  1. Get the map struct defaults and handle pointer types
//  2. Navigate to the specific defaults struct using the configdefaults key
//  3. Handle different value types (pointer, interface{}) by dereferencing
//  4. If no remaining path: format and return the whole struct
//  5. If remaining path exists: navigate through the defaults struct following the path
//  6. For each path component:
//     a. Handle pointer types by dereferencing
//     b. Find field by JSON tag name (strip "[]" suffix for lookup)
//     c. Continue to next path component
//  7. Format the final value for display
//
// This enables showing defaults for individual fields within array/map elements even when
// the container itself has no default value (empty array/map).
func (g *ConfigDocGenerator) getDefaultValueFromMapStructDefaults(configDefaultsKey string, remainingPath []string) string {
	// Get the map struct defaults
	mapDefaults := g.mapStructDefaults

	// Handle pointer types
	if mapDefaults.Kind() == reflect.Ptr {
		if mapDefaults.IsNil() {
			return "-"
		}
		mapDefaults = mapDefaults.Elem()
	}

	// Navigate to the specific defaults struct using the configdefaults key
	if mapDefaults.Kind() == reflect.Map {
		// Look up the defaults struct by key
		defaultsValue := mapDefaults.MapIndex(reflect.ValueOf(configDefaultsKey))
		if !defaultsValue.IsValid() {
			return "-"
		}

		// Handle pointer types
		if defaultsValue.Kind() == reflect.Ptr {
			if defaultsValue.IsNil() {
				return "-"
			}
			defaultsValue = defaultsValue.Elem()
		}

		// Handle interface{} types (values from map[string]any)
		if defaultsValue.Kind() == reflect.Interface {
			if defaultsValue.IsNil() {
				return "-"
			}
			defaultsValue = defaultsValue.Elem()
		}

		// If no remaining path, return the whole struct (formatted as "-" for complex types)
		if len(remainingPath) == 0 {
			return g.formatDefaultValue(defaultsValue)
		}

		// Navigate through the remaining path within the defaults struct
		currentValue := defaultsValue
		for _, part := range remainingPath {
			if !currentValue.IsValid() {
				return "-"
			}

			// Handle pointer types
			if currentValue.Kind() == reflect.Ptr {
				if currentValue.IsNil() {
					return "-"
				}
				currentValue = currentValue.Elem()
			}

			// Find the field by JSON tag
			if currentValue.Kind() == reflect.Struct {
				found := false
				for j := 0; j < currentValue.Type().NumField(); j++ {
					field := currentValue.Type().Field(j)
					jsonTag := field.Tag.Get("json")
					if jsonTag != "" && jsonTag != "-" {
						jsonFieldName := strings.Split(jsonTag, ",")[0]
						// Strip [] from field name for lookup (added by map/array handling)
						lookupFieldName := strings.TrimSuffix(part, "[]")
						if jsonFieldName == lookupFieldName {
							currentValue = currentValue.Field(j)
							found = true
							break
						}
					}
				}

				if !found {
					return "-"
				}
			} else {
				return "-"
			}
		}

		// Format the final value
		return g.formatDefaultValue(currentValue)
	}

	return "-"
}

// formatDefaultValue formats a Go value for display in markdown documentation.
// It handles different Go types and formats them appropriately for markdown tables.
//
// Process Overview:
//  1. Check if value is valid, return "-" if not
//  2. Handle pointer types by dereferencing (return "-" if nil)
//  3. Format based on value kind:
//     a. String: Quote and wrap in backticks, return "-" if empty
//     b. Integer types: Format as decimal number in backticks
//     c. Unsigned integer types: Format as decimal number in backticks
//     d. Float types: Format with 2 decimal places in backticks
//     e. Boolean: Format as true/false in backticks
//     f. Complex types (slice, array, map, struct): Return "-" (not displayed)
//     g. Unknown types: Return "-" for safety
//
// Formatting rules:
// - Strings are quoted and wrapped in backticks
// - Empty strings are omitted (show as "-")
// - Numbers and booleans are wrapped in backticks
// - Complex types (structs, maps, arrays) show as "-"
func (g *ConfigDocGenerator) formatDefaultValue(value reflect.Value) string {
	if !value.IsValid() {
		return "-"
	}

	// Handle pointer types by dereferencing them
	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return "-"
		}
		value = value.Elem()
	}

	// Handle different types
	switch value.Kind() {
	case reflect.String:
		str := value.String()
		if str == "" {
			return "-" // Omit empty string defaults
		}
		return fmt.Sprintf("`\"%s\"`", str)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("`%d`", value.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprintf("`%d`", value.Uint())
	case reflect.Float32, reflect.Float64:
		return fmt.Sprintf("`%.2f`", value.Float())
	case reflect.Bool:
		return fmt.Sprintf("`%t`", value.Bool())
	case reflect.Slice, reflect.Array:
		return "-" // Complex types should show "-"
	case reflect.Map:
		return "-" // Complex types should show "-"
	case reflect.Struct:
		return "-" // Complex types should show "-"
	default:
		return "-" // Unknown types should show "-"
	}
}
