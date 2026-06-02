package internal

import (
	"encoding/json"
	"reflect"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/structpb"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// formanceGroupVersion is the GroupVersion for all Formance CRDs,
// defined locally to avoid importing the operator module.
var formanceGroupVersion = schema.GroupVersion{Group: "formance.com", Version: "v1beta1"}

// status mirrors the operator's v1beta1.Status struct so we can filter
// unstructured status maps without importing the operator module.
type status struct {
	Ready      bool        `json:"ready"`
	Info       string      `json:"info,omitempty"`
	Conditions interface{} `json:"conditions,omitempty"`
}

func restrict[T any](obj map[string]interface{}) (map[string]interface{}, error) {
	if len(obj) == 0 {
		return nil, errors.New("obj is empty")
	}

	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}

	res := new(T)
	err = json.Unmarshal(jsonBytes, &res)
	if err != nil {
		return nil, errors.Wrap(err, "unable to unmarshal json in"+reflect.TypeOf(res).String())
	}

	filtered, err := json.Marshal(res)
	if err != nil {
		return nil, err
	}

	var tmp map[string]interface{}
	if err := json.Unmarshal(filtered, &tmp); err != nil {
		return nil, err
	}

	return tmp, nil
}

func getStatus(unstructuredModule *unstructured.Unstructured) (*structpb.Struct, error) {
	s, found, err := unstructured.NestedMap(unstructuredModule.Object, "status")
	if err != nil {
		return nil, errors.Wrap(err, "unable to get status from unstructured")
	}

	if !found {
		return nil, nil
	}

	s, err = restrict[status](s)
	if err != nil {
		return nil, errors.Wrap(err, "unable to restrict status")
	}

	protoStatus, err := structpb.NewStruct(s)
	if err != nil {
		return nil, errors.Wrap(err, "unable to convert status to proto struct")
	}
	return protoStatus, nil
}
