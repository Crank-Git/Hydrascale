package scripts

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// docsWorkflow holds the fields of .github/workflows/docs.yml that the publish
// requirements FR-site-39 to FR-site-41 name.
type docsWorkflow struct {
	On struct {
		Push *struct {
			Branches []string `yaml:"branches"`
		} `yaml:"push"`
	} `yaml:"on"`
	Permissions map[string]string `yaml:"permissions"`
	Concurrency struct {
		Group            string `yaml:"group"`
		CancelInProgress bool   `yaml:"cancel-in-progress"`
	} `yaml:"concurrency"`
	Jobs map[string]docsWorkflowJob `yaml:"jobs"`
}

type docsWorkflowJob struct {
	Needs       any               `yaml:"needs"`
	Permissions map[string]string `yaml:"permissions"`
	Environment struct {
		Name string `yaml:"name"`
	} `yaml:"environment"`
	Steps []docsWorkflowStep `yaml:"steps"`
}

type docsWorkflowStep struct {
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	With map[string]string `yaml:"with"`
}

// stepUses returns the with block of the first step that uses action, and false when no
// step uses it.
func (j docsWorkflowJob) stepUses(action string) (map[string]string, bool) {
	for _, s := range j.Steps {
		if s.Uses == action {
			return s.With, true
		}
	}
	return nil, false
}

// runs reports whether a run step of the job holds command.
func (j docsWorkflowJob) runs(command string) bool {
	return slices.ContainsFunc(j.Steps, func(s docsWorkflowStep) bool {
		return strings.Contains(s.Run, command)
	})
}

func TestTheDocsWorkflowDeploysFromMain(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "docs.yml"))
	if err != nil {
		t.Fatalf("read the docs workflow: %v", err)
	}
	var wf docsWorkflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatalf("parse the docs workflow: %v", err)
	}
	// A second pass reads the trigger names. A pull request trigger would publish an
	// unmerged branch, so the test compares the whole set.
	var raw struct {
		On map[string]any `yaml:"on"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parse the docs workflow triggers: %v", err)
	}

	t.Run("triggers on a push to main and on a manual run (FR-site-39)", func(t *testing.T) {
		var triggers []string
		for name := range raw.On {
			triggers = append(triggers, name)
		}
		slices.Sort(triggers)
		if want := []string{"push", "workflow_dispatch"}; !slices.Equal(triggers, want) {
			t.Errorf("the triggers are %q, want %q", triggers, want)
		}
		if wf.On.Push == nil || !slices.Equal(wf.On.Push.Branches, []string{"main"}) {
			t.Errorf("the push trigger does not name the branch main alone")
		}
	})

	t.Run("holds read access to the contents alone at the top level", func(t *testing.T) {
		if want := map[string]string{"contents": "read"}; !maps.Equal(wf.Permissions, want) {
			t.Errorf("the top-level permissions are %v, want %v", wf.Permissions, want)
		}
	})

	t.Run("lets a running deploy finish before the next deploy starts", func(t *testing.T) {
		if wf.Concurrency.Group != "pages" || wf.Concurrency.CancelInProgress {
			t.Errorf("the concurrency is %+v, want the group pages with cancel-in-progress false", wf.Concurrency)
		}
	})

	build, ok := wf.Jobs["build"]
	if !ok {
		t.Fatal("the docs workflow holds no build job")
	}
	deploy, ok := wf.Jobs["deploy"]
	if !ok {
		t.Fatal("the docs workflow holds no deploy job")
	}

	t.Run("builds the site with the build script", func(t *testing.T) {
		if !build.runs("pip install -r docs/site/requirements.txt") {
			t.Error("the build job does not install docs/site/requirements.txt")
		}
		if !build.runs("scripts/docs-build.sh") {
			t.Error("the build job does not run scripts/docs-build.sh")
		}
	})

	t.Run("uploads build/site with the pinned Pages actions (FR-site-40)", func(t *testing.T) {
		if _, ok := build.stepUses("actions/configure-pages@v6"); !ok {
			t.Error("the build job does not use actions/configure-pages@v6")
		}
		with, ok := build.stepUses("actions/upload-pages-artifact@v5")
		if !ok {
			t.Fatal("the build job does not use actions/upload-pages-artifact@v5")
		}
		if with["path"] != "build/site" {
			t.Errorf("the upload path is %q, want build/site", with["path"])
		}
		if _, ok := deploy.stepUses("actions/deploy-pages@v5"); !ok {
			t.Error("the deploy job does not use actions/deploy-pages@v5")
		}
	})

	t.Run("deploys after the build in the github-pages environment (FR-site-41)", func(t *testing.T) {
		if deploy.Needs != "build" && !slices.Equal(anySlice(deploy.Needs), []any{"build"}) {
			t.Errorf("the deploy job needs %v, want build", deploy.Needs)
		}
		want := map[string]string{"pages": "write", "id-token": "write"}
		if !maps.Equal(deploy.Permissions, want) {
			t.Errorf("the deploy job permissions are %v, want %v", deploy.Permissions, want)
		}
		if deploy.Environment.Name != "github-pages" {
			t.Errorf("the deploy job environment is %q, want github-pages", deploy.Environment.Name)
		}
	})
}

func anySlice(v any) []any {
	s, _ := v.([]any)
	return s
}
