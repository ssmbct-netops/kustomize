/*
Copyright 2018 The Kubernetes Authors.

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

package target

import (
	"encoding/base64"
	"reflect"
	"sort"
	"strings"
	"testing"

	"sigs.k8s.io/kustomize/k8sdeps/kunstruct"
	"sigs.k8s.io/kustomize/pkg/gvk"
	"sigs.k8s.io/kustomize/pkg/ifc"
	"sigs.k8s.io/kustomize/pkg/internal/loadertest"
	"sigs.k8s.io/kustomize/pkg/resid"
	"sigs.k8s.io/kustomize/pkg/resmap"
	"sigs.k8s.io/kustomize/pkg/resource"
	"sigs.k8s.io/kustomize/pkg/types"
)

const (
	kustomizationContent1 = `
apiVersion: v1beta1
kind: Kustomization
namePrefix: foo-
nameSuffix: -bar
namespace: ns1
commonLabels:
  app: nginx
commonAnnotations:
  note: This is a test annotation
resources:
  - deployment.yaml
  - namespace.yaml
configMapGenerator:
- name: literalConfigMap
  literals:
  - DB_USERNAME=admin
  - DB_PASSWORD=somepw
secretGenerator:
- name: secret
  commands:
    DB_USERNAME: "printf admin"
    DB_PASSWORD: "printf somepw"
  type: Opaque
patchesJson6902:
- target:
    group: apps
    version: v1
    kind: Deployment
    name: dply1
  path: jsonpatch.json
`
	kustomizationContent2 = `
apiVersion: v1beta1
kind: Kustomization
secretGenerator:
- name: secret
  timeoutSeconds: 1
  commands:
    USER: "sleep 2"
  type: Opaque
`
	deploymentContent = `apiVersion: apps/v1
metadata:
  name: dply1
kind: Deployment
`
	namespaceContent = `apiVersion: v1
kind: Namespace
metadata:
  name: ns1
`
	jsonpatchContent = `[
    {"op": "add", "path": "/spec/replica", "value": "3"}
]`
)

var rf = resmap.NewFactory(resource.NewFactory(
	kunstruct.NewKunstructuredFactoryImpl()))

var deploy = gvk.Gvk{Group: "apps", Version: "v1", Kind: "Deployment"}
var cmap = gvk.Gvk{Version: "v1", Kind: "ConfigMap"}
var secret = gvk.Gvk{Version: "v1", Kind: "Secret"}
var ns = gvk.Gvk{Version: "v1", Kind: "Namespace"}

func makeALoader(t *testing.T) ifc.Loader {
	ldr := loadertest.NewFakeLoader("/testpath")
	writeK(t, ldr, "/testpath/", kustomizationContent1)
	writeF(t, ldr, "/testpath/deployment.yaml", deploymentContent)
	writeF(t, ldr, "/testpath/namespace.yaml", namespaceContent)
	writeF(t, ldr, "/testpath/jsonpatch.json", jsonpatchContent)
	return ldr
}

func TestResources1(t *testing.T) {
	expected := resmap.ResMap{
		resid.NewResIdWithPrefixSuffixNamespace(
			deploy, "dply1", "foo-", "-bar", "ns1"): rf.RF().FromMap(
			map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name":      "foo-dply1-bar",
					"namespace": "ns1",
					"labels": map[string]interface{}{
						"app": "nginx",
					},
					"annotations": map[string]interface{}{
						"note": "This is a test annotation",
					},
				},
				"spec": map[string]interface{}{
					"replica": "3",
					"selector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"app": "nginx",
						},
					},
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"annotations": map[string]interface{}{
								"note": "This is a test annotation",
							},
							"labels": map[string]interface{}{
								"app": "nginx",
							},
						},
					},
				},
			}),
		resid.NewResIdWithPrefixSuffixNamespace(
			cmap, "literalConfigMap", "foo-", "-bar", "ns1"): rf.RF().FromMap(
			map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "foo-literalConfigMap-bar-8d2dkb8k24",
					"namespace": "ns1",
					"labels": map[string]interface{}{
						"app": "nginx",
					},
					"annotations": map[string]interface{}{
						"note": "This is a test annotation",
					},
				},
				"data": map[string]interface{}{
					"DB_USERNAME": "admin",
					"DB_PASSWORD": "somepw",
				},
			}).SetBehavior(ifc.BehaviorCreate),
		resid.NewResIdWithPrefixSuffixNamespace(
			secret, "secret", "foo-", "-bar", "ns1"): rf.RF().FromMap(
			map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"name":      "foo-secret-bar-9btc7bt4kb",
					"namespace": "ns1",
					"labels": map[string]interface{}{
						"app": "nginx",
					},
					"annotations": map[string]interface{}{
						"note": "This is a test annotation",
					},
				},
				"type": ifc.SecretTypeOpaque,
				"data": map[string]interface{}{
					"DB_USERNAME": base64.StdEncoding.EncodeToString([]byte("admin")),
					"DB_PASSWORD": base64.StdEncoding.EncodeToString([]byte("somepw")),
				},
			}).SetBehavior(ifc.BehaviorCreate),
		resid.NewResIdWithPrefixSuffixNamespace(
			ns, "ns1", "foo-", "-bar", ""): rf.RF().FromMap(
			map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]interface{}{
					"name": "foo-ns1-bar",
					"labels": map[string]interface{}{
						"app": "nginx",
					},
					"annotations": map[string]interface{}{
						"note": "This is a test annotation",
					},
				},
			}),
	}
	actual, err := makeKustTarget(
		t, makeALoader(t)).MakeCustomizedResMap()
	if err != nil {
		t.Fatalf("unexpected Resources error %v", err)
	}

	if !reflect.DeepEqual(actual, expected) {
		err = expected.ErrorIfNotEqual(actual)
		t.Fatalf("unexpected inequality: %v", err)
	}
}

func TestResourceNotFound(t *testing.T) {
	l := loadertest.NewFakeLoader("/testpath")
	writeK(t, l, "/testpath", kustomizationContent1)
	_, err := makeKustTarget(t, l).MakeCustomizedResMap()
	if err == nil {
		t.Fatalf("Didn't get the expected error for an unknown resource")
	}
	if !strings.Contains(err.Error(), `cannot read file`) {
		t.Fatalf("unexpected error: %q", err)
	}
}

func TestSecretTimeout(t *testing.T) {
	l := loadertest.NewFakeLoader("/testpath")
	writeK(t, l, "/testpath", kustomizationContent2)
	_, err := makeKustTarget(t, l).MakeCustomizedResMap()
	if err == nil {
		t.Fatalf("Didn't get the expected error for an unknown resource")
	}
	if !strings.Contains(err.Error(), "killed") {
		t.Fatalf("unexpected error: %q", err)
	}
}

func findSecret(m resmap.ResMap) *resource.Resource {
	for id, res := range m {
		if id.Gvk().Kind == "Secret" {
			return res
		}
	}
	return nil
}

func TestDisableNameSuffixHash(t *testing.T) {
	kt := makeKustTarget(t, makeALoader(t))

	m, err := kt.MakeCustomizedResMap()
	if err != nil {
		t.Fatalf("unexpected Resources error %v", err)
	}
	secret := findSecret(m)
	if secret == nil {
		t.Errorf("Expected to find a Secret")
	}
	if secret.GetName() != "foo-secret-bar-9btc7bt4kb" {
		t.Errorf("unexpected secret resource name: %s", secret.GetName())
	}

	kt.kustomization.GeneratorOptions = &types.GeneratorOptions{
		DisableNameSuffixHash: true}
	m, err = kt.MakeCustomizedResMap()
	if err != nil {
		t.Fatalf("unexpected Resources error %v", err)
	}
	secret = findSecret(m)
	if secret == nil {
		t.Errorf("Expected to find a Secret")
	}
	if secret.GetName() != "foo-secret-bar" { // No hash at end.
		t.Errorf("unexpected secret resource name: %s", secret.GetName())
	}
}

func TestIssue596AllowDirectoriesThatAreSubstringsOfEachOther(t *testing.T) {
	ldr := loadertest.NewFakeLoader(
		"/app/overlays/aws-sandbox2.us-east-1")
	writeK(t, ldr, "/app/base", "")
	writeK(t, ldr, "/app/overlays/aws", `
bases:
- ../../base
`)
	writeK(t, ldr, "/app/overlays/aws-nonprod", `
bases:
- ../aws
`)
	writeK(t, ldr, "/app/overlays/aws-sandbox2.us-east-1", `
bases:
- ../aws-nonprod
`)
	m, err := makeKustTarget(t, ldr).MakeCustomizedResMap()
	if err != nil {
		t.Fatalf("Err: %v", err)
	}
	if m == nil {
		t.Fatalf("Empty map.")
	}
}

// To simplify tests, these vars specified in alphabetical order.
var someVars = []types.Var{
	{
		Name: "AWARD",
		ObjRef: types.Target{
			APIVersion: "v7",
			Gvk:        gvk.Gvk{Kind: "Service"},
			Name:       "nobelPrize"},
		FieldRef: types.FieldSelector{FieldPath: "some.arbitrary.path"},
	},
	{
		Name: "BIRD",
		ObjRef: types.Target{
			APIVersion: "v300",
			Gvk:        gvk.Gvk{Kind: "Service"},
			Name:       "heron"},
		FieldRef: types.FieldSelector{FieldPath: "metadata.name"},
	},
	{
		Name: "FRUIT",
		ObjRef: types.Target{
			Gvk:  gvk.Gvk{Kind: "Service"},
			Name: "apple"},
		FieldRef: types.FieldSelector{FieldPath: "metadata.name"},
	},
	{
		Name: "VEGETABLE",
		ObjRef: types.Target{
			Gvk:  gvk.Gvk{Kind: "Leafy"},
			Name: "kale"},
		FieldRef: types.FieldSelector{FieldPath: "metadata.name"},
	},
}

func TestGetAllVarsSimple(t *testing.T) {
	ldr := loadertest.NewFakeLoader(
		"/app")
	writeK(t, ldr, "/app", `
vars:
  - name: AWARD
    objref:
      kind: Service
      name: nobelPrize
      apiVersion: v7
    fieldref:
      fieldpath: some.arbitrary.path
  - name: BIRD
    objref:
      kind: Service
      name: heron
      apiVersion: v300
`)
	vars, err := makeKustTarget(t, ldr).getAllVars()
	if err != nil {
		t.Fatalf("Err: %v", err)
	}
	if len(vars) != 2 {
		t.Fatalf("unexpected size %d", len(vars))
	}
	for i, k := range sortedKeys(vars)[:2] {
		if !reflect.DeepEqual(vars[k], someVars[i]) {
			t.Fatalf("unexpected var[%d]:\n  %v\n  %v", i, vars[k], someVars[i])
		}
	}
}

func sortedKeys(m map[string]types.Var) (result []string) {
	for k := range m {
		result = append(result, k)
	}
	sort.Strings(result)
	return
}

func TestGetAllVarsNested(t *testing.T) {
	ldr := loadertest.NewFakeLoader(
		"/app/overlays/o2")
	writeK(t, ldr, "/app/base", `
vars:
  - name: AWARD
    objref:
      kind: Service
      name: nobelPrize
      apiVersion: v7
    fieldref:
      fieldpath: some.arbitrary.path
  - name: BIRD
    objref:
      kind: Service
      name: heron
      apiVersion: v300
`)
	writeK(t, ldr, "/app/overlays/o1", `
vars:
  - name: FRUIT
    objref:
      kind: Service
      name: apple
bases:
- ../../base
`)
	writeK(t, ldr, "/app/overlays/o2", `
vars:
  - name: VEGETABLE
    objref:
      kind: Leafy
      name: kale
bases:
- ../o1
`)
	vars, err := makeKustTarget(t, ldr).getAllVars()
	if err != nil {
		t.Fatalf("Err: %v", err)
	}
	if len(vars) != 4 {
		t.Fatalf("unexpected size %d", len(vars))
	}
	for i, k := range sortedKeys(vars) {
		if !reflect.DeepEqual(vars[k], someVars[i]) {
			t.Fatalf("unexpected var[%d]:\n  %v\n  %v", i, vars[k], someVars[i])
		}
	}
}

func TestVarCollisionsForbidden(t *testing.T) {
	ldr := loadertest.NewFakeLoader(
		"/app/overlays/o2")
	writeK(t, ldr, "/app/base", `
vars:
  - name: AWARD
    objref:
      kind: Service
      name: nobelPrize
      apiVersion: v7
    fieldref:
      fieldpath: some.arbitrary.path
  - name: BIRD
    objref:
      kind: Service
      name: heron
      apiVersion: v300
`)
	writeK(t, ldr, "/app/overlays/o1", `
vars:
  - name: AWARD
    objref:
      kind: Service
      name: academy
bases:
- ../../base
`)
	writeK(t, ldr, "/app/overlays/o2", `
vars:
  - name: VEGETABLE
    objref:
      kind: Leafy
      name: kale
bases:
- ../o1
`)
	_, err := makeKustTarget(t, ldr).getAllVars()
	if err == nil {
		t.Fatalf("expected var collision")
	}
	if _, ok := err.(ErrVarCollision); !ok {
		t.Fatalf("unexpected error: %v", err)
	}
}
