package config_test

import (
	"bytes"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

type templateInput struct {
	Default     any    `yaml:"default"`
	Description string `yaml:"description"`
}

type templateSpec struct {
	Inputs map[string]templateInput `yaml:"inputs"`
}

type templateSpecDoc struct {
	Spec templateSpec `yaml:"spec"`
}

type jobToken struct {
	Aud string `yaml:"aud"`
}

type genericJob struct {
	Stage     string              `yaml:"stage"`
	Image     any                 `yaml:"image"`
	IDTokens  map[string]jobToken `yaml:"id_tokens"`
	Variables map[string]string   `yaml:"variables"`
	Script    any                 `yaml:"script"`
}

type templateBodyDoc struct {
	Jobs map[string]genericJob
}

func (d *templateBodyDoc) UnmarshalYAML(node *yaml.Node) error {
	var raw map[string]yaml.Node
	if err := node.Decode(&raw); err != nil {
		return err
	}
	d.Jobs = make(map[string]genericJob)
	for key, val := range raw {
		if key == "stages" || key == "workflow" || key == "include" || key == "variables" || key == "default" {
			continue
		}
		var job genericJob
		if err := val.Decode(&job); err == nil && job.Stage != "" {
			d.Jobs[key] = job
		}
	}
	return nil
}

func readCoreTemplateBytes(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "templates", "core.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		t.Fatal("templates/core.yml is empty")
	}
	return data
}

func decodeCoreTemplateDocs(t *testing.T) (templateSpecDoc, templateBodyDoc) {
	t.Helper()
	data := readCoreTemplateBytes(t)
	dec := yaml.NewDecoder(bytes.NewReader(data))

	var specDoc templateSpecDoc
	if err := dec.Decode(&specDoc); err != nil {
		t.Fatalf("decode spec document error = %v", err)
	}

	var bodyDoc templateBodyDoc
	if err := dec.Decode(&bodyDoc); err != nil {
		t.Fatalf("decode body document error = %v", err)
	}

	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Errorf("expected EOF after 2 documents, got err = %v, extra = %+v", err, extra)
	}

	return specDoc, bodyDoc
}

func TestCoreTemplate_DocumentStructure(t *testing.T) {
	specDoc, bodyDoc := decodeCoreTemplateDocs(t)

	if len(specDoc.Spec.Inputs) == 0 {
		t.Error("spec.inputs must not be empty")
	}
	job, ok := bodyDoc.Jobs["review:code"]
	if !ok {
		t.Fatal("templates/core.yml missing review:code job")
	}
	if job.Stage != "review" {
		t.Errorf("review:code stage = %q, want %q", job.Stage, "review")
	}
}

func TestCoreTemplate_AnthropicWIFInputs_Boundary(t *testing.T) {
	specDoc, _ := decodeCoreTemplateDocs(t)

	input, ok := specDoc.Spec.Inputs["review_anthropic_audience"]
	if !ok {
		t.Fatal("spec.inputs missing required review_anthropic_audience input")
	}

	gotDefault, ok := input.Default.(string)
	if !ok {
		t.Fatalf("review_anthropic_audience.default type = %T, want string", input.Default)
	}
	if gotDefault != "https://api.anthropic.com" {
		t.Errorf("review_anthropic_audience.default = %q, want %q", gotDefault, "https://api.anthropic.com")
	}
	if strings.TrimSpace(gotDefault) != gotDefault {
		t.Errorf("review_anthropic_audience.default has leading/trailing whitespace: %q", gotDefault)
	}

	parsedURL, err := url.Parse(gotDefault)
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" {
		t.Errorf("review_anthropic_audience.default %q is not a valid HTTPS URL", gotDefault)
	}

	if strings.TrimSpace(input.Description) == "" {
		t.Error("review_anthropic_audience.description must not be empty")
	}
}

func TestCoreTemplate_ReviewCodeIDTokens_Boundary(t *testing.T) {
	_, bodyDoc := decodeCoreTemplateDocs(t)
	job, ok := bodyDoc.Jobs["review:code"]
	if !ok {
		t.Fatal("templates/core.yml missing review:code job")
	}
	idTokens := job.IDTokens

	if len(idTokens) == 0 {
		t.Fatal("review:code.id_tokens must not be empty")
	}

	vaultToken, ok := idTokens["VAULT_ID_TOKEN"]
	if !ok {
		t.Fatal("review:code.id_tokens missing VAULT_ID_TOKEN")
	}
	if vaultToken.Aud != "$[[ inputs.review_vault_audience ]]" {
		t.Errorf("VAULT_ID_TOKEN.aud = %q, want %q", vaultToken.Aud, "$[[ inputs.review_vault_audience ]]")
	}

	anthropicToken, ok := idTokens["ANTHROPIC_ID_TOKEN"]
	if !ok {
		t.Fatal("review:code.id_tokens missing ANTHROPIC_ID_TOKEN")
	}
	if anthropicToken.Aud != "$[[ inputs.review_anthropic_audience ]]" {
		t.Errorf("ANTHROPIC_ID_TOKEN.aud = %q, want %q", anthropicToken.Aud, "$[[ inputs.review_anthropic_audience ]]")
	}
}

func TestCoreTemplate_ReviewCodeVariables_DoNotShadowIDTokens(t *testing.T) {
	_, bodyDoc := decodeCoreTemplateDocs(t)
	job, ok := bodyDoc.Jobs["review:code"]
	if !ok {
		t.Fatal("templates/core.yml missing review:code job")
	}

	for _, tokenVar := range []string{"ANTHROPIC_ID_TOKEN", "VAULT_ID_TOKEN"} {
		if val, exists := job.Variables[tokenVar]; exists {
			t.Errorf("review:code.variables must not declare %s (got %q), which would shadow OIDC id_tokens", tokenVar, val)
		}
	}
}

func TestCoreTemplate_NonReviewJobsDoNotLeakIDTokens(t *testing.T) {
	_, bodyDoc := decodeCoreTemplateDocs(t)

	for jobName, job := range bodyDoc.Jobs {
		if jobName == "review:code" || strings.HasPrefix(jobName, ".") {
			continue
		}
		if len(job.IDTokens) > 0 {
			t.Errorf("job %q declares id_tokens %+v, but only review:code is authorized for OIDC token provisioning", jobName, job.IDTokens)
		}
	}
}
