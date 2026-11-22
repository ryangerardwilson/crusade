// Package boxyaml reads and writes the box outline as the YAML that
// crusade -m box -c opens in Vim. It keeps the YAML library out of the box
// domain.
package boxyaml

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/ryangerardwilson/crusade/internal/box"
	"gopkg.in/yaml.v3"
)

const OutlineVersion = 1

type yamlDocument struct {
	Version int        `yaml:"version"`
	Boxes   []yamlItem `yaml:"boxes"`
}

type yamlItem struct {
	Label    string     `yaml:"label"`
	Children []yamlItem `yaml:"children,omitempty"`
}

func Parse(data []byte) (box.Outline, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return box.Outline{}, fmt.Errorf("failed to read boxes: YAML document is empty")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var document yamlDocument
	if err := decoder.Decode(&document); err != nil {
		return box.Outline{}, fmt.Errorf("failed to read boxes: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return box.Outline{}, fmt.Errorf("failed to read boxes: multiple YAML documents")
		}
		return box.Outline{}, fmt.Errorf("failed to read boxes: %w", err)
	}
	if document.Version != OutlineVersion {
		return box.Outline{}, fmt.Errorf("failed to read boxes: unsupported YAML version %d", document.Version)
	}
	outline := box.Outline{Items: itemsFromYAML(document.Boxes)}
	if err := outline.Validate(); err != nil {
		return box.Outline{}, fmt.Errorf("failed to read boxes: %w", err)
	}
	return outline, nil
}

func Marshal(outline box.Outline) ([]byte, error) {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	document := yamlDocument{Version: OutlineVersion, Boxes: yamlItems(outline.Items)}
	if err := encoder.Encode(document); err != nil {
		_ = encoder.Close()
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func itemsFromYAML(nodes []yamlItem) []box.Item {
	items := make([]box.Item, 0, len(nodes))
	for _, node := range nodes {
		items = append(items, box.Item{Label: strings.TrimSpace(node.Label), Children: itemsFromYAML(node.Children)})
	}
	return items
}

func yamlItems(items []box.Item) []yamlItem {
	nodes := make([]yamlItem, 0, len(items))
	for _, item := range items {
		nodes = append(nodes, yamlItem{Label: item.Label, Children: yamlItems(item.Children)})
	}
	return nodes
}
