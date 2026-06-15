package extractors

import "testing"

func findNode(nodes []nodeLite, symbol string) (nodeLite, bool) {
	for _, n := range nodes {
		if n.symbol == symbol {
			return n, true
		}
	}
	return nodeLite{}, false
}

type nodeLite struct {
	symbol, kind, sig string
}

func TestCloudFormationExtractsResourcesAndRefs(t *testing.T) {
	tmpl := `AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31
Parameters:
  ProjectName:
    Type: String
Resources:
  UserTable:
    Type: AWS::DynamoDB::Table
    Properties:
      TableName: !Sub '${ProjectName}-users'
  FormsFunction:
    Type: AWS::Serverless::Function
    DependsOn: UserTable
    Properties:
      Environment:
        Variables:
          TABLE: !Ref UserTable
          ARN: !GetAtt UserTable.Arn
Outputs:
  TableName:
    Value: !Ref UserTable
`
	e := &CloudFormationExtractor{}
	res, err := e.Extract("template.yaml", []byte(tmpl))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	var nodes []nodeLite
	for _, n := range res.Nodes {
		nodes = append(nodes, nodeLite{n.Symbol, n.Kind, n.Signature})
	}

	// Nodes: parameter, two resources, output.
	if n, ok := findNode(nodes, "ProjectName"); !ok || n.kind != "parameter" {
		t.Errorf("want ProjectName parameter, got %+v ok=%v", n, ok)
	}
	if n, ok := findNode(nodes, "FormsFunction"); !ok || n.kind != "resource" || n.sig != "AWS::Serverless::Function" {
		t.Errorf("want FormsFunction resource with type signature, got %+v ok=%v", n, ok)
	}
	if n, ok := findNode(nodes, "TableName"); !ok || n.kind != "output" {
		t.Errorf("want TableName output, got %+v ok=%v", n, ok)
	}

	// Edges: FormsFunction references UserTable (via Ref, GetAtt, DependsOn);
	// UserTable references ProjectName (via Sub).
	hasEdge := func(from, to string) bool {
		for _, ed := range res.Edges {
			if ed.FromSymbol == from && ed.ToSymbol == to {
				return true
			}
		}
		return false
	}
	if !hasEdge("FormsFunction", "UserTable") {
		t.Errorf("want edge FormsFunction->UserTable; edges=%+v", res.Edges)
	}
	if !hasEdge("UserTable", "ProjectName") {
		t.Errorf("want edge UserTable->ProjectName (from !Sub); edges=%+v", res.Edges)
	}
}

func TestCloudFormationNestedStack(t *testing.T) {
	tmpl := `Resources:
  DynamoDBStack:
    Type: AWS::CloudFormation::Stack
    Properties:
      TemplateURL: ./modules/dynamodb/tables.yaml
`
	e := &CloudFormationExtractor{}
	res, err := e.Extract("template-modular.yaml", []byte(tmpl))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	found := false
	for _, ed := range res.Edges {
		if ed.Kind == "nested_stack" && ed.FromSymbol == "DynamoDBStack" &&
			ed.ToSymbol == "modules/dynamodb/tables.yaml" {
			found = true
		}
	}
	if !found {
		t.Errorf("want nested_stack edge to modules/dynamodb/tables.yaml; edges=%+v", res.Edges)
	}
}

func TestCloudFormationIgnoresNonTemplateYAML(t *testing.T) {
	// GitHub Actions workflow — has no Resources mapping.
	ci := `name: CI
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: make test
`
	e := &CloudFormationExtractor{}
	res, err := e.Extract(".github/workflows/ci.yml", []byte(ci))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(res.Nodes) != 0 || len(res.Edges) != 0 {
		t.Errorf("want empty result for non-template YAML, got %d nodes %d edges", len(res.Nodes), len(res.Edges))
	}
}
