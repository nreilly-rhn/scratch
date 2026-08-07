package main

import (
	"encoding/json"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestHandleDeployMutationSetsLimit(t *testing.T) {
	limit := int32(10)
	deploy := appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			RevisionHistoryLimit: &limit,
		},
	}
	raw, err := json.Marshal(deploy)
	if err != nil {
		t.Fatal(err)
	}

	resp := handleDeployMutation(&admissionv1.AdmissionRequest{
		Kind: metav1.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		Object: runtime.RawExtension{Raw: raw},
	})
	if !resp.Allowed {
		t.Fatalf("expected allowed, got %#v", resp.Result)
	}
	if resp.PatchType == nil || *resp.PatchType != admissionv1.PatchTypeJSONPatch {
		t.Fatalf("expected JSON patch, got %#v", resp.PatchType)
	}

	var ops []jsonPatchOp
	if err := json.Unmarshal(resp.Patch, &ops); err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].Path != "/spec/revisionHistoryLimit" {
		t.Fatalf("unexpected patch: %#v", ops)
	}
	// JSON numbers decode as float64
	if int32(ops[0].Value.(float64)) != desiredRevisionHistoryLimit {
		t.Fatalf("expected value %d, got %#v", desiredRevisionHistoryLimit, ops[0].Value)
	}
}

func TestHandleDeployMutationNoopWhenAlreadySet(t *testing.T) {
	limit := desiredRevisionHistoryLimit
	deploy := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"},
		Spec:       appsv1.DeploymentSpec{RevisionHistoryLimit: &limit},
	}
	raw, err := json.Marshal(deploy)
	if err != nil {
		t.Fatal(err)
	}

	resp := handleDeployMutation(&admissionv1.AdmissionRequest{
		Kind:   metav1.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		Object: runtime.RawExtension{Raw: raw},
	})
	if !resp.Allowed || resp.Patch != nil {
		t.Fatalf("expected allow with no patch, got allowed=%v patch=%s", resp.Allowed, string(resp.Patch))
	}
}
