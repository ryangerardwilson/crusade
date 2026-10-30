package box

import (
	"bytes"
	"fmt"
	"io"
	"strings"

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

func ParseYAML(data []byte) (Outline, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return Outline{}, fmt.Errorf("failed to read boxes: YAML document is empty")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var document yamlDocument
	if err := decoder.Decode(&document); err != nil {
		return Outline{}, fmt.Errorf("failed to read boxes: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Outline{}, fmt.Errorf("failed to read boxes: multiple YAML documents")
		}
		return Outline{}, fmt.Errorf("failed to read boxes: %w", err)
	}
	if document.Version != OutlineVersion {
		return Outline{}, fmt.Errorf("failed to read boxes: unsupported YAML version %d", document.Version)
	}
	items, err := itemsFromYAML(document.Boxes, nil, map[string]struct{}{})
	if err != nil {
		return Outline{}, fmt.Errorf("failed to read boxes: %w", err)
	}
	outline := Outline{Items: items}
	if err := outline.Validate(); err != nil {
		return Outline{}, fmt.Errorf("failed to read boxes: %w", err)
	}
	return outline, nil
}

func MarshalYAML(outline Outline) ([]byte, error) {
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

func itemsFromYAML(nodes []yamlItem, parents []string, seen map[string]struct{}) ([]Item, error) {
	items := make([]Item, 0, len(nodes))
	for _, node := range nodes {
		label := strings.TrimSpace(node.Label)
		idPart := normalizeID(label)
		if idPart == "" {
			return nil, fmt.Errorf("item label must contain a letter or number")
		}
		nextParents := append(append([]string{}, parents...), idPart)
		id := strings.Join(nextParents, "/")
		if _, ok := seen[id]; ok {
			return nil, fmt.Errorf("duplicate item id %q", id)
		}
		seen[id] = struct{}{}
		children, err := itemsFromYAML(node.Children, nextParents, seen)
		if err != nil {
			return nil, err
		}
		items = append(items, Item{ID: id, Label: label, Children: children})
	}
	return items, nil
}

func yamlItems(items []Item) []yamlItem {
	nodes := make([]yamlItem, 0, len(items))
	for _, item := range items {
		nodes = append(nodes, yamlItem{Label: item.Label, Children: yamlItems(item.Children)})
	}
	return nodes
}
