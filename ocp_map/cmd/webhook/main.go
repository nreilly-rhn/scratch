package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

const desiredRevisionHistoryLimit int32 = 3

var (
	scheme = runtime.NewScheme()
	codecs = serializer.NewCodecFactory(scheme)
)

func init() {
	_ = admissionv1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
}

type jsonPatchOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

func main() {
	var (
		tlsCert = flag.String("tls-cert", "/tls/tls.crt", "TLS certificate file")
		tlsKey  = flag.String("tls-key", "/tls/tls.key", "TLS private key file")
		listen  = flag.String("listen", ":8443", "HTTPS listen address")
	)
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/mutate-deployment", mutateDeployment)

	log.Printf("listening on %s (cert=%s)", *listen, *tlsCert)
	if err := http.ListenAndServeTLS(*listen, *tlsCert, *tlsKey, mux); err != nil {
		log.Printf("server exited: %v", err)
		os.Exit(1)
	}
}

func mutateDeployment(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("read body: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}

	review := admissionv1.AdmissionReview{}
	deserializer := codecs.UniversalDeserializer()
	if _, _, err := deserializer.Decode(body, nil, &review); err != nil {
		http.Error(w, fmt.Sprintf("decode AdmissionReview: %v", err), http.StatusBadRequest)
		return
	}
	if review.Request == nil {
		http.Error(w, "AdmissionReview request is nil", http.StatusBadRequest)
		return
	}

	response := handleDeployMutation(review.Request)
	review.Response = response
	review.Response.UID = review.Request.UID
	review.Request = nil

	out, err := json.Marshal(review)
	if err != nil {
		http.Error(w, fmt.Sprintf("encode response: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}

func handleDeployMutation(req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	if req.Kind.Kind != "Deployment" || req.Kind.Group != "apps" {
		return &admissionv1.AdmissionResponse{
			Allowed: true,
			Result:  &metav1.Status{Message: "ignored non-Deployment"},
		}
	}

	var deploy appsv1.Deployment
	if err := json.Unmarshal(req.Object.Raw, &deploy); err != nil {
		return &admissionv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Message: fmt.Sprintf("could not unmarshal Deployment: %v", err),
			},
		}
	}

	if deploy.Spec.RevisionHistoryLimit != nil &&
		*deploy.Spec.RevisionHistoryLimit == desiredRevisionHistoryLimit {
		log.Printf("allow %s/%s: revisionHistoryLimit already %d",
			deploy.Namespace, deploy.Name, desiredRevisionHistoryLimit)
		return &admissionv1.AdmissionResponse{Allowed: true}
	}

	patch := []jsonPatchOp{{
		Op:    "add",
		Path:  "/spec/revisionHistoryLimit",
		Value: desiredRevisionHistoryLimit,
	}}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return &admissionv1.AdmissionResponse{
			Allowed: false,
			Result:  &metav1.Status{Message: fmt.Sprintf("marshal patch: %v", err)},
		}
	}

	log.Printf("mutate %s/%s: set revisionHistoryLimit=%d (was %v)",
		deploy.Namespace, deploy.Name, desiredRevisionHistoryLimit, deploy.Spec.RevisionHistoryLimit)

	pt := admissionv1.PatchTypeJSONPatch
	return &admissionv1.AdmissionResponse{
		Allowed:   true,
		Patch:     patchBytes,
		PatchType: &pt,
	}
}
