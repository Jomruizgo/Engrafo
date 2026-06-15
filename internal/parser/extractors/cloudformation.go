package extractors

import (
	"path"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Jomruizgo/Engrafo/v2/internal/parser"
)

// CloudFormationExtractor extracts a resource-dependency graph from AWS
// CloudFormation / SAM templates. Unlike the tree-sitter extractors it parses
// YAML semantically (gopkg.in/yaml.v3) so it can interpret intrinsic functions
// (!Ref, !GetAtt, !Sub, Fn::ImportValue) and nested-stack references.
//
// Model:
//   - nodes: parameters, resources, outputs (Symbol = logical id)
//   - edges: resource -> referenced logical id   (Ref/GetAtt/Sub/DependsOn)
//            template file -> child template path (nested stacks, cross-file)
//
// Pure Go, no CGO: testable without the tree-sitter build tag.
type CloudFormationExtractor struct{}

func (e *CloudFormationExtractor) Language() parser.Language {
	return parser.LangCloudFormation
}

// subRefPattern matches ${LogicalId} placeholders inside !Sub strings, skipping
// AWS pseudo-parameters like ${AWS::Region} (which contain a colon).
var subRefPattern = regexp.MustCompile(`\$\{([A-Za-z0-9]+)\}`)

func (e *CloudFormationExtractor) Extract(filePath string, source []byte) (*parser.Result, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(source, &doc); err != nil {
		// Not valid YAML (or empty) — nothing to extract, never fail the walk.
		return &parser.Result{}, nil
	}
	root := documentRoot(&doc)
	if root == nil || root.Kind != yaml.MappingNode {
		return &parser.Result{}, nil
	}

	resources := mappingValue(root, "Resources")
	// A CloudFormation/SAM template must have a Resources mapping. Anything else
	// (GitHub Actions, config files, k8s manifests) is ignored.
	if resources == nil || resources.Kind != yaml.MappingNode {
		return &parser.Result{}, nil
	}

	var nodes []parser.Node
	var edges []parser.Edge

	// Collect the set of local logical ids first so edges only point at real,
	// in-template targets (resources, parameters, outputs).
	local := map[string]bool{}
	addNodes := func(section string, kind string) {
		m := mappingValue(root, section)
		if m == nil || m.Kind != yaml.MappingNode {
			return
		}
		for i := 0; i+1 < len(m.Content); i += 2 {
			key := m.Content[i]
			val := m.Content[i+1]
			id := key.Value
			local[id] = true
			sig := ""
			if t := mappingValue(val, "Type"); t != nil {
				sig = t.Value
			}
			nodes = append(nodes, parser.Node{
				Symbol:    id,
				Kind:      kind,
				FilePath:  filePath,
				LineStart: key.Line,
				LineEnd:   key.Line,
				Signature: sig,
				Language:  string(parser.LangCloudFormation),
			})
		}
	}
	addNodes("Parameters", "parameter")
	addNodes("Resources", "resource")
	addNodes("Outputs", "output")

	// Resource edges: each resource references other logical ids via intrinsics,
	// and nested stacks reference a child template file.
	for i := 0; i+1 < len(resources.Content); i += 2 {
		logicalID := resources.Content[i].Value
		body := resources.Content[i+1]

		// Intrinsic + DependsOn references to other local logical ids.
		refs := map[string]bool{}
		collectRefs(body, refs)
		for ref := range refs {
			if ref == logicalID || !local[ref] {
				continue
			}
			edges = append(edges, parser.Edge{
				FromSymbol: logicalID,
				ToSymbol:   ref,
				Kind:       "references",
			})
		}

		// Nested stack: AWS::CloudFormation::Stack (TemplateURL) or
		// AWS::Serverless::Application (Location) pointing at a child template.
		if child := nestedTemplatePath(filePath, body); child != "" {
			edges = append(edges, parser.Edge{
				FromSymbol: logicalID,
				ToSymbol:   child,
				Kind:       "nested_stack",
			})
		}
	}

	return &parser.Result{Nodes: nodes, Edges: edges}, nil
}

// documentRoot unwraps a DocumentNode to its single content mapping.
func documentRoot(n *yaml.Node) *yaml.Node {
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		return n.Content[0]
	}
	return n
}

// mappingValue returns the value node for key in a mapping, or nil.
func mappingValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// collectRefs walks a resource body collecting referenced logical ids from
// intrinsic functions (both short-tag and long form) and DependsOn.
func collectRefs(n *yaml.Node, out map[string]bool) {
	if n == nil {
		return
	}
	switch n.Kind {
	case yaml.ScalarNode:
		switch n.Tag {
		case "!Ref":
			out[n.Value] = true
		case "!GetAtt":
			out[firstSegment(n.Value)] = true
		case "!Sub":
			for _, m := range subRefPattern.FindAllStringSubmatch(n.Value, -1) {
				out[m[1]] = true
			}
		}
	case yaml.SequenceNode:
		if n.Tag == "!GetAtt" && len(n.Content) > 0 {
			out[n.Content[0].Value] = true
		}
		for _, c := range n.Content {
			collectRefs(c, out)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i]
			val := n.Content[i+1]
			switch key.Value {
			case "Ref":
				out[val.Value] = true
			case "Fn::GetAtt":
				if val.Kind == yaml.ScalarNode {
					out[firstSegment(val.Value)] = true
				} else if val.Kind == yaml.SequenceNode && len(val.Content) > 0 {
					out[val.Content[0].Value] = true
				}
			case "Fn::Sub":
				collectSubRefs(val, out)
			case "DependsOn":
				collectDependsOn(val, out)
			}
			collectRefs(val, out)
		}
	}
}

func collectSubRefs(n *yaml.Node, out map[string]bool) {
	if n == nil {
		return
	}
	if n.Kind == yaml.ScalarNode {
		for _, m := range subRefPattern.FindAllStringSubmatch(n.Value, -1) {
			out[m[1]] = true
		}
		return
	}
	if n.Kind == yaml.SequenceNode && len(n.Content) > 0 {
		collectSubRefs(n.Content[0], out)
	}
}

func collectDependsOn(n *yaml.Node, out map[string]bool) {
	switch n.Kind {
	case yaml.ScalarNode:
		out[n.Value] = true
	case yaml.SequenceNode:
		for _, c := range n.Content {
			if c.Kind == yaml.ScalarNode {
				out[c.Value] = true
			}
		}
	}
}

// firstSegment returns the logical id portion of a GetAtt target "Logical.Attr".
func firstSegment(s string) string {
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return s[:i]
	}
	return s
}

// nestedTemplatePath returns the child template path of a nested-stack resource,
// resolved relative to the parent template, or "" if not a nested stack.
func nestedTemplatePath(parentPath string, body *yaml.Node) string {
	typeNode := mappingValue(body, "Type")
	props := mappingValue(body, "Properties")
	if typeNode == nil || props == nil {
		return ""
	}
	var raw string
	switch typeNode.Value {
	case "AWS::CloudFormation::Stack":
		if v := mappingValue(props, "TemplateURL"); v != nil {
			raw = v.Value
		}
	case "AWS::Serverless::Application":
		if v := mappingValue(props, "Location"); v != nil {
			raw = v.Value
		}
	}
	// Only resolve local file references, not s3:// or https:// URLs.
	if raw == "" || strings.Contains(raw, "://") {
		return ""
	}
	return path.Clean(path.Join(path.Dir(parentPath), raw))
}
