package internal

import (
	"reflect"

	"github.com/formancehq/go-libs/v2/logging"
	"github.com/formancehq/stack/components/agent/internal/generated"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"
)

// extractVersionsSpec extracts the .spec field from an unstructured Versions
// resource as map[string]string. The Versions CRD spec is simply a map of
// module name → image tag.
func extractVersionsSpec(obj *unstructured.Unstructured) map[string]string {
	spec, _, _ := unstructured.NestedMap(obj.Object, "spec")
	if spec == nil {
		return nil
	}
	result := make(map[string]string, len(spec))
	for k, v := range spec {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	return result
}

func VersionsEventHandler(logger logging.Logger, membershipClient MembershipClient) cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			version := obj.(*unstructured.Unstructured)

			logger.Infof("Detect versions '%s' added", version.GetName())
			if err := membershipClient.Send(&generated.Message{
				Message: &generated.Message_AddedVersion{
					AddedVersion: &generated.AddedVersion{
						Name:       version.GetName(),
						Versions:   extractVersionsSpec(version),
						Deprecated: version.GetAnnotations()["formance.com/deprecated"] == "true",
					},
				},
			}); err != nil {
				logger.Errorf("Unable to send version update: %s", err)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldVersions := oldObj.(*unstructured.Unstructured)
			newVersions := newObj.(*unstructured.Unstructured)

			oldSpec := extractVersionsSpec(oldVersions)
			newSpec := extractVersionsSpec(newVersions)

			if reflect.DeepEqual(oldSpec, newSpec) {
				return
			}

			logger.Infof("Detect versions '%s' modified", newVersions.GetName())
			if err := membershipClient.Send(&generated.Message{
				Message: &generated.Message_UpdatedVersion{
					UpdatedVersion: &generated.UpdatedVersion{
						Name:       newVersions.GetName(),
						Versions:   newSpec,
						Deprecated: newVersions.GetAnnotations()["formance.com/deprecated"] == "true",
					},
				},
			}); err != nil {
				logger.Errorf("Unable to send version update: %s", err)
			}
		},
		DeleteFunc: func(obj interface{}) {
			version := obj.(*unstructured.Unstructured)

			logger.Infof("Detect versions '%s' as deleted", version.GetName())
			if err := membershipClient.Send(&generated.Message{
				Message: &generated.Message_DeletedVersion{
					DeletedVersion: &generated.DeletedVersion{
						Name: version.GetName(),
					},
				},
			}); err != nil {
				logger.Errorf("Unable to send version update: %s", err)
			}
		},
	}
}
